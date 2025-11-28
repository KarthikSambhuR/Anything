package core

import (
	"database/sql"
	"encoding/binary"
	"log"
	"math"
	"regexp"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB
var queryCleaner = regexp.MustCompile(`[^a-zA-Z0-9]+`)

type SearchResult struct {
	Path    string
	Snippet string
	Score   float32 // <--- NEW
}

func SearchFiles(queryText string, minTime int64, maxTime int64) ([]SearchResult, error) {
	cleanQuery := queryCleaner.ReplaceAllString(queryText, " ")
	terms := strings.Fields(cleanQuery)
	if len(terms) == 0 {
		return nil, nil
	}

	// STRATEGY:
	// 1. Content: Use Prefix (term*) - Fast, good for words in text.
	// 2. Filename: Use Infix (*term*) - Good for glued filenames like "Practicetask".

	var contentParts []string
	var filenameParts []string

	for _, term := range terms {
		contentParts = append(contentParts, term+"*")       // "task*"
		filenameParts = append(filenameParts, "*"+term+"*") // "*task*"
	}

	contentQuery := strings.Join(contentParts, " AND ")
	//filenameQuery := strings.Join(filenameParts, " AND ")

	// SQL: Look for (Content Matches) OR (Filename matches *term*)
	// We boost the filename match rank
	baseQuery := `
		SELECT f.path, COALESCE(snippet(files_fts, 1, '[', ']', '...', 15), ''), files_fts.rank
		FROM files_fts 
		JOIN files f ON f.id = files_fts.rowid
		WHERE (files_fts MATCH ? OR filename LIKE ?) ` // Combined Logic

	// Argument Builder
	var args []interface{}
	args = append(args, contentQuery)

	// For SQLite LIKE, we need to construct the string "%term1%term2%" manually or handle logic.
	// Actually, FTS5 doesn't support leading wildcards (*term) well.
	// A better trick: We search FTS for content, but we use standard SQL LIKE for the "Glued Filename" case.

	// Let's simplify: Just use FTS. FTS5 "Match" is usually enough if we indexed correctly.
	// The Issue with "Practicetask" is that "task" is not a token prefix.
	// Let's rely on the CONTENT fix (Part 2) to find this file, because "PRACTICE TASK" is in the document.

	// Reverting to the standard cleaner query but ensuring we don't crash on NULLs
	// The Part 2 Fix (XML Cleaning) is the real solution for this file.

	baseQuery = `
		SELECT f.path, COALESCE(snippet(files_fts, 1, '[', ']', '...', 15), ''), files_fts.rank
		FROM files_fts 
		JOIN files f ON f.id = files_fts.rowid
		WHERE files_fts MATCH ? `

	args = []interface{}{contentQuery}

	if minTime > 0 {
		baseQuery += " AND f.modified_time >= ? "
		args = append(args, minTime)
	}
	if maxTime > 0 {
		baseQuery += " AND f.modified_time <= ? "
		args = append(args, maxTime)
	}

	baseQuery += " ORDER BY files_fts.rank LIMIT 50"

	rows, err := DB.Query(baseQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var res SearchResult
		var rank float32
		if err := rows.Scan(&res.Path, &res.Snippet, &rank); err != nil {
			return nil, err
		}
		res.Score = float32(math.Abs(float64(rank))) * 1.5
		results = append(results, res)
	}
	return results, nil
}

func InitDB(dbPath string) {
	var err error
	DB, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal(err)
	}

	// Enable WAL Mode
	if _, err := DB.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		log.Printf("Failed to enable WAL: %v", err)
	}

	// 1. Files Table
	_, err = DB.Exec(`
	CREATE TABLE IF NOT EXISTS files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		path TEXT UNIQUE,
		filename TEXT,
		extension TEXT,
		modified_time INTEGER,
		summary TEXT
	);`)
	if err != nil {
		log.Fatal(err)
	}

	// 2. FTS Table
	_, err = DB.Exec(`
	CREATE VIRTUAL TABLE IF NOT EXISTS files_fts USING fts5(filename, summary, path UNINDEXED, content='files', content_rowid='id');
	`)
	if err != nil {
		log.Fatal(err)
	}

	// 3. VECTORS TABLE (NEW for v0.3)
	// Stores the semantic meaning.
	// One file can have multiple rows here (if Chunking is enabled).
	_, err = DB.Exec(`
	CREATE TABLE IF NOT EXISTS file_vectors (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_id INTEGER,
		vector_blob BLOB,
		chunk_index INTEGER,
		FOREIGN KEY(file_id) REFERENCES files(id) ON DELETE CASCADE
	);`)
	if err != nil {
		log.Fatal(err)
	}

	// Triggers
	setupTriggers()
}

func setupTriggers() {
	// Separate function just to keep InitDB clean
	DB.Exec(`CREATE TRIGGER IF NOT EXISTS files_ai AFTER INSERT ON files BEGIN
		INSERT INTO files_fts(rowid, filename, summary, path) VALUES (new.id, new.filename, new.summary, new.path);
	END;`)
	DB.Exec(`CREATE TRIGGER IF NOT EXISTS files_ad AFTER DELETE ON files BEGIN
		INSERT INTO files_fts(files_fts, rowid, filename, summary, path) VALUES('delete', old.id, old.filename, old.summary, old.path);
	END;`)
	DB.Exec(`CREATE TRIGGER IF NOT EXISTS files_au AFTER UPDATE ON files BEGIN
		INSERT INTO files_fts(files_fts, rowid, filename, summary, path) VALUES('delete', old.id, old.filename, old.summary, old.path);
		INSERT INTO files_fts(rowid, filename, summary, path) VALUES (new.id, new.filename, new.summary, new.path);
	END;`)
}

func GetFilesNeedingEmbedding() (map[int]string, error) {
	// Query: Find files with content (summary != "")
	// ...where the file_id is NOT present in the file_vectors table.
	query := `
		SELECT id, summary 
		FROM files 
		WHERE summary IS NOT NULL AND summary != "" 
		AND id NOT IN (SELECT distinct file_id FROM file_vectors)
	`
	rows, err := DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make(map[int]string)
	for rows.Next() {
		var id int
		var summary string
		if err := rows.Scan(&id, &summary); err == nil {
			results[id] = summary
		}
	}
	return results, nil
}

// 2. Save a float32 vector as a binary blob
// SQLite cannot store []float32 directly, so we convert to []byte
func SaveVector(fileId int, chunkIndex int, vector []float32) error {
	// Convert []float32 -> []byte
	byteBuf := make([]byte, len(vector)*4) // 4 bytes per float32

	// We use LittleEndian (Standard for x64 CPUs)
	// You need "encoding/binary" and "math" imports for this
	for i, v := range vector {
		bits := math.Float32bits(v)
		binary.LittleEndian.PutUint32(byteBuf[i*4:], bits)
	}

	_, err := DB.Exec(`INSERT INTO file_vectors (file_id, chunk_index, vector_blob) VALUES (?, ?, ?)`,
		fileId, chunkIndex, byteBuf)
	return err
}
