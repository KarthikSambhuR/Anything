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
	Path      string
	Snippet   string
	Score     float32
	IconData  string
	Extension string
}

func InitDB(dbPath string) {
	// Use AppData path to ensure write permissions
	realPath := GetDataPath("index.db")

	var err error
	DB, err = sql.Open("sqlite3", realPath)
	if err != nil {
		log.Fatal(err)
	}

	if _, err := DB.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		log.Printf("Failed to enable WAL: %v", err)
	}

	// Main file registry. 'icon_data' stores base64 images.
	_, err = DB.Exec(`
	CREATE TABLE IF NOT EXISTS files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		path TEXT UNIQUE,
		filename TEXT,
		extension TEXT,
		modified_time INTEGER,
		summary TEXT,		
		icon_data TEXT 
	);`)
	if err != nil {
		log.Fatal(err)
	}

	// Full Text Search (FTS5) virtual table
	_, err = DB.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS files_fts USING fts5(filename, summary, path UNINDEXED, content='files', content_rowid='id');`)
	if err != nil {
		log.Fatal(err)
	}

	// Vector storage for AI embeddings
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

	// Caches one icon per extension to reduce DB size (e.g., .pdf -> Base64)
	_, err = DB.Exec(`
	CREATE TABLE IF NOT EXISTS extension_icons (
		extension TEXT PRIMARY KEY,
		icon_data TEXT
	);`)
	if err != nil {
		log.Fatal(err)
	}

	setupTriggers()
}

func setupTriggers() {
	// Automatically sync the FTS table when the main 'files' table changes
	DB.Exec(`CREATE TRIGGER IF NOT EXISTS files_ai AFTER INSERT ON files BEGIN INSERT INTO files_fts(rowid, filename, summary, path) VALUES (new.id, new.filename, new.summary, new.path); END;`)
	DB.Exec(`CREATE TRIGGER IF NOT EXISTS files_ad AFTER DELETE ON files BEGIN INSERT INTO files_fts(files_fts, rowid, filename, summary, path) VALUES('delete', old.id, old.filename, old.summary, old.path); END;`)
	DB.Exec(`CREATE TRIGGER IF NOT EXISTS files_au AFTER UPDATE ON files BEGIN INSERT INTO files_fts(files_fts, rowid, filename, summary, path) VALUES('delete', old.id, old.filename, old.summary, old.path); INSERT INTO files_fts(rowid, filename, summary, path) VALUES (new.id, new.filename, new.summary, new.path); END;`)
}

func GetFilesNeedingEmbedding() (map[int]string, error) {
	query := `SELECT id, summary FROM files WHERE summary IS NOT NULL AND summary != "" AND id NOT IN (SELECT distinct file_id FROM file_vectors)`
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

func SaveVector(fileId int, chunkIndex int, vector []float32) error {
	// Convert []float32 to byte slice for BLOB storage
	byteBuf := make([]byte, len(vector)*4)
	for i, v := range vector {
		bits := math.Float32bits(v)
		binary.LittleEndian.PutUint32(byteBuf[i*4:], bits)
	}
	_, err := DB.Exec(`INSERT INTO file_vectors (file_id, chunk_index, vector_blob) VALUES (?, ?, ?)`, fileId, chunkIndex, byteBuf)
	return err
}

func SearchFiles(queryText string, minTime int64, maxTime int64) ([]SearchResult, error) {
	cleanQuery := queryCleaner.ReplaceAllString(queryText, " ")
	terms := strings.Fields(cleanQuery)
	if len(terms) == 0 {
		return nil, nil
	}

	var contentParts []string
	for _, term := range terms {
		contentParts = append(contentParts, term+"*")
	}
	contentQuery := strings.Join(contentParts, " AND ")

	baseQuery := `
		SELECT f.path, COALESCE(snippet(files_fts, 1, '[', ']', '...', 15), ''), COALESCE(f.icon_data, ''), f.extension, files_fts.rank
		FROM files_fts 
		JOIN files f ON f.id = files_fts.rowid
		WHERE files_fts MATCH ? `

	args := []interface{}{contentQuery}

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
		if err := rows.Scan(&res.Path, &res.Snippet, &res.IconData, &res.Extension, &rank); err != nil {
			return nil, err
		}
		// Invert rank (FTS returns negative for better matches) and scale
		res.Score = float32(math.Abs(float64(rank))) * 1.5
		results = append(results, res)
	}
	return results, nil
}
