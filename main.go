package main

import (
	"Anything/core"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
)

func main() {
	// Colors
	fileColor := color.New(color.FgHiGreen, color.Bold).SprintFunc()
	pathColor := color.New(color.FgWhite, color.Faint).SprintFunc()
	snippetColor := color.New(color.FgYellow).SprintFunc()

	scanAll := flag.Bool("scan", false, "Scan all available drives")
	searchQuery := flag.String("search", "", "Search term")
	flag.Parse()

	// 1. Initialize Core Systems
	core.InitDB("./index.db")
	core.LoadSettings() // <--- NEW: Load Settings
	core.InitAI()       // <--- NEW: Load ONNX
	core.InitTokenizer()
	core.LoadVectorIndex()
	defer core.CloseAI()

	if *scanAll {
		drives := core.GetDrives()
		fmt.Println("Detected Drives:", drives)

		for _, drive := range drives {
			core.RunQuickScan(drive)
		}

		fmt.Println("\n------------------------------------------------")
		fmt.Println("Filenames indexed. Starting content extraction...")
		fmt.Println("------------------------------------------------")

		core.RunDeepScan()

		fmt.Println("\n------------------------------------------------")
		fmt.Println("Content ready. Starting AI Embedding...")
		fmt.Println("------------------------------------------------")
		core.RunEmbeddingScan()

		fmt.Println("\nAll Done.")
		return
	}

	if *searchQuery != "" {
		fmt.Printf("Searching for: %s\n", *searchQuery)

		// USE HYBRID SEARCH
		results, err := core.HybridSearch(*searchQuery)
		if err != nil {
			fmt.Println("Search failed:", err)
			return
		}

		if len(results) == 0 {
			fmt.Println("No matches.")
			return
		}

		for _, res := range results {
			printResult(res, fileColor, pathColor, snippetColor)
		}
	}
}

func printResult(res core.SearchResult, fileColor, pathColor, snippetColor func(a ...interface{}) string) {
	filename := filepath.Base(res.Path)
	dir := filepath.Dir(res.Path)

	fmt.Printf("📄 %s\n", fileColor(filename))
	fmt.Printf("   %s %s\n", color.HiBlackString("└─"), pathColor(dir))

	if res.Snippet != "" {
		// Clean formatting for CLI
		prettySnippet := strings.ReplaceAll(res.Snippet, "\n", " ")
		// Highlight visual brackets if they exist (from Keyword search)
		prettySnippet = strings.ReplaceAll(prettySnippet, "[", "\033[1;31m")
		prettySnippet = strings.ReplaceAll(prettySnippet, "]", "\033[0m\033[33m")

		fmt.Printf("   %s \"%s\"\n", color.HiBlackString("└─ context:"), snippetColor(prettySnippet))
	}
	fmt.Println("")
}
