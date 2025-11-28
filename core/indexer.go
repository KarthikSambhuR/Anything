package core

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
	"github.com/nguyenthenguyen/docx"
)

const MaxReadSize = 50 * 1024 // 50 KB
const FileTimeout = 2 * time.Second

var xmlTagRegex = regexp.MustCompile(`<[^>]*>`)

// --- CLEANING ENGINE ---
func cleanText(raw string, isXml bool) string {
	if isXml {
		// Aggressive Fix: Replace all tags with space to handle table boundaries/formatting
		raw = xmlTagRegex.ReplaceAllString(raw, " ")

		// Decode common XML entities
		raw = strings.ReplaceAll(raw, "&nbsp;", " ")
		raw = strings.ReplaceAll(raw, "&quot;", "\"")
		raw = strings.ReplaceAll(raw, "&apos;", "'")
		raw = strings.ReplaceAll(raw, "&amp;", "&")
		raw = strings.ReplaceAll(raw, "&lt;", "<")
		raw = strings.ReplaceAll(raw, "&gt;", ">")
	}

	var b strings.Builder
	b.Grow(len(raw))
	lastWasSpace := true

	for _, r := range raw {
		// Allow standard text + Newlines
		isValid := (r >= 32 && r <= 126) || r == '\n' || r == '\t'

		if isValid {
			b.WriteRune(r)
			lastWasSpace = false
		} else {
			// Convert strange chars to single space
			if !lastWasSpace {
				b.WriteByte(' ')
				lastWasSpace = true
			}
		}
	}

	return strings.Join(strings.Fields(b.String()), " ")
}

// --- WHITELIST ---
func isContentReadable(ext string) bool {
	e := strings.ToLower(ext)
	switch e {
	case ".txt", ".rtf", ".pdf", ".docx":
		return true
	}
	return false
}

// --- PARSERS ---

func readPdfContent(path string) string {
	defer func() { recover() }()
	f, r, err := pdf.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	var buf bytes.Buffer
	limit := 5
	if r.NumPage() < limit {
		limit = r.NumPage()
	}

	for i := 1; i <= limit; i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		text, _ := p.GetPlainText(nil)
		buf.WriteString(text + " ")
		if buf.Len() > MaxReadSize {
			break
		}
	}
	// PDF extractor returns Plain Text, so isXml = false
	return cleanText(buf.String(), false)
}

func readDocxContent(path string) string {
	defer func() { recover() }()
	r, err := docx.ReadDocxFile(path)
	if err != nil {
		return ""
	}
	defer r.Close()

	// Library returns RAW XML (e.g. <w:t>Hello</w:t>), so isXml = true
	content := r.Editable().GetContent()

	if len(content) > MaxReadSize {
		content = content[:MaxReadSize]
	}

	return cleanText(content, true)
}

func readTextContent(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	buf := make([]byte, MaxReadSize)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return ""
	}

	raw := string(buf[:n])
	ext := strings.ToLower(filepath.Ext(path))
	isXmlOrHtml := ext == ".xml" || ext == ".html" || ext == ".htm" || ext == ".svg"

	return cleanText(raw, isXmlOrHtml)
}

// --- SAFE RUNNER ---
func getContentWithTimeout(path string) string {
	resultChan := make(chan string, 1)

	go func() {
		ext := strings.ToLower(filepath.Ext(path))
		var res string
		switch ext {
		case ".pdf":
			res = readPdfContent(path)
		case ".docx":
			res = readDocxContent(path)
		default:
			res = readTextContent(path)
		}
		resultChan <- res
	}()

	select {
	case res := <-resultChan:
		return res
	case <-time.After(FileTimeout):
		return ""
	}
}

// --- DB HELPERS ---
func LoadFileMap(driveRoot string) (map[string]int64, error) {
	fmt.Printf("Loading index for %s into RAM... ", driveRoot)
	fileMap := make(map[string]int64)
	query := "SELECT path, modified_time FROM files WHERE path LIKE ?"
	rows, err := DB.Query(query, driveRoot+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var path string
		var modTime int64
		if err := rows.Scan(&path, &modTime); err != nil {
			continue
		}
		fileMap[path] = modTime
		count++
	}
	fmt.Printf("Loaded %d files.\n", count)
	return fileMap, nil
}

