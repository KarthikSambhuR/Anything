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
	"sync"
	"syscall"
	"time"
	"unsafe"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Windows API constants
const (
	SW_HIDE          = 0
	SW_SHOWNORMAL    = 1
	SW_SHOW          = 5
	SW_RESTORE       = 9
	WM_ACTIVATE      = 0x0006
	WA_INACTIVE      = 0
	HWND_TOPMOST     = ^uintptr(0) // -1
	HWND_NOTOPMOST   = ^uintptr(1) // -2
	SWP_NOMOVE       = 0x0002
	SWP_NOSIZE       = 0x0001
	SWP_SHOWWINDOW   = 0x0040
	GWL_EXSTYLE      = -20
	WS_EX_NOACTIVATE = 0x08000000
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
	procSetWindowPos             = user32.NewProc("SetWindowPos")
	procGetWindowLongPtrW        = user32.NewProc("GetWindowLongPtrW")
	procGetActiveWindow          = user32.NewProc("GetActiveWindow")
)

type App struct {
	ctx                  context.Context
	lastForegroundWindow uintptr
	isWindowVisible      bool
	visibilityMutex      sync.Mutex
	ourWindowHandle      uintptr
	ignoreFocusLoss      bool        // NEW: Flag to temporarily ignore focus loss
	focusLossTimer       *time.Timer // NEW: Timer for delayed focus loss detection
}

func NewApp() *App {
	return &App{}
}

func (a *App) GetThumbnail(path string) string {
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
	core.SetContext(ctx)

	fmt.Println("🚀 Starting Backend Engine...")
	core.InitDB("./index.db")
	core.LoadSettings()
	loadExtIcons()
	core.InitAI()
	core.InitVision()

	// Get and store our window handle once
	ptrTitle, _ := syscall.UTF16PtrFromString("Anything")
	a.ourWindowHandle, _, _ = procFindWindowW.Call(0, uintptr(unsafe.Pointer(ptrTitle)))
	fmt.Printf("🔌 Our window handle: %x\n", a.ourWindowHandle)

	core.StartHotkeyListener(core.CurrentSettings.Hotkey, func() {
		hwnd := a.ourWindowHandle
		if hwnd == 0 {
			hwnd, _, _ = procFindWindowW.Call(0, uintptr(unsafe.Pointer(ptrTitle)))
			if hwnd == 0 {
				return
			}
			a.ourWindowHandle = hwnd
		}

		a.visibilityMutex.Lock()
		visible := a.isWindowVisible
		a.visibilityMutex.Unlock()

		if visible {
			fmt.Println("🔑 Hotkey pressed - HIDING window")
			a.hideWindow()
		} else {
			fmt.Println("🔑 Hotkey pressed - SHOWING window")
			foreground, _, _ := procGetForegroundWindow.Call()
			a.showWindow(foreground, hwnd)
		}
	})

	// Start monitoring for focus loss
	go a.monitorFocusLoss()

	core.InitTokenizer()
	core.LoadVectorIndex()

	// BACKGROUND INDEXING SEQUENCE
	go func() {
		core.RunAppScan()

		drives := core.GetDrives()
		for _, drive := range drives {
			core.RunQuickScan(drive)
		}

		core.RunIconScan()
		loadExtIcons()

		fmt.Println("\n--- Starting Content Extraction ---")
		core.RunDeepScan()

		fmt.Println("\n--- Starting AI Embedding ---")
		core.RunEmbeddingScan()

		core.LoadVectorIndex()
		fmt.Println("\nAll Done. Ready.")
	}()
}

func (a *App) OpenSettings() {
	wruntime.WindowSetSize(a.ctx, 900, 600)
	wruntime.WindowCenter(a.ctx)
	wruntime.WindowShow(a.ctx)
	wruntime.EventsEmit(a.ctx, "settings:open")

	a.visibilityMutex.Lock()
	a.isWindowVisible = true
	a.ignoreFocusLoss = true // Ignore focus loss temporarily
	a.visibilityMutex.Unlock()

	// Re-enable focus loss detection after 500ms
	time.AfterFunc(500*time.Millisecond, func() {
		a.visibilityMutex.Lock()
		a.ignoreFocusLoss = false
		a.visibilityMutex.Unlock()
		fmt.Println("✅ Focus loss detection enabled")
	})
}

