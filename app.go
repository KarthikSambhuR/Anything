package main

import (
	"Anything/core"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
)

type App struct {
	ctx context.Context
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	fmt.Println("🚀 Starting Backend Engine...")
	core.InitDB("./index.db")
	core.LoadSettings()
	loadExtIcons()
	core.InitAI()
	core.InitTokenizer()
	core.LoadVectorIndex()

	// BACKGROUND INDEXING SEQUENCE
	go func() {
		// 1. Apps (High Priority)
		core.RunAppScan()

		// 2. Files
		// Must find files first to populate known extensions for the icon scan
		drives := core.GetDrives()
		for _, drive := range drives {
			core.RunQuickScan(drive)
		}

		// 3. Icons
		// Scans based on extensions found in Step 2
		core.RunIconScan()

		// CRITICAL: Reload cache so Search() can use new icons immediately
		loadExtIcons()

		// 4. Content Extraction
		fmt.Println("\n--- Starting Content Extraction ---")
		core.RunDeepScan()

		// 5. AI Embeddings
		fmt.Println("\n--- Starting AI Embedding ---")
		core.RunEmbeddingScan()

		core.LoadVectorIndex()
		fmt.Println("\nAll Done. Ready.")
	}()
}

func (a *App) shutdown(ctx context.Context) {
	core.CloseAI()
}

var extIconCache = make(map[string]string)

func loadExtIcons() {
	rows, err := core.DB.Query("SELECT extension, icon_data FROM extension_icons")
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var ext, data string
		if err := rows.Scan(&ext, &data); err == nil {
			extIconCache[ext] = data
		}
	}
}

func (a *App) Search(query string) []core.SearchResult {
	if query == "" {
		return []core.SearchResult{}
	}

	results, err := core.HybridSearch(query)
	if err != nil {
		return []core.SearchResult{}
	}

	for i := range results {
		// Case A: Specific Icon exists (e.g. Apps)
		if results[i].IconData != "" {
			continue
		}

		// Case B: Fallback to Extension Icon
		ext := strings.ToLower(results[i].Extension)
		if icon, ok := extIconCache[ext]; ok {
			results[i].IconData = icon
		}
	}
	return results
}

func (a *App) OpenFile(path string) {
	fmt.Printf("Opening: %s\n", path)
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "start", "", path)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	} else if runtime.GOOS == "darwin" {
		cmd = exec.Command("open", path)
	} else {
		cmd = exec.Command("xdg-open", path)
	}
	cmd.Start()
}
