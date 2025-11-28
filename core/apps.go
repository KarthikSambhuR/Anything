package core

import (
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func getAppPaths() []string {
	var paths []string
	if runtime.GOOS == "windows" {
		programData := os.Getenv("ProgramData")
		if programData != "" {
			paths = append(paths, filepath.Join(programData, "Microsoft", "Windows", "Start Menu", "Programs"))
		}
		appData := os.Getenv("APPDATA")
		if appData != "" {
			paths = append(paths, filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs"))
		}
	}
	return paths
}

func RunAppScan() {
	fmt.Println("\n>>> Scanning for Installed Applications...")

	appPaths := getAppPaths()
	if len(appPaths) == 0 {
		return
	}

	tx, err := DB.Begin()
	if err != nil {
		fmt.Printf("Error starting transaction: %v\n", err)
		return
	}

	insertQuery := `INSERT INTO files (path, filename, extension, modified_time, summary, icon_data) VALUES (?, ?, ?, ?, ?, ?)`
	insertStmt, err := tx.Prepare(insertQuery)
	if err != nil {
		fmt.Printf("❌ Error preparing insert: %v\n(Hint: Delete index.db to reset schema)\n", err)
		tx.Rollback()
		return
	}
	defer insertStmt.Close()

	updateQuery := `UPDATE files SET modified_time = ?, summary = ?, icon_data = ? WHERE path = ?`
	updateStmt, err := tx.Prepare(updateQuery)
	if err != nil {
		fmt.Printf("❌ Error preparing update: %v\n", err)
		tx.Rollback()
		return
	}
	defer updateStmt.Close()

	count := 0

	for _, root := range appPaths {
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}

			if !d.IsDir() {
				ext := strings.ToLower(filepath.Ext(d.Name()))
				if ext == ".lnk" || ext == ".exe" || ext == ".url" {

					info, err := d.Info()
					if err != nil {
						return nil
					}
					currentModTime := info.ModTime().Unix()

					cleanName := strings.TrimSuffix(d.Name(), ext)
					appSummary := cleanName + " Application"

					// Extract Base64 Icon from binary/shortcut
					iconData := GetAppIconBase64(path)

					var storedTime int64
					err = DB.QueryRow("SELECT modified_time FROM files WHERE path = ?", path).Scan(&storedTime)

					if err == sql.ErrNoRows {
						_, err = insertStmt.Exec(path, d.Name(), ext, currentModTime, appSummary, iconData)
						if err == nil {
							count++
						}
					} else {
						_, err = updateStmt.Exec(currentModTime, appSummary, iconData, path)
					}
				}
			}
			return nil
		})
	}

	tx.Commit()
	fmt.Printf("Checked applications. Added %d new apps.\n", count)
}