func (a *App) CloseSettings() {
	wruntime.WindowSetSize(a.ctx, 700, 60)
	wruntime.WindowCenter(a.ctx)
	wruntime.WindowHide(a.ctx)

	a.visibilityMutex.Lock()
	a.isWindowVisible = false
	a.ignoreFocusLoss = false
	a.visibilityMutex.Unlock()
}

func (a *App) GetSettings() core.AppSettings {
	return core.CurrentSettings
}

func (a *App) RebuildIndex() {
	go func() {
		core.EmitProgress("indexing", "Starting Full Rebuild...", 0)

		drives := core.GetDrives()
		for i, drive := range drives {
			pct := (i * 100) / len(drives)
			core.EmitProgress("indexing", fmt.Sprintf("Scanning %s...", drive), pct)
			core.RunQuickScan(drive)
		}

		core.EmitProgress("indexing", "Extracting Content...", 50)
		core.RunDeepScan()

		core.EmitProgress("indexing", "Complete", 100)
	}()
}

func (a *App) DownloadModels() {
	go func() {
		core.EmitProgress("download", "Checking Models...", 0)
		core.InitAI()
		core.InitVision()
		core.EmitProgress("download", "All Models Ready", 100)
	}()
}

func (a *App) SaveSettings(s core.AppSettings) {
	core.CurrentSettings = s
	core.SaveSettings()
}

func (a *App) hideWindow() {
	a.visibilityMutex.Lock()
	defer a.visibilityMutex.Unlock()

	if !a.isWindowVisible {
		return
	}

	fmt.Println("🔒 Hiding window...")

	// 1. Hide the window first
	wruntime.WindowHide(a.ctx)
	a.isWindowVisible = false
	a.ignoreFocusLoss = false

	// 2. THE FIX: Reset Geometry while hidden
	// This ensures next time it opens, it's already the correct size and centered.
	wruntime.WindowSetSize(a.ctx, 700, 60)
	wruntime.WindowCenter(a.ctx)

	// Cancel any pending focus loss timer
	if a.focusLossTimer != nil {
		a.focusLossTimer.Stop()
		a.focusLossTimer = nil
	}

	// 3. Restore Focus
	ptrTitle, _ := syscall.UTF16PtrFromString("Anything")
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(ptrTitle)))

	if a.lastForegroundWindow != 0 && a.lastForegroundWindow != hwnd {
		procSetForegroundWindow.Call(a.lastForegroundWindow)
		fmt.Printf("🔄 Restored focus to previous window (handle: %x)\n", a.lastForegroundWindow)
	}
}

func (a *App) showWindow(foreground, hwnd uintptr) {
	a.visibilityMutex.Lock()

	fmt.Printf("📖 showWindow called - foreground: %x, hwnd: %x\n", foreground, hwnd)

	// Save the last foreground window (but not if it's us or null)
	if foreground != 0 && foreground != hwnd {
		a.lastForegroundWindow = foreground
		fmt.Printf("💾 Saved last foreground window: %x\n", foreground)
	} else {
		fmt.Printf("⚠️ Not saving foreground (same as us or null)\n")
	}

	// Set flags BEFORE showing to prevent race condition
	a.isWindowVisible = true
	a.ignoreFocusLoss = true

	a.visibilityMutex.Unlock()

	// Force restore if minimized
	isIconic, _, _ := procIsIconic.Call(hwnd)
	if isIconic != 0 {
		fmt.Println("↗️ Window was minimized, restoring...")
		procShowWindow.Call(hwnd, uintptr(SW_RESTORE))
	} else {
		procShowWindow.Call(hwnd, uintptr(SW_SHOW))
	}

	// Focus stealing hack
	var currentThreadId uintptr
	currentThreadId, _, _ = procGetCurrentThreadId.Call()
	var foregroundThreadId uintptr
	foregroundThreadId, _, _ = procGetWindowThreadProcessId.Call(foreground, 0)

	if foregroundThreadId != currentThreadId && foregroundThreadId != 0 {
		procAttachThreadInput.Call(foregroundThreadId, currentThreadId, 1)
		procSetForegroundWindow.Call(hwnd)
		procAttachThreadInput.Call(foregroundThreadId, currentThreadId, 0)
	} else {
		procSetForegroundWindow.Call(hwnd)
	}

	// Show window & emit reset signal
	wruntime.WindowShow(a.ctx)

	fmt.Println("✨ Window shown, starting grace period...")

	// Small delay before emitting reset to ensure window is ready
	time.Sleep(50 * time.Millisecond)
	wruntime.EventsEmit(a.ctx, "window:reset")

	// Re-enable focus loss detection after delay
	time.AfterFunc(800*time.Millisecond, func() {
		a.visibilityMutex.Lock()
		defer a.visibilityMutex.Unlock()

		// Only enable if window is still visible
		if a.isWindowVisible {
			a.ignoreFocusLoss = false
			fmt.Println("✅ Grace period ended - focus loss detection enabled")

			// Log current state for debugging
			fg, _, _ := procGetForegroundWindow.Call()
			fmt.Printf("📊 Current foreground after grace: %x (ours: %x)\n", fg, hwnd)
		}
	})
}

