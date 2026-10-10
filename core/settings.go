package core

import (
	"encoding/json"
	"fmt"
	"os"
)

type AppSettings struct {
	// "simple" (First 512 tokens) or "accurate" (Chunking)
	EmbeddingStrategy string `json:"embedding_strategy"`

	// Selected embedding model: "all-minilm-l6-v2" or "embeddinggemma-2-q4"
	EmbeddingModel string `json:"embedding_model"`

	// Max vectors per file (Only used in "accurate" mode)
	MaxChunksPerFile int    `json:"max_chunks_per_file"`
	Hotkey           string `json:"hotkey"`

	IgnoredPaths      []string `json:"ignored_paths"`
	AllowedExtensions []string `json:"allowed_extensions"`
	IndexedDirectories []string `json:"indexed_directories"`
	PriorityDirectories []string `json:"priority_directories"`
}

var CurrentSettings AppSettings

func getDefaultSettings() AppSettings {
	return AppSettings{
		EmbeddingStrategy: "simple",
		EmbeddingModel:    "all-minilm-l6-v2",
		MaxChunksPerFile:  15,
		Hotkey:            "Alt+Space",
		IgnoredPaths: []string{
			"node_modules", ".git", "$RECYCLE.BIN", "System Volume Information",
			"Windows", "Program Files", "Program Files (x86)", "ProgramData",
			"AppData", ".vscode", ".idea", "build", "dist", "out", "target", "bin", "obj", "vendor", ".next", ".cache",
		},
		AllowedExtensions: []string{
			".txt", ".md", ".markdown", ".pdf", ".docx", ".rtf",
			".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".svg",
			".mp4", ".mov", ".avi", ".mkv",
		},
		IndexedDirectories: []string{},
		PriorityDirectories: []string{},
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