// --- PHASE 1: QUICK SCAN ---
func RunQuickScan(root string) {
	fmt.Printf("\n>>> PHASE 1: Quick Scan (Filenames) on %s\n", root)
	startTime := time.Now()
	stats := struct{ Added, Updated, Skipped, Scanned int }{}

	existingFiles, err := LoadFileMap(root)
	if err != nil {
		return
	}

	tx, err := DB.Begin()
	if err != nil {
		return
	}

	insertStmt, _ := tx.Prepare(`INSERT INTO files (path, filename, extension, modified_time, summary) VALUES (?, ?, ?, ?, NULL)`)
	updateStmt, _ := tx.Prepare(`UPDATE files SET modified_time = ?, summary = NULL WHERE path = ?`)
	defer insertStmt.Close()
	defer updateStmt.Close()

	batchSize := 2000

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			name := d.Name()

			// 1. GLOBAL SKIPS
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "$RECYCLE.BIN" || name == "System Volume Information" {
				return filepath.SkipDir
			}

			// 2. ROOT SKIPS: System folders at drive root
			if name == "Windows" || name == "Program Files" || name == "Program Files (x86)" {
				parent := filepath.Dir(path)
				if filepath.Clean(parent) == filepath.Clean(root) {
					return filepath.SkipDir
				}
			}

			// 3. SPECIFIC JUNK: Windows Store App Data packages
			if name == "Packages" && strings.Contains(path, "AppData\\Local\\Packages") {
				return filepath.SkipDir
			}

			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		stats.Scanned++

		currentModTime := info.ModTime().Unix()
		storedModTime, exists := existingFiles[path]

		if exists {
			if storedModTime == currentModTime {
				stats.Skipped++
				delete(existingFiles, path)
				return nil
			}
			_, err = updateStmt.Exec(currentModTime, path)
			stats.Updated++
		} else {
			_, err = insertStmt.Exec(path, d.Name(), filepath.Ext(d.Name()), currentModTime)
			stats.Added++
		}

		delete(existingFiles, path)

		if (stats.Added+stats.Updated)%batchSize == 0 && (stats.Added+stats.Updated) > 0 {
			tx.Commit()
			tx, _ = DB.Begin()
			insertStmt, _ = tx.Prepare(`INSERT INTO files (path, filename, extension, modified_time, summary) VALUES (?, ?, ?, ?, NULL)`)
			updateStmt, _ = tx.Prepare(`UPDATE files SET modified_time = ?, summary = NULL WHERE path = ?`)
			fmt.Printf("\r[QuickScan] Scanned: %d | New: %d | Upd: %d", stats.Scanned, stats.Added, stats.Updated)
		}
		return nil
	})

	tx.Commit()
	fmt.Printf("\nPHASE 1 Complete! Time: %v\n", time.Since(startTime))
}

// --- PHASE 2: DEEP SCAN ---
func RunDeepScan() {
	fmt.Println("\n>>> PHASE 2: Deep Scan (Content Extraction)")
	startTime := time.Now()
	processedCount := 0

	rows, err := DB.Query("SELECT path, filename FROM files WHERE summary IS NULL")
	if err != nil {
		fmt.Printf("Error querying: %v\n", err)
		return
	}
	defer rows.Close()

	var pendingFiles []string
	for rows.Next() {
		var path, name string
		rows.Scan(&path, &name)
		if isContentReadable(filepath.Ext(name)) {
			pendingFiles = append(pendingFiles, path)
		}
	}
	rows.Close()

	total := len(pendingFiles)
	fmt.Printf("Found %d files needing content extraction.\n", total)
	if total == 0 {
		return
	}

	tx, _ := DB.Begin()
	updateStmt, _ := tx.Prepare("UPDATE files SET summary = ? WHERE path = ?")
	defer updateStmt.Close()

	for _, path := range pendingFiles {
		processedCount++

		percent := (processedCount * 100) / total
		fmt.Printf("\r[DeepScan] [%d/%d] (%d%%) Reading: %-40s", processedCount, total, percent, truncateString(filepath.Base(path), 40))

		content := getContentWithTimeout(path)

		_, err := updateStmt.Exec(content, path)
		if err != nil {
			fmt.Printf("\nError saving %s: %v\n", path, err)
		}

		if processedCount%100 == 0 {
			tx.Commit()
			tx, _ = DB.Begin()
			updateStmt, _ = tx.Prepare("UPDATE files SET summary = ? WHERE path = ?")
		}
	}

	tx.Commit()
	fmt.Printf("\nPHASE 2 Complete! Extracted text from %d files in %v\n", processedCount, time.Since(startTime))
}

