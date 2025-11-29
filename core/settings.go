package core

import (
	"encoding/json"
	"fmt"
	"os"
)

type AppSettings struct {
	// "simple" (First 512 tokens) or "accurate" (Chunking)
	EmbeddingStrategy string `json:"embedding_strategy"`

	// Max vectors per file (Only used in "accurate" mode)
	MaxChunksPerFile int    `json:"max_chunks_per_file"`
	Hotkey           string `json:"hotkey"`

	IgnoredPaths      []string `json:"ignored_paths"`
	AllowedExtensions []string `json:"allowed_extensions"`
}

var CurrentSettings AppSettings

func getDefaultSettings() AppSettings {
	return AppSettings{
		EmbeddingStrategy: "simple",
		MaxChunksPerFile:  15,
		Hotkey:            "Alt+Space",
		IgnoredPaths: []string{
			"node_modules", ".git", "$RECYCLE.BIN", "System Volume Information",
			"Windows", "Program Files", "Program Files (x86)",
		},
		AllowedExtensions: []string{
			".txt", ".md", ".markdown", ".pdf", ".docx", ".rtf",
		},
	}
}

func LoadSettings() {
	path := GetDataPath("settings.json")
	file, err := os.Open(path)
	if err != nil {
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
	path := GetDataPath("settings.json")
	file, err := os.Create(path)
	if err != nil {
		fmt.Printf("Error saving settings: %v\n", err)
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.Encode(CurrentSettings)
}
