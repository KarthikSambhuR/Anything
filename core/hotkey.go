package core

import (
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32Lib            = syscall.NewLazyDLL("user32.dll")
	procRegisterHotKey   = user32Lib.NewProc("RegisterHotKey")
	procPeekMessageW     = user32Lib.NewProc("PeekMessageW")
	procTranslateMessage = user32Lib.NewProc("TranslateMessage")
	procDispatchMessageW = user32Lib.NewProc("DispatchMessageW")
	procUnregisterHotKey = user32Lib.NewProc("UnregisterHotKey")
)

const (
	MOD_ALT     = 0x0001
	MOD_CONTROL = 0x0002
	MOD_SHIFT   = 0x0004
	MOD_WIN     = 0x0008
	WM_HOTKEY   = 0x0312
	PM_REMOVE   = 0x0001
)

var keyMap = map[string]uintptr{
	"SPACE": 0x20, "ENTER": 0x0D, "ESC": 0x1B, "TAB": 0x09,
	"A": 0x41, "B": 0x42, "C": 0x43, "D": 0x44, "E": 0x45, "F": 0x46,
	"G": 0x47, "H": 0x48, "I": 0x49, "J": 0x4A, "K": 0x4B, "L": 0x4C,
	"M": 0x4D, "N": 0x4E, "O": 0x4F, "P": 0x50, "Q": 0x51, "R": 0x52,
	"S": 0x53, "T": 0x54, "U": 0x55, "V": 0x56, "W": 0x57, "X": 0x58,
	"Y": 0x59, "Z": 0x5A,
	"F1": 0x70, "F2": 0x71, "F3": 0x72, "F4": 0x73, "F5": 0x74,
	"F6": 0x75, "F7": 0x76, "F8": 0x77, "F9": 0x78, "F10": 0x79,
	"F11": 0x7A, "F12": 0x7B,
}

func parseHotkey(shortcut string) (uintptr, uintptr) {
	parts := strings.Split(strings.ToUpper(shortcut), "+")
	var mods uintptr
	var key uintptr

	for _, p := range parts {
		p = strings.TrimSpace(p)
		switch p {
		case "ALT":
			mods |= MOD_ALT
		case "CTRL", "CONTROL":
			mods |= MOD_CONTROL
		case "SHIFT":
			mods |= MOD_SHIFT
		case "WIN", "CMD":
			mods |= MOD_WIN
		default:
			if k, ok := keyMap[p]; ok {
				key = k
			}
		}
	}
	return mods, key
}

var lastForegroundWindow uintptr

func StartHotkeyListener(shortcut string, callback func()) {
	mods, key := parseHotkey(shortcut)
	if key == 0 {
		fmt.Println("❌ [Hotkey] Invalid Config:", shortcut)
		return
	}

	go func() {
		// CRITICAL: Ensure this Goroutine stays on the same OS Thread
		// Windows Message Queues are thread-local.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		// ID 1, hWnd 0 (Thread-Specific Hotkey)
		r1, _, err := procRegisterHotKey.Call(0, 1, mods, key)
		if r1 == 0 {
			fmt.Printf("❌ [Hotkey] Failed to register '%s'. Error: %v\n", shortcut, err)
			fmt.Println("   (Is another app using this shortcut?)")
			return
		}
		defer procUnregisterHotKey.Call(0, 1)

		fmt.Printf("✅ [Hotkey] Listening for: %s\n", shortcut)

		// Message Loop with PeekMessage (non-blocking)
		var msg struct {
			Hwnd    uintptr
			Message uint32
			WParam  uintptr
			LParam  uintptr
			Time    uint32
			Pt      struct{ X, Y int32 }
		}

		for {
			// PeekMessage is non-blocking, checks if message exists
			ret, _, _ := procPeekMessageW.Call(
				uintptr(unsafe.Pointer(&msg)),
				0,
				0,
				0,
				PM_REMOVE,
			)

			if ret != 0 {
				// Message exists, process it
				if msg.Message == WM_HOTKEY {
					fmt.Println("⚡ [Hotkey] Triggered!")
					callback()
				}

				// Translate and dispatch the message
				procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
				procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
			} else {
				// No message, sleep briefly to avoid CPU spinning
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()
}
