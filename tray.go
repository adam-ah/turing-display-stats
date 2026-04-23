//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ---------------------------------------------------------------------------
// Tray icon — system tray, context menu, exit channel
// ---------------------------------------------------------------------------

const (
	NIM_ADD         = 0x00000000
	NIM_DELETE      = 0x00000002
	NIF_MESSAGE     = 0x00000001
	NIF_ICON        = 0x00000002
	NIF_TIP         = 0x00000004
	WM_APP          = 0x8000
	WM_USER_TRAY    = WM_APP + 1
	IDI_APPLICATION = 0x7F00
	LR_SHARED       = 0x00000001
	LR_LOADFROMFILE = 0x00000010
	IMAGE_ICON      = 1
	WM_RBUTTONUP    = 0x0205
	WM_LBUTTONUP    = 0x0202
	WM_RBUTTONDOWN  = 0x0204
	WM_LBUTTONDOWN  = 0x0201
	WM_DESTROY      = 0x0002
	WM_QUIT         = 0x0012
	WM_COMMAND      = 0x0111
	MF_STRING       = 0x00000000
	MF_SEPARATOR    = 0x00000800
	TPM_LEFTALIGN   = 0x0000
	TPM_BOTTOMALIGN = 0x0020
)

const (
	MENU_ABOUT       = 1001
	MENU_OPEN_CONFIG = 1002
	MENU_EXIT        = 1003
)

var (
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	user32                  = syscall.NewLazyDLL("user32.dll")
	shell32                 = syscall.NewLazyDLL("shell32.dll")
	procGetModuleHandle     = kernel32.NewProc("GetModuleHandleW")
	procLoadCursor          = user32.NewProc("LoadCursorW")
	procLoadImage           = user32.NewProc("LoadImageW")
	procRegisterClassEx     = user32.NewProc("RegisterClassExW")
	procCreateWindowEx      = user32.NewProc("CreateWindowExW")
	procDefWindowProc       = user32.NewProc("DefWindowProcW")
	procPostMessage         = user32.NewProc("PostMessageW")
	procShellNotifyIcon     = shell32.NewProc("Shell_NotifyIconW")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenu          = user32.NewProc("AppendMenuW")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procShellExecute        = shell32.NewProc("ShellExecuteW")
	procMessageBox          = user32.NewProc("MessageBoxW")
	procGetMessage          = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessage     = user32.NewProc("DispatchMessageW")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
)

// WNDCLASSEX matches the Windows WNDCLASSEX structure.
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

// NOTIFYICONDATA matches the Windows NOTIFYICONDATA structure.
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
	GuidItem         GUID
	HBalloonIcon     uintptr
}

// GUID matches the Windows GUID structure.
type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// MSG matches the Windows MSG structure for GetMessage/DispatchMessage.
type MSG struct {
	HWnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       POINT
	LPrivate uint32
}

// POINT matches the Windows POINT structure for GetCursorPos.
type POINT struct {
	X int32
	Y int32
}

var (
	hInstance windows.Handle
	mainWnd   uintptr
	exitApp   = make(chan struct{})
)

// trayReady is closed when the tray icon is fully initialized.
var trayReady = make(chan struct{})

// initTray starts a dedicated goroutine that creates the tray window and
// runs the message pump. The window and message pump MUST be in the same
// goroutine AND locked to the same OS thread (runtime.LockOSThread) because
// Windows messages are thread-local.
func initTray() error {
	initErr := make(chan error, 1)
	go func() {
		// Lock this goroutine to a single OS thread. Windows requires that
		// the window creation, callback registration, and message pump all
		// run on the same OS thread. Without this, Go's scheduler can move
		// the goroutine between threads and messages are silently lost.
		runtime.LockOSThread()
		if err := createTrayWindow(); err != nil {
			initErr <- err
		}
	}()
	// Wait for either an error or the ready signal.
	select {
	case err := <-initErr:
		return err
	case <-trayReady:
		return nil
	}
}

