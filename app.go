package main

import (
	"Anything/core"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image/png"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Windows API constants
const (
	SW_HIDE       = 0
	SW_SHOWNORMAL = 1
	SW_SHOW       = 5
	SW_RESTORE    = 9
)

// Load DLLs
var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procFindWindowW              = user32.NewProc("FindWindowW")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procShowWindow               = user32.NewProc("ShowWindow")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procAttachThreadInput        = user32.NewProc("AttachThreadInput")
	procGetCurrentThreadId       = kernel32.NewProc("GetCurrentThreadId")
	procIsIconic                 = user32.NewProc("IsIconic")
)

type App struct {
	ctx                  context.Context
	lastForegroundWindow uintptr
}

func NewApp() *App {
	return &App{}
}

func (a *App) GetThumbnail(path string) string {
	// Call the core function we just exported
	img, err := core.GetImageThumbnail(path)
	if err != nil {
		return ""
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return ""
	}

	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	fmt.Println("🚀 Starting Backend Engine...")
	core.InitDB("./index.db")
	core.LoadSettings()
	loadExtIcons()
	core.InitAI()
	core.InitVision()
	core.StartHotkeyListener(core.CurrentSettings.Hotkey, func() {
		// 1. Get Our Window Handle
		ptrTitle, _ := syscall.UTF16PtrFromString("Anything")
		hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(ptrTitle)))

		if hwnd == 0 {
			return
		}

		// 2. Check State
		foreground, _, _ := procGetForegroundWindow.Call()

		if foreground == hwnd {
			// A. We are active -> HIDE and restore previous window

			// First hide our window
			wruntime.WindowHide(a.ctx)

			// Then restore focus to the last window
			if a.lastForegroundWindow != 0 && a.lastForegroundWindow != hwnd {
				procSetForegroundWindow.Call(a.lastForegroundWindow)
			}

		} else {
			// B. We are hidden/background -> SAVE current window and SHOW

			// Save the current foreground window before we take focus
			a.lastForegroundWindow = foreground

			// Step 1: Force Restore if Minimized
			isIconic, _, _ := procIsIconic.Call(hwnd)
			if isIconic != 0 {
				procShowWindow.Call(hwnd, uintptr(SW_RESTORE))
			} else {
				procShowWindow.Call(hwnd, uintptr(SW_SHOW))
			}

			// Step 2: The Focus Stealing Hack (AttachThreadInput)
			var currentThreadId uintptr
			currentThreadId, _, _ = procGetCurrentThreadId.Call()

			var foregroundThreadId uintptr
			foregroundThreadId, _, _ = procGetWindowThreadProcessId.Call(foreground, 0)

			if foregroundThreadId != currentThreadId {
				// Attach our thread to the foreground window's thread
				procAttachThreadInput.Call(foregroundThreadId, currentThreadId, 1) // 1 = True

				// Now we have permission to steal focus
				procSetForegroundWindow.Call(hwnd)

				// Detach immediately
				procAttachThreadInput.Call(foregroundThreadId, currentThreadId, 0) // 0 = False
			} else {
				// Already on same thread (unlikely but possible), just set focus
				procSetForegroundWindow.Call(hwnd)
			}

			// Step 3: Ensure Wails knows we are visible
			wruntime.WindowShow(a.ctx)
		}
	})
	core.InitTokenizer()
	core.LoadVectorIndex()

	// BACKGROUND INDEXING SEQUENCE
	go func() {
		core.RunAppScan() // 1. Apps

		drives := core.GetDrives() // 2. Files
		for _, drive := range drives {
			core.RunQuickScan(drive)
		}

		core.RunIconScan() // 3. Icons
		loadExtIcons()

		fmt.Println("\n--- Starting Content Extraction ---")
		core.RunDeepScan() // 4. Content (Text + OCR)

		fmt.Println("\n--- Starting AI Embedding ---")
		core.RunEmbeddingScan() // 5. AI

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
