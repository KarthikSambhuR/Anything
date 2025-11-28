package core

import (
	"os"
	"path/filepath"
)

// GetDataPath returns the path to a file within the user's config directory.
// Windows: C:\Users\Name\AppData\Roaming\Anything\<filename>
func GetDataPath(filename string) string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = "." // Fallback to local
	}

	appDir := filepath.Join(configDir, "Anything")

	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		os.MkdirAll(appDir, 0755)
	}

	return filepath.Join(appDir, filename)
}

// GetDataDir returns the root config directory path.
func GetDataDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "."
	}
	return filepath.Join(configDir, "Anything")
}
