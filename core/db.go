package core

import (
	"database/sql"
	"log"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func InitDB(dbPath string) {
	var err error
	DB, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal(err)
	}

	// --- OPTIMIZATION: Enable WAL Mode ---
	// Allows reading (Searching) while writing (Indexing)
	if _, err := DB.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		log.Printf("Failed to enable WAL mode: %v", err)
	}

	// 1. Main Data Table
	_, err = DB.Exec(`
	CREATE TABLE IF NOT EXISTS files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		path TEXT UNIQUE,
		filename TEXT,
		extension TEXT,
		modified_time INTEGER
	);`)
	if err != nil {
		log.Fatal(err)
	}

	// 2. FTS5 Virtual Table (The Speed Layer)
	_, err = DB.Exec(`
	CREATE VIRTUAL TABLE IF NOT EXISTS files_fts USING fts5(filename, path UNINDEXED, content='files', content_rowid='id');
	`)
	if err != nil {
		log.Fatal(err)
	}

	// 3. Triggers (Keep FTS in sync automatically)
	_, err = DB.Exec(`
	CREATE TRIGGER IF NOT EXISTS files_ai AFTER INSERT ON files BEGIN
		INSERT INTO files_fts(rowid, filename, path) VALUES (new.id, new.filename, new.path);
	END;`)
	if err != nil {
		log.Fatal(err)
	}

	_, err = DB.Exec(`
	CREATE TRIGGER IF NOT EXISTS files_ad AFTER DELETE ON files BEGIN
		INSERT INTO files_fts(files_fts, rowid, filename, path) VALUES('delete', old.id, old.filename, old.path);
	END;`)
	if err != nil {
		log.Fatal(err)
	}

	_, err = DB.Exec(`
	CREATE TRIGGER IF NOT EXISTS files_au AFTER UPDATE ON files BEGIN
		INSERT INTO files_fts(files_fts, rowid, filename, path) VALUES('delete', old.id, old.filename, old.path);
		INSERT INTO files_fts(rowid, filename, path) VALUES (new.id, new.filename, new.path);
	END;`)
	if err != nil {
		log.Fatal(err)
	}
}

// SearchFiles using FTS5 (Blazingly Fast)
func SearchFiles(queryText string) ([]string, error) {
	cleanQuery := strings.ReplaceAll(queryText, "\"", "")
	cleanQuery = strings.ReplaceAll(cleanQuery, "'", "")

	terms := strings.Fields(cleanQuery)
	if len(terms) == 0 {
		return nil, nil
	}

	// Build FTS Query: "private mal" -> "private* AND mal*"
	var ftsParts []string
	for _, term := range terms {
		ftsParts = append(ftsParts, term+"*")
	}
	ftsQuery := strings.Join(ftsParts, " AND ")

	rows, err := DB.Query("SELECT path FROM files_fts WHERE filename MATCH ? ORDER BY rank LIMIT 20", ftsQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		results = append(results, path)
	}
	return results, nil
}
