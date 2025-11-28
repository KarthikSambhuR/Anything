package core

import (
	"database/sql"
	"log"
	"regexp"
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

	// Enable Write-Ahead Logging (WAL) for better concurrency
	if _, err := DB.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		log.Printf("Failed to enable WAL mode: %v", err)
	}

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

	// FTS5 "External Content" table.
	// It relies on 'files' for storage (content='files') to save space.
	_, err = DB.Exec(`
	CREATE VIRTUAL TABLE IF NOT EXISTS files_fts USING fts5(filename, summary, path UNINDEXED, content='files', content_rowid='id');
	`)
	if err != nil {
		log.Fatal(err)
	}

	// Triggers to manually sync changes from 'files' to 'files_fts'
	// Required because we are using an External Content FTS table.
	_, err = DB.Exec(`
	CREATE TRIGGER IF NOT EXISTS files_ai AFTER INSERT ON files BEGIN
		INSERT INTO files_fts(rowid, filename, summary, path) VALUES (new.id, new.filename, new.summary, new.path);
	END;`)
	if err != nil {
		log.Fatal(err)
	}

	_, err = DB.Exec(`
	CREATE TRIGGER IF NOT EXISTS files_ad AFTER DELETE ON files BEGIN
		INSERT INTO files_fts(files_fts, rowid, filename, summary, path) VALUES('delete', old.id, old.filename, old.summary, old.path);
	END;`)
	if err != nil {
		log.Fatal(err)
	}

	_, err = DB.Exec(`
	CREATE TRIGGER IF NOT EXISTS files_au AFTER UPDATE ON files BEGIN
		INSERT INTO files_fts(files_fts, rowid, filename, summary, path) VALUES('delete', old.id, old.filename, old.summary, old.path);
		INSERT INTO files_fts(rowid, filename, summary, path) VALUES (new.id, new.filename, new.summary, new.path);
	END;`)
	if err != nil {
		log.Fatal(err)
	}
}

type SearchResult struct {
	Path    string
	Snippet string
}

var queryCleaner = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func SearchFiles(queryText string) ([]SearchResult, error) {
	// Normalize symbols: "my-file.go" -> "my file go" to allow partial matching on parts
	cleanQuery := queryCleaner.ReplaceAllString(queryText, " ")

	terms := strings.Fields(cleanQuery)
	if len(terms) == 0 {
		return nil, nil
	}

	// Construct Prefix Query: "term1* AND term2*"
	var ftsParts []string
	for _, term := range terms {
		ftsParts = append(ftsParts, term+"*")
	}
	ftsQuery := strings.Join(ftsParts, " AND ")

	// snippet(table, col_index, start_tag, end_tag, ellipsis, max_tokens)
	// col_index 1 targets 'summary'
	query := `
		SELECT path, snippet(files_fts, 1, '[', ']', '...', 15) 
		FROM files_fts 
		WHERE files_fts MATCH ? 
		ORDER BY rank 
		LIMIT 20`

	rows, err := DB.Query(query, ftsQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var res SearchResult
		if err := rows.Scan(&res.Path, &res.Snippet); err != nil {
			return nil, err
		}
		results = append(results, res)
	}
	return results, nil
}
