package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"
)

type AppRecord struct {
	Name        string
	DisplayName string
	Path        string
	IconData    string
	Publisher   string
	Version     string
}

func getStartMenuShortcuts() map[string]string {
	shortcuts := make(map[string]string)
	if runtime.GOOS != "windows" {
		return shortcuts
	}

	roots := []string{
		filepath.Join(os.Getenv("ProgramData"), "Microsoft", "Windows", "Start Menu", "Programs"),
		filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs"),
	}

	junkWords := []string{"uninstall", "readme", "help", "documentation", "license", "release notes", "website", "support", "manual", "diagnostics"}

	for _, root := range roots {
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}

		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}

			if strings.EqualFold(filepath.Ext(d.Name()), ".lnk") {
				rawName := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
				lower := strings.ToLower(strings.TrimSpace(rawName))

				isJunk := false
				for _, j := range junkWords {
					if strings.Contains(lower, j) {
						isJunk = true
						break
					}
				}

				if !isJunk {
					shortcuts[lower] = path
				}
			}
			return nil
		})
	}

	return shortcuts
}

func cleanRegistryPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, "\"")
	if idx := strings.Index(p, ","); idx != -1 {
		p = strings.TrimSpace(p[:idx])
	}
	return strings.Trim(p, "\"")
}