// createTrayWindow registers the window class, creates the window, and
// registers the tray icon. Must run in the same goroutine as runMessageLoop
// with runtime.LockOSThread() called first.
func createTrayWindow() error {
	hInst, _, _ := procGetModuleHandle.Call(0)
	hInstance = windows.Handle(hInst)

	className, _ := windows.UTF16PtrFromString("TuringDisplayTray")
	arrowCursor, _, _ := procLoadCursor.Call(0, 32512) // IDC_ARROW

	wc := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc:   syscall.NewCallback(trayWndProc),
		HInstance:     uintptr(hInstance),
		HCursor:       arrowCursor,
		HbrBackground: uintptr(1),
		LpszClassName: className,
	}
	if ret, _, _ := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); ret == 0 {
		return fmt.Errorf("RegisterClassEx failed")
	}

	title, _ := windows.UTF16PtrFromString("Turing Display")
	ret, _, _ := procCreateWindowEx.Call(
		0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)),
		0, 0, 0, 0, 0, 0, 0,
		uintptr(hInstance), 0,
	)
	if ret == 0 {
		return fmt.Errorf("CreateWindowEx failed")
	}
	mainWnd = ret

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("runtime.Caller failed")
	}
	iconPath := filepath.Join(filepath.Dir(file), "..", "res", "icons", "monitor-icon-17865", "icon.ico")
	iconPathPtr, err := windows.UTF16PtrFromString(iconPath)
	if err != nil {
		return err
	}
	icon, _, _ := procLoadImage.Call(
		0, uintptr(unsafe.Pointer(iconPathPtr)), IMAGE_ICON,
		0, 0, LR_SHARED|LR_LOADFROMFILE,
	)
	if icon == 0 {
		return fmt.Errorf("LoadImage failed for %s", iconPath)
	}

	nid := NOTIFYICONDATA{
		CbSize:           uint32(unsafe.Sizeof(NOTIFYICONDATA{})),
		HWnd:             mainWnd,
		UID:              1,
		UFlags:           NIF_MESSAGE | NIF_ICON | NIF_TIP,
		UCallbackMessage: WM_USER_TRAY,
		HIcon:            icon,
	}
	for i, r := range "Turing Smart Screen Display" {
		nid.SzTip[i] = uint16(r)
	}

	if ret, _, err := procShellNotifyIcon.Call(NIM_ADD, uintptr(unsafe.Pointer(&nid))); ret == 0 {
		return fmt.Errorf("Shell_NotifyIcon failed: %v", err)
	}

	// Signal that the tray is ready before entering the blocking message pump.
	close(trayReady)

	// Run the message pump in THIS goroutine (same thread as window creation).
	runMessageLoop()
	return nil
}

func removeTray() {
	nid := NOTIFYICONDATA{
		CbSize: uint32(unsafe.Sizeof(NOTIFYICONDATA{})),
		HWnd:   mainWnd,
		UID:    1,
	}
	procShellNotifyIcon.Call(NIM_DELETE, uintptr(unsafe.Pointer(&nid)))
}

func trayWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_DESTROY:
		procPostMessage.Call(uintptr(hwnd), WM_QUIT, 0, 0)
		return 0
	case WM_COMMAND:
		switch uint32(wParam) {
		case MENU_ABOUT:
			showAbout()
		case MENU_OPEN_CONFIG:
			openConfigFile()
		case MENU_EXIT:
			select {
			case <-exitApp:
			default:
				close(exitApp)
			}
		}
		return 0
	case WM_USER_TRAY:
		// lParam contains the mouse message that triggered the callback.
		switch uint32(lParam) {
		case WM_RBUTTONUP, WM_LBUTTONUP, WM_RBUTTONDOWN, WM_LBUTTONDOWN:
			showContextMenu()
		}
		return 0
	}
	ret, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return ret
}

func showContextMenu() {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)

	procAppendMenu.Call(menu, MF_STRING, MENU_ABOUT,
		uintptr(unsafe.Pointer(utf16ptr("About"))))
	procAppendMenu.Call(menu, MF_STRING, MENU_OPEN_CONFIG,
		uintptr(unsafe.Pointer(utf16ptr("Open Config"))))
	procAppendMenu.Call(menu, MF_SEPARATOR, 0, 0)
	procAppendMenu.Call(menu, MF_STRING, MENU_EXIT,
		uintptr(unsafe.Pointer(utf16ptr("Exit"))))

	// Get the current cursor position so the menu appears under the mouse.
	var pt POINT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	procSetForegroundWindow.Call(uintptr(mainWnd))
	procTrackPopupMenu.Call(
		menu,
		TPM_LEFTALIGN|TPM_BOTTOMALIGN,
		uintptr(pt.X), uintptr(pt.Y),
		0, uintptr(mainWnd), 0,
	)
}

// runMessageLoop pumps Windows messages so that the tray window procedure
// receives WM_COMMAND, WM_USER_TRAY, etc. It signals exitApp on WM_QUIT.
// MUST run in the same gor/thread that created the window (runtime.LockOSThread).
func runMessageLoop() {
	for {
		var msg MSG
		ret, _, _ := procGetMessage.Call(
			uintptr(unsafe.Pointer(&msg)),
			0, 0, 0,
		)
		// GetMessage returns 0 when WM_QUIT is received.
		if ret == 0 {
			select {
			case <-exitApp:
			default:
				close(exitApp)
			}
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func showAbout() {
	title, _ := windows.UTF16PtrFromString("Turing Display")
	msg, _ := windows.UTF16PtrFromString("Turing Smart Screen Display\n\nMonitors GPU VRAM, GPU utilization (all engines), RAM, and CPU usage.\nDisplays data on the Turing Smart Screen device.")
	procMessageBox.Call(0, uintptr(unsafe.Pointer(msg)),
		uintptr(unsafe.Pointer(title)), 0x40)
}

func openConfigFile() {
	path, _ := windows.UTF16PtrFromString("config.json")
	procShellExecute.Call(0, 0, uintptr(unsafe.Pointer(path)), 0, 0, 1)
}

func utf16ptr(s string) *uint16 {
	p, _ := windows.UTF16PtrFromString(s)
	return p
}
