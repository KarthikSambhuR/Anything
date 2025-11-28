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
		// Replace tags with space so "hello</b><b>world" becomes "hello world"
		// instead of "helloworld"
		raw = xmlTagRegex.ReplaceAllString(raw, " ")

		raw = strings.ReplaceAll(raw, "&nbsp;", " ")
		raw = strings.ReplaceAll(raw, "&amp;", "&")
		raw = strings.ReplaceAll(raw, "&lt;", "<")
		raw = strings.ReplaceAll(raw, "&gt;", ">")
	}

	var b strings.Builder
	b.Grow(len(raw))
	lastWasSpace := false

	for _, r := range raw {
		// Allow standard ASCII (32-126), newline, and tab
		isValid := (r >= 32 && r <= 126) || r == '\n' || r == '\t'

		if isValid {
			b.WriteRune(r)
			lastWasSpace = false
		} else {
			if !lastWasSpace {
				b.WriteByte(' ')
				lastWasSpace = true
			}
		}
	}

	// Collapse multiple spaces into one using Fields
	return strings.Join(strings.Fields(b.String()), " ")
}

// --- WHITELIST ---
func isContentReadable(ext string) bool {
	e := strings.ToLower(ext)
	switch e {
	case ".txt", ".md", ".markdown", ".rtf", ".pdf", ".docx":
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

	// This library returns RAW XML (e.g. <w:t>Hello</w:t>), so we must use isXml=true
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
	// HTML/XML/SVG need tag stripping
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
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "$RECYCLE.BIN" || name == "System Volume Information" {
				return filepath.SkipDir
			}
			if name == "Windows" || name == "Program Files" || name == "Program Files (x86)" {
				// Only skip these if they are at the drive root (e.g. C:\Windows)
				parent := filepath.Dir(path)
				if filepath.Clean(parent) == filepath.Clean(root) {
					return filepath.SkipDir
				}
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