func scanRegistryApps(shortcuts map[string]string) []AppRecord {
	var apps []AppRecord
	if runtime.GOOS != "windows" {
		return apps
	}

	regPaths := []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `Software\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.LOCAL_MACHINE, `Software\Wow6432Node\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Uninstall`},
	}

	for _, rp := range regPaths {
		k, err := registry.OpenKey(rp.root, rp.path, registry.READ)
		if err != nil {
			continue
		}

		subkeys, err := k.ReadSubKeyNames(-1)
		if err != nil {
			k.Close()
			continue
		}

		for _, subkeyName := range subkeys {
			subKey, err := registry.OpenKey(rp.root, rp.path+`\`+subkeyName, registry.READ)
			if err != nil {
				continue
			}

			dispName, _, err := subKey.GetStringValue("DisplayName")
			if err != nil || strings.TrimSpace(dispName) == "" {
				subKey.Close()
				continue
			}

			dispName = strings.TrimSpace(dispName)

			// Windows Settings filter: SystemComponent == 1
			sysComp, _, err := subKey.GetIntegerValue("SystemComponent")
			if err == nil && sysComp == 1 {
				subKey.Close()
				continue
			}

			// Windows Settings filter: ParentKeyName != ""
			parentKey, _, err := subKey.GetStringValue("ParentKeyName")
			if err == nil && strings.TrimSpace(parentKey) != "" {
				subKey.Close()
				continue
			}

			// Skip Windows Update hotfixes (e.g., KB5001234)
			if strings.HasPrefix(dispName, "KB") && len(dispName) > 2 && dispName[2] >= '0' && dispName[2] <= '9' {
				subKey.Close()
				continue
			}

			dispVer, _, _ := subKey.GetStringValue("DisplayVersion")
			pub, _, _ := subKey.GetStringValue("Publisher")
			icon, _, _ := subKey.GetStringValue("DisplayIcon")
			loc, _, _ := subKey.GetStringValue("InstallLocation")

			subKey.Close()

			var launchPath string
			var iconPath string

			lowerName := strings.ToLower(dispName)

			// 1. Try matching Start Menu shortcut
			if lnk, ok := shortcuts[lowerName]; ok {
				launchPath = lnk
				iconPath = lnk
			} else {
				for sName, sPath := range shortcuts {
					if strings.HasPrefix(lowerName, sName) || strings.HasPrefix(sName, lowerName) {
						launchPath = sPath
						iconPath = sPath
						break
					}
				}
			}

			// 2. Fall back to DisplayIcon if executable
			if launchPath == "" && icon != "" {
				cleaned := cleanRegistryPath(icon)
				if strings.EqualFold(filepath.Ext(cleaned), ".exe") {
					if _, err := os.Stat(cleaned); err == nil {
						launchPath = cleaned
						iconPath = cleaned
					}
				}
			}

			// 3. Fall back to InstallLocation
			if launchPath == "" && loc != "" {
				loc = cleanRegistryPath(loc)
				if entries, err := os.ReadDir(loc); err == nil {
					for _, entry := range entries {
						if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".exe") {
							eName := strings.ToLower(entry.Name())
							if !strings.Contains(eName, "unins") && !strings.Contains(eName, "helper") && !strings.Contains(eName, "crash") && !strings.Contains(eName, "update") {
								launchPath = filepath.Join(loc, entry.Name())
								iconPath = launchPath
								break
							}
						}
					}
				}
			}

			// If iconPath is still empty, try cleaned DisplayIcon even if not an exe (e.g. .ico)
			if iconPath == "" && icon != "" {
				cleaned := cleanRegistryPath(icon)
				if _, err := os.Stat(cleaned); err == nil {
					iconPath = cleaned
				}
			}

			if launchPath == "" {
				launchPath = iconPath
			}

			var iconData string
			if iconPath != "" {
				iconData = GetAppIconBase64(iconPath)
			}

			apps = append(apps, AppRecord{
				Name:        dispName,
				DisplayName: dispName,
				Path:        launchPath,
				IconData:    iconData,
				Publisher:   pub,
				Version:     dispVer,
			})
		}
		k.Close()
	}

	return apps
}

func getStoreApps() []AppRecord {
	var apps []AppRecord
	if runtime.GOOS != "windows" {
		return apps
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", "Get-StartApps | Where-Object { $_.AppID -match '!' } | Select-Object Name, AppID | ConvertTo-Json")
	out, err := cmd.Output()
	if err != nil {
		return apps
	}

	type StartAppJSON struct {
		Name  string `json:"Name"`
		AppID string `json:"AppID"`
	}

	var parsed []StartAppJSON
	if err := json.Unmarshal(out, &parsed); err != nil {
		var single StartAppJSON
		if err2 := json.Unmarshal(out, &single); err2 == nil {
			parsed = []StartAppJSON{single}
		}
	}

	for _, sa := range parsed {
		name := strings.TrimSpace(sa.Name)
		if name == "" {
			continue
		}

		apps = append(apps, AppRecord{
			Name:        name,
			DisplayName: name,
			Path:        "shell:AppsFolder\\" + sa.AppID,
			IconData:    "", // Frontend renders Windows icon placeholder
			Publisher:   "Windows App",
			Version:     "",
		})
	}

	return apps
}

func RunAppScan() {
	fmt.Println("\n>>> Scanning for Installed Applications...")
	startTime := time.Now()

	shortcuts := getStartMenuShortcuts()
	regApps := scanRegistryApps(shortcuts)
	storeApps := getStoreApps()

	seen := make(map[string]bool)
	var allApps []AppRecord

	for _, a := range regApps {
		k := strings.ToLower(strings.TrimSpace(a.DisplayName))
		if !seen[k] {
			seen[k] = true
			allApps = append(allApps, a)
		}
	}

	for _, sa := range storeApps {
		k := strings.ToLower(strings.TrimSpace(sa.DisplayName))
		if !seen[k] {
			seen[k] = true
			allApps = append(allApps, sa)
		}
	}

	tx, err := DB.Begin()
	if err != nil {
		fmt.Printf("Error starting transaction: %v\n", err)
		return
	}

	// Reset installed_apps so stale/uninstalled apps are purged
	_, _ = tx.Exec("DELETE FROM installed_apps")

	stmt, err := tx.Prepare(`INSERT INTO installed_apps (name, display_name, path, icon_data, publisher, version) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		fmt.Printf("❌ Error preparing insert for installed_apps: %v\n", err)
		tx.Rollback()
		return
	}
	defer stmt.Close()

	count := 0
	for _, app := range allApps {
		_, err := stmt.Exec(app.Name, app.DisplayName, app.Path, app.IconData, app.Publisher, app.Version)
		if err == nil {
			count++
		}
	}

	tx.Commit()
	fmt.Printf("✅ Checked installed applications: Indexed %d apps in %v\n", count, time.Since(startTime))
}
