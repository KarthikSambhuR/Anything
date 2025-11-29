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

	// Get and store our window handle once
	ptrTitle, _ := syscall.UTF16PtrFromString("Anything")
	a.ourWindowHandle, _, _ = procFindWindowW.Call(0, uintptr(unsafe.Pointer(ptrTitle)))
	fmt.Printf("📌 Our window handle: %x\n", a.ourWindowHandle)

	core.StartHotkeyListener(core.CurrentSettings.Hotkey, func() {
		// Use stored handle
		hwnd := a.ourWindowHandle
		if hwnd == 0 {
			// Fallback: try to find it again
			hwnd, _, _ = procFindWindowW.Call(0, uintptr(unsafe.Pointer(ptrTitle)))
			if hwnd == 0 {
				return
			}
			a.ourWindowHandle = hwnd
		}

		// 2. Check State
		a.visibilityMutex.Lock()
		visible := a.isWindowVisible
		a.visibilityMutex.Unlock()

		if visible {
			// A. We are active -> HIDE and restore previous window
			a.hideWindow()
		} else {
			// B. We are hidden/background -> SAVE current window and SHOW
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

func (a *App) hideWindow() {
	a.visibilityMutex.Lock()
	defer a.visibilityMutex.Unlock()

	if !a.isWindowVisible {
		return // Already hidden
	}

	fmt.Println("🔒 Hiding window...")

	// Get our window handle
	ptrTitle, _ := syscall.UTF16PtrFromString("Anything")
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(ptrTitle)))

	// First hide our window
	wruntime.WindowHide(a.ctx)
	a.isWindowVisible = false

	// Then restore focus to the last window
	if a.lastForegroundWindow != 0 && a.lastForegroundWindow != hwnd {
		procSetForegroundWindow.Call(a.lastForegroundWindow)
		fmt.Printf("🔄 Restored focus to previous window (handle: %x)\n", a.lastForegroundWindow)
	}
}

func (a *App) showWindow(foreground, hwnd uintptr) {
	a.visibilityMutex.Lock()
	defer a.visibilityMutex.Unlock()

	// Only save if the foreground window is valid and not us
	if foreground != 0 && foreground != hwnd {
		a.lastForegroundWindow = foreground
		fmt.Printf("💾 Saved previous window (handle: %x)\n", foreground)
	}

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

	if foregroundThreadId != currentThreadId && foregroundThreadId != 0 {
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
	a.isWindowVisible = true
	fmt.Println("👁️ Window shown and focused")
}

// Monitor if the window loses focus (user clicked elsewhere)
func (a *App) monitorFocusLoss() {
	lastForeground := uintptr(0)
	lastActive := uintptr(0)

	for {
		time.Sleep(50 * time.Millisecond)

		a.visibilityMutex.Lock()
		visible := a.isWindowVisible
		hwnd := a.ourWindowHandle
		a.visibilityMutex.Unlock()

		if !visible || hwnd == 0 {
			lastForeground = 0
			lastActive = 0
			continue
		}

		// Check both foreground AND active window (important for AlwaysOnTop windows)
		foreground, _, _ := procGetForegroundWindow.Call()
		active, _, _ := procGetActiveWindow.Call()

		// Debug: Log when things change
		if foreground != lastForeground {
			if foreground == hwnd {
				fmt.Printf("✅ Foreground: We have it (handle: %x)\n", hwnd)
			} else if foreground != 0 {
				fmt.Printf("⚠️ Foreground changed to: %x\n", foreground)
			}
			lastForeground = foreground
		}

		if active != lastActive {
			if active == hwnd {
				fmt.Printf("✅ Active: We have it (handle: %x)\n", hwnd)
			} else if active != 0 {
				fmt.Printf("⚠️ Active changed to: %x\n", active)
			} else {
				fmt.Printf("⚠️ Active is now NULL\n")
			}
			lastActive = active
		}

		// If EITHER foreground or active window is not us (and not null), hide
		// For AlwaysOnTop windows, active window becomes null when clicking elsewhere
		if (foreground != 0 && foreground != hwnd) || (active == 0 && lastActive == hwnd) {
			fmt.Println("🔍 Focus lost - hiding window")
			a.hideWindow()
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

func (a *App) OnHide() {
	// Called from frontend when hiding
	a.visibilityMutex.Lock()
	a.isWindowVisible = false
	a.visibilityMutex.Unlock()
	fmt.Println("📞 Frontend requested hide")
}

func (a *App) OpenFile(path string) {
	fmt.Printf("Opening: %s\n", path)

	// NEW: Track usage to boost this item in future searches
	go core.IncrementUsage(path)

	// ... rest of existing execution code ...
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

	// Hide window immediately after launch for better UX
	a.hideWindow()
}
