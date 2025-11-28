package main

import (
	"Anything/core"
	"flag"
	"fmt"
	"os"
)

func main() {
	// CLI Flags
	scanAll := flag.Bool("scan", false, "Scan all available drives (C:\\, D:\\, etc)")
	searchQuery := flag.String("search", "", "Search term")
	flag.Parse()

	// Initialize DB (Creates index.db if missing)
	core.InitDB("./index.db")

	// MODE 1: Scan All Drives
	if *scanAll {
		drives := core.GetDrives()
		fmt.Println("\nDetected Drives:", drives)

		for _, drive := range drives {
			core.ScanDirectory(drive)
		}
		fmt.Println("\nAll drives indexed successfully!")
		return
	}

	// MODE 2: Search
	if *searchQuery != "" {
		results, err := core.SearchFiles(*searchQuery)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("--- Results ---")
		if len(results) == 0 {
			fmt.Println("No matches.")
		}
		for _, path := range results {
			fmt.Println(path)
		}
		return
	}

	// Default Help
	fmt.Println("Usage:")
	fmt.Println("  go run -tags fts5 main.go -scan         (Indexes all drives)")
	fmt.Println("  go run -tags fts5 main.go -search=\"abc\" (Searches for files)")
}
