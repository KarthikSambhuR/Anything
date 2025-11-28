package core

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Indexer Stats
type IndexStats struct {
	TotalScanned int
	Added        int
	Updated      int
	Skipped      int
	Duration     time.Duration
}

// 1. Load ONLY the files for the specific drive we are scanning
func LoadFileMap(driveRoot string) (map[string]int64, error) {
	fmt.Printf("Loading index for %s into RAM... ", driveRoot)
	fileMap := make(map[string]int64)

	// OPTIMIZATION: Only fetch paths starting with the drive letter
	// We use standard SQL wildcard: 'C:\%'
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

// 2. Incremental Scanner
func ScanDirectory(root string) {
	fmt.Printf("\n>>> Starting Smart Scan on: %s\n", root)
	startTime := time.Now()
	stats := IndexStats{}

	// Step A: Load ONLY this drive's index
	existingFiles, err := LoadFileMap(root)
	if err != nil {
		fmt.Printf("Error loading DB map: %v\n", err)
		return
	}

	// Step B: Prepare Transactions
	tx, err := DB.Begin()
	if err != nil {
		fmt.Printf("Tx Error: %v\n", err)
		return
	}

	insertQuery := `INSERT INTO files (path, filename, extension, modified_time) VALUES (?, ?, ?, ?)`
	insertStmt, _ := tx.Prepare(insertQuery)

	updateQuery := `UPDATE files SET modified_time = ? WHERE path = ?`
	updateStmt, _ := tx.Prepare(updateQuery)

	defer insertStmt.Close()
	defer updateStmt.Close()

	batchSize := 1000

	// Step C: Walk the Disk
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		currentModTime := info.ModTime().Unix()
		stats.TotalScanned++

		// --- CHECK RAM MAP ---
		storedModTime, exists := existingFiles[path]

		if exists {
			if storedModTime == currentModTime {
				stats.Skipped++
				return nil // EXACT MATCH -> SKIP
			}
			// Modified? -> Update
			_, err = updateStmt.Exec(currentModTime, path)
			stats.Updated++
		} else {
			// New? -> Insert
			_, err = insertStmt.Exec(path, d.Name(), filepath.Ext(d.Name()), currentModTime)
			stats.Added++
		}

		// Optimization: Remove from map to free RAM as we go (optional, but nice)
		delete(existingFiles, path)

		// Batch Commit
		currentChanges := stats.Added + stats.Updated
		if currentChanges%batchSize == 0 && currentChanges > 0 {
			tx.Commit()
			tx, _ = DB.Begin()
			insertStmt, _ = tx.Prepare(insertQuery)
			updateStmt, _ = tx.Prepare(updateQuery)
			fmt.Printf("\r[Scanned: %d] [New: %d] [Upd: %d] [Skip: %d]", stats.TotalScanned, stats.Added, stats.Updated, stats.Skipped)
		}
		return nil
	})

	tx.Commit() // Final Save
	stats.Duration = time.Since(startTime)

	fmt.Printf("\n--- Scan Complete for %s ---\n", root)
	fmt.Printf("Total Scanned: %d\n", stats.TotalScanned)
	fmt.Printf("Added:         %d\n", stats.Added)
	fmt.Printf("Updated:       %d\n", stats.Updated)
	fmt.Printf("Skipped:       %d\n", stats.Skipped)
	fmt.Printf("Time Taken:    %v\n", stats.Duration)
}

// Helper
func GetDrives() []string {
	var drives []string
	fmt.Println("Checking for available drives...")
	for _, drive := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		path := string(drive) + ":\\"
		_, err := os.ReadDir(path)
		if err == nil {
			drives = append(drives, path)
		}
	}
	return drives
}
