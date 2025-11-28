package core

import (
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

func ScanDirectory(root string) {
	fmt.Printf("\n>>> Starting Scan on Drive: %s\n", root)
	startTime := time.Now()

	// Transaction Variables
	var tx *sql.Tx
	var stmt *sql.Stmt
	var err error
	count := 0
	batchSize := 1000 // Save every 1000 files

	// Helper to start a fresh transaction
	startTx := func() {
		tx, err = DB.Begin()
		if err != nil {
			fmt.Printf("Tx Error: %v\n", err)
			return
		}

		query := `INSERT INTO files (path, filename, extension, modified_time) VALUES (?, ?, ?, ?)
				  ON CONFLICT(path) DO UPDATE SET modified_time = excluded.modified_time;`
		stmt, err = tx.Prepare(query)
		if err != nil {
			fmt.Printf("Stmt Error: %v\n", err)
		}
	}

	// Helper to commit current transaction
	commitTx := func() {
		if stmt != nil {
			stmt.Close()
		}
		if tx != nil {
			tx.Commit()
		}
	}

	// Start the first batch
	startTx()

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		} // Skip denied
		if d.IsDir() {
			return nil
		} // Skip folders

		info, err := d.Info()
		if err != nil {
			return nil
		}

		// Execute Insert (inside transaction)
		if stmt != nil {
			_, err = stmt.Exec(path, d.Name(), filepath.Ext(d.Name()), info.ModTime().Unix())
		}
		count++

		// --- BATCH SAVE ---
		if count%batchSize == 0 {
			commitTx() // Save 1000 files
			startTx()  // Start new batch
			fmt.Printf("\r[Saved: %d] %s                     ", count, truncateString(d.Name(), 30))
		}
		return nil
	})

	// Final Save
	commitTx()

	fmt.Printf("\nFinished drive %s. Indexed %d files in %v\n", root, count, time.Since(startTime))
}

func truncateString(str string, num int) string {
	if len(str) > num {
		return str[0:num] + "..."
	}
	return str
}

// GetDrives detects all available drive letters
func GetDrives() []string {
	var drives []string
	fmt.Println("Checking for available drives...")
	for _, drive := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		path := string(drive) + ":\\"
		fmt.Printf("Checking Drive %s... ", string(drive))
		_, err := os.ReadDir(path)
		if err == nil {
			fmt.Println("FOUND")
			drives = append(drives, path)
		} else {
			fmt.Println("Skipped")
		}
	}
	return drives
}