func (a *App) monitorFocusLoss() {
	lastForeground := uintptr(0)
	lastActive := uintptr(0)
	consecutiveLosses := 0 // NEW: Count consecutive focus losses

	for {
		time.Sleep(100 * time.Millisecond) // Increased from 50ms to 100ms

		a.visibilityMutex.Lock()
		visible := a.isWindowVisible
		hwnd := a.ourWindowHandle
		ignore := a.ignoreFocusLoss
		a.visibilityMutex.Unlock()

		// Reset counter when not visible
		if !visible || hwnd == 0 {
			lastForeground = 0
			lastActive = 0
			consecutiveLosses = 0
			continue
		}

		// Skip monitoring if we're ignoring focus loss
		if ignore {
			lastForeground = 0
			lastActive = 0
			consecutiveLosses = 0
			continue
		}

		// Check focus state
		foreground, _, _ := procGetForegroundWindow.Call()
		active, _, _ := procGetActiveWindow.Call()

		// Debug logging (only on changes)
		if foreground != lastForeground {
			if foreground == hwnd {
				fmt.Printf("✅ Foreground: We have it (handle: %x)\n", hwnd)
				consecutiveLosses = 0
			} else if foreground != 0 {
				fmt.Printf("⚠️ Foreground changed to: %x\n", foreground)
			}
			lastForeground = foreground
		}

		if active != lastActive {
			if active == hwnd {
				fmt.Printf("✅ Active: We have it (handle: %x)\n", hwnd)
				consecutiveLosses = 0
			} else if active != 0 {
				fmt.Printf("⚠️ Active changed to: %x\n", active)
			} else {
				fmt.Printf("⚠️ Active is now NULL\n")
			}
			lastActive = active
		}

		// Check if we lost focus
		lostFocus := false
		if foreground != 0 && foreground != hwnd {
			lostFocus = true
		} else if active == 0 && lastActive == hwnd {
			lostFocus = true
		}

		if lostFocus {
			consecutiveLosses++
			fmt.Printf("⚠️ Focus loss detected (count: %d)\n", consecutiveLosses)

			// Only hide after 3 consecutive detections (300ms total)
			// This prevents false positives from window activation timing
			if consecutiveLosses >= 3 {
				fmt.Println("🔒 Focus confirmed lost - hiding window")
				a.hideWindow()
				consecutiveLosses = 0
			}
		} else {
			// Reset counter if we have focus
			if consecutiveLosses > 0 {
				fmt.Println("✅ Focus regained, resetting counter")
			}
			consecutiveLosses = 0
		}
	}
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

	results, _ := core.HybridSearch(query)

	lowerQ := strings.ToLower(query)
	if strings.Contains("settings", lowerQ) || strings.Contains("config", lowerQ) {
		settingsRes := core.SearchResult{
			Path:      "anything://settings",
			Snippet:   "Configure AI, Indexing, and Hotkeys",
			Score:     1000.0,
			IconData:  "",
			Extension: ".settings",
		}
		results = append([]core.SearchResult{settingsRes}, results...)
	}

	for i := range results {
		if results[i].IconData != "" {
			continue
		}
		ext := strings.ToLower(results[i].Extension)
		if icon, ok := extIconCache[ext]; ok {
			results[i].IconData = icon
		}
	}
	return results
}

func (a *App) OpenFile(path string) {
	if path == "anything://settings" {
		a.OpenSettings()
		return
	}

	fmt.Printf("Opening: %s\n", path)
	go core.IncrementUsage(path)

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

	a.hideWindow()
}

func (a *App) OnHide() {
	a.visibilityMutex.Lock()
	a.isWindowVisible = false
	a.ignoreFocusLoss = false
	a.visibilityMutex.Unlock()
	fmt.Println("📞 Frontend requested hide")
}