func chunkText(text string, maxChunks int) []string {
	words := strings.Fields(text)
	var chunks []string

	chunkSize := 300
	overlap := 50

	for i := 0; i < len(words); i += (chunkSize - overlap) {
		end := i + chunkSize
		if end > len(words) {
			end = len(words)
		}

		segment := strings.Join(words[i:end], " ")
		chunks = append(chunks, segment)

		if len(chunks) >= maxChunks {
			break
		}
	}
	if len(chunks) == 0 && len(text) > 0 {
		return []string{text}
	}
	return chunks
}

func RunEmbeddingScan() {
	if !IsAIReady {
		fmt.Println("⚠️  AI Engine not ready. Skipping semantic indexing.")
		return
	}

	fmt.Println("\n>>> PHASE 3: AI Embedding Generation")
	startTime := time.Now()

	pendingFiles, err := GetFilesNeedingEmbedding()
	if err != nil {
		fmt.Printf("Error querying DB: %v\n", err)
		return
	}

	total := len(pendingFiles)
	fmt.Printf("Found %d files needing vectors.\n", total)
	if total == 0 {
		return
	}

	maxChunks := CurrentSettings.MaxChunksPerFile
	if maxChunks < 1 {
		maxChunks = 1
	}
	if CurrentSettings.EmbeddingStrategy == "simple" {
		maxChunks = 1 // Force single vector for simple mode
	}

	count := 0

	for id, summary := range pendingFiles {
		count++
		percent := (count * 100) / total
		fmt.Printf("\r[AI Scan] [%d/%d] (%d%%) Embedding...", count, total, percent)

		var chunks []string

		if CurrentSettings.EmbeddingStrategy == "simple" {
			chunks = []string{summary}
		} else {
			chunks = chunkText(summary, maxChunks)
		}

		for i, segment := range chunks {
			if len(segment) < 10 {
				continue
			}

			vec, err := GetEmbedding(segment)
			if err != nil {
				fmt.Printf("\nAI Error on file %d: %v\n", id, err)
				continue
			}

			SaveVector(id, i, vec)
		}
	}

	fmt.Printf("\nPHASE 3 Complete! Vectors generated in %v\n", time.Since(startTime))
}

func RunIconScan() {
	fmt.Println("\n>>> PHASE 4: Caching System Icons (to DB)")

	var totalFiles int
	DB.QueryRow("SELECT count(*) FROM files").Scan(&totalFiles)
	fmt.Printf("[Debug] Total files in DB: %d\n", totalFiles)

	rows, err := DB.Query("SELECT DISTINCT extension FROM files")
	if err != nil {
		fmt.Printf("Error querying extensions: %v\n", err)
		return
	}
	defer rows.Close()

	stmt, _ := DB.Prepare(`INSERT OR REPLACE INTO extension_icons (extension, icon_data) VALUES (?, ?)`)
	defer stmt.Close()

	count := 0
	foundExtensions := 0

	for rows.Next() {
		var ext string
		if err := rows.Scan(&ext); err != nil {
			continue
		}

		if ext == "" || len(ext) > 10 {
			continue
		}

		foundExtensions++

		b64 := GetExtensionIconBase64(ext)
		if b64 != "" {
			_, err := stmt.Exec(ext, b64)
			if err == nil {
				count++
			}
		}
	}

	fmt.Printf("Cached icons for %d / %d file types.\n", count, foundExtensions)
}

func truncateString(str string, num int) string {
	if len(str) > num {
		return str[0:num] + "..."
	}
	return str
}

func GetDrives() []string {
	var drives []string
	for _, drive := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		path := string(drive) + ":\\"
		_, err := os.ReadDir(path)
		if err == nil {
			drives = append(drives, path)
		}
	}
	return drives
}
