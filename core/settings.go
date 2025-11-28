package core

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config Structure
type AppSettings struct {
	// "simple" (First 512 tokens) or "accurate" (Chunking)
	EmbeddingStrategy string `json:"embedding_strategy"`

	// Max vectors per file (Only used in "accurate" mode)
	MaxChunksPerFile int `json:"max_chunks_per_file"`

	// Paths to ignore (e.g. node_modules)
	IgnoredPaths []string `json:"ignored_paths"`

	// File extensions to read content from
	AllowedExtensions []string `json:"allowed_extensions"`
}

var CurrentSettings AppSettings

// Defaults
func getDefaultSettings() AppSettings {
	return AppSettings{
		EmbeddingStrategy: "simple", // Default to fast mode
		MaxChunksPerFile:  15,       // As per your request
		IgnoredPaths: []string{
			"node_modules", ".git", "$RECYCLE.BIN", "System Volume Information",
			"Windows", "Program Files", "Program Files (x86)",
		},
		AllowedExtensions: []string{
			".txt", ".md", ".markdown", ".pdf", ".docx", ".rtf",
		},
	}
}

// Load or Create
func LoadSettings() {
	file, err := os.Open("settings.json")
	if err != nil {
		// File doesn't exist? Create default.
		fmt.Println("Settings file not found. Creating default settings.json...")
		CurrentSettings = getDefaultSettings()
		SaveSettings()
		return
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&CurrentSettings)
	if err != nil {
		fmt.Printf("Error parsing settings: %v. Using defaults.\n", err)
		CurrentSettings = getDefaultSettings()
	}
	fmt.Println("Settings loaded.")
}

func SaveSettings() {
	file, err := os.Create("settings.json")
	if err != nil {
		fmt.Printf("Error saving settings: %v\n", err)
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.Encode(CurrentSettings)
}
