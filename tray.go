//go:build windows

package main

import (
	"fmt"
	goruntime "runtime"
	"os"
	"syscall"
	"unsafe"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Windows API constants for Shell_NotifyIcon
const (
	NIM_ADD    = 0x00000000
	NIM_MODIFY = 0x00000001
	NIM_DELETE = 0x00000002

	NIF_MESSAGE = 0x00000001
	NIF_ICON    = 0x00000002
	NIF_TIP     = 0x00000004

	NIM_SETVERSION = 0x00000004
	NOTIFYICON_VERSION_4 = 4

	WM_TRAY        = 0x0400 + 1 // WM_USER + 1
	WM_LBUTTONUP   = 0x0202
	WM_RBUTTONUP   = 0x0205
	WM_CONTEXTMENU = 0x007B

	// Menu item IDs
	IDM_SETTINGS = 1001
	IDM_QUIT     = 1002

	// Icon loading
	LR_LOADFROMFILE = 0x00000010
	LR_DEFAULTSIZE  = 0x00000040
	IMAGE_ICON      = 1

	TPM_LEFTALIGN  = 0x0000
	TPM_BOTTOMALIGN = 0x0020
	TPM_RETURNCMD  = 0x0100

	GWLP_HINSTANCE = -6
)

var (
	shell32          = syscall.NewLazyDLL("shell32.dll")
	procShellNotify  = shell32.NewProc("Shell_NotifyIconW")

	procLoadImage    = user32.NewProc("LoadImageW")
	procDestroyIcon  = user32.NewProc("DestroyIcon")

	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procGetWindowLong    = user32.NewProc("GetWindowLongPtrW")

	procCreatePopupMenu   = user32.NewProc("CreatePopupMenu")
	procAppendMenuW       = user32.NewProc("AppendMenuW")
	procDestroyMenu       = user32.NewProc("DestroyMenu")
	procTrackPopupMenu    = user32.NewProc("TrackPopupMenu")
	procSetForegroundWin  = user32.NewProc("SetForegroundWindow")
	procGetCursorPos      = user32.NewProc("GetCursorPos")
)

// NOTIFYICONDATA structure for Shell_NotifyIcon
type NOTIFYICONDATA struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

type WNDCLASSEX struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type MSG struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

type POINT struct {
	X, Y int32
}

var (
	trayHWnd    uintptr
	trayIcon    uintptr
	trayNID     NOTIFYICONDATA
	trayAppRef  *App
)

// loadIconFromFile loads an ICO file from disk
func loadIconFromFile(path string) uintptr {
	pPath, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0
	}
	icon, _, _ := procLoadImage.Call(
		0,
		uintptr(unsafe.Pointer(pPath)),
		IMAGE_ICON,
		0, 0,
		LR_LOADFROMFILE|LR_DEFAULTSIZE,
	)
	return icon
}

// trayWndProc is the window procedure for our invisible tray message window
func trayWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_TRAY:
		event := lParam & 0xFFFF
		switch event {
		case WM_LBUTTONUP:
			// Left click: show launcher
			if trayAppRef != nil {
				ptrTitle, _ := syscall.UTF16PtrFromString("Anything")
				appHwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(ptrTitle)))
				fg, _, _ := procGetForegroundWindow.Call()
				trayAppRef.showWindow(fg, appHwnd)
			}
		case WM_RBUTTONUP, WM_CONTEXTMENU:
			// Right click: show menu
			showTrayMenu(hwnd)
		}
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return ret
}

func showTrayMenu(hwnd uintptr) {
	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hMenu)

	settingsText, _ := syscall.UTF16PtrFromString("⚙  Settings")
	quitText, _ := syscall.UTF16PtrFromString("✕  Quit")

	procAppendMenuW.Call(hMenu, 0, IDM_SETTINGS, uintptr(unsafe.Pointer(settingsText)))
	procAppendMenuW.Call(hMenu, 0x800, 0, 0) // MF_SEPARATOR
	procAppendMenuW.Call(hMenu, 0, IDM_QUIT, uintptr(unsafe.Pointer(quitText)))

	var pt POINT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	// Required: set foreground window so menu dismisses on click outside
	procSetForegroundWin.Call(hwnd)

	cmd, _, _ := procTrackPopupMenu.Call(
		hMenu,
		TPM_LEFTALIGN|TPM_BOTTOMALIGN|TPM_RETURNCMD,
		uintptr(pt.X),
		uintptr(pt.Y),
		0,
		hwnd,
		0,
	)

	switch cmd {
	case IDM_SETTINGS:
		if trayAppRef != nil {
			trayAppRef.OpenSettings()
		}
	case IDM_QUIT:
		fmt.Println("🔔 Tray: Quit selected")
		// Clean up tray icon
		procShellNotify.Call(NIM_DELETE, uintptr(unsafe.Pointer(&trayNID)))
		if trayIcon != 0 {
			procDestroyIcon.Call(trayIcon)
		}
		wruntime.Quit(trayAppRef.ctx)
		os.Exit(0)
	}
}

func (a *App) startSysTray() {
	trayAppRef = a
	go a.runTrayMessageLoop()
}

func (a *App) runTrayMessageLoop() {
	// CRITICAL: Win32 message loops require the goroutine to be pinned to one OS thread
	goruntime.LockOSThread()
	defer goruntime.UnlockOSThread()

	// Register a hidden window class for receiving tray messages
	className, _ := syscall.UTF16PtrFromString("AnythingTrayClass")
	windowName, _ := syscall.UTF16PtrFromString("AnythingTrayWindow")

	wc := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc:   syscall.NewCallback(trayWndProc),
		LpszClassName: className,
	}

	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	// Create invisible message-only window
	trayHWnd, _, _ = procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		0,
		0, 0, 0, 0,
		^uintptr(2), // HWND_MESSAGE = -3
		0, 0, 0,
	)

	if trayHWnd == 0 {
		fmt.Println("⚠️ Tray: Failed to create message window")
		return
	}

	// Load icon
	trayIcon = loadIconFromFile("build/windows/icon.ico")
	if trayIcon == 0 {
		trayIcon = loadIconFromFile("./build/windows/icon.ico")
	}

	// Register tray icon
	tip, _ := syscall.UTF16FromString("Anything — Universal Search")
	trayNID = NOTIFYICONDATA{
		CbSize:           uint32(unsafe.Sizeof(NOTIFYICONDATA{})),
		HWnd:             trayHWnd,
		UID:              1,
		UFlags:           NIF_MESSAGE | NIF_ICON | NIF_TIP,
		UCallbackMessage: WM_TRAY,
		HIcon:            trayIcon,
	}
	copy(trayNID.SzTip[:], tip)

	ret, _, err := procShellNotify.Call(NIM_ADD, uintptr(unsafe.Pointer(&trayNID)))
	if ret == 0 {
		fmt.Printf("⚠️ Tray: Shell_NotifyIcon failed: %v\n", err)
	} else {
		fmt.Println("✅ Tray icon added successfully")
	}

	// Message pump for the tray window
	var msg MSG
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if r == 0 || r == ^uintptr(0) { // 0 = WM_QUIT, -1 = error
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}
