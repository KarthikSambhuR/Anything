package main

import (
	"Anything/core"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
)

func main() {
	// UI Style Definitions
	fileColor := color.New(color.FgHiGreen, color.Bold).SprintFunc()
	pathColor := color.New(color.FgWhite, color.Faint).SprintFunc()
	snippetColor := color.New(color.FgYellow).SprintFunc()

	scanAll := flag.Bool("scan", false, "Scan all available drives")
	searchQuery := flag.String("search", "", "Search term")
	flag.Parse()

	core.InitDB("./index.db")

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

		fmt.Println("\nAll Done.")
		return
	}

	if *searchQuery != "" {
		results, err := core.SearchFiles(*searchQuery)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("")
		if len(results) == 0 {
			fmt.Println("No matches found.")
		}

		for _, res := range results {
			filename := filepath.Base(res.Path)
			dir := filepath.Dir(res.Path)

			fmt.Printf("📄 %s\n", fileColor(filename))
			fmt.Printf("   %s %s\n", color.HiBlackString("└─"), pathColor(dir))

			if res.Snippet != "" {
				// SQLite matches are wrapped in [ ]. Replace brackets with ANSI codes
				// to highlight the keyword in Red within the Yellow snippet.
				coloredSnippet := strings.ReplaceAll(res.Snippet, "[", "\033[1;31m")        // Start Red
				coloredSnippet = strings.ReplaceAll(coloredSnippet, "]", "\033[0m\033[33m") // Reset, then back to Yellow

				fmt.Printf("   %s \"%s\"\n", color.HiBlackString("└─ match:"), snippetColor(coloredSnippet))
			}
			fmt.Println("")
		}
		return
	}

	fmt.Println("Usage: Anything.exe -scan")
}
