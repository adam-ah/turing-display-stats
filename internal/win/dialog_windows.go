//go:build windows

package win

import (
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	errorDialogClassName = "TuringDisplayErrorDialog"
	errorDialogWidth     = 720
	errorDialogHeight    = 420
	errorDialogMargin    = 16
	errorDialogButtonW   = 96
	errorDialogButtonH   = 28
	errorDialogGap       = 8
	errorDialogCopyID    = 1001
	errorDialogCloseID   = 1002
)

const (
	wmCreate       = 0x0001
	wmDestroy      = 0x0002
	wmSize         = 0x0005
	wmCommand      = 0x0111
	wmCopy         = 0x0301
	emSetSel       = 0x00B1
	wsChild        = 0x40000000
	wsVisible      = 0x10000000
	wsBorder       = 0x00800000
	wsTabStop      = 0x00010000
	wsVScroll      = 0x00200000
	wsCaption      = 0x00C00000
	wsSysMenu      = 0x00080000
	wsThickFrame   = 0x00040000
	wsExClientEdge = 0x00000200
	esMultiline    = 0x0004
	esAutovscroll  = 0x0040
	esAutoHscroll  = 0x0080
	esReadOnly     = 0x0800
	esWantReturn   = 0x1000
	bsPushbutton   = 0x00000000
	swShow         = 5
)

var (
	procDestroyWindow   = user32.NewProc("DestroyWindow")
	procShowWindow      = user32.NewProc("ShowWindow")
	procUpdateWindow    = user32.NewProc("UpdateWindow")
	procSendMessage     = user32.NewProc("SendMessageW")
	procPostQuitMessage = user32.NewProc("PostQuitMessage")
	procSetFocus        = user32.NewProc("SetFocus")
	procGetClientRect   = user32.NewProc("GetClientRect")
	procMoveWindow      = user32.NewProc("MoveWindow")
	procMessageBox      = user32.NewProc("MessageBoxW")
)

var currentErrorDialog *errorDialogState

type errorDialogState struct {
	hwnd        uintptr
	edit        uintptr
	copyButton  uintptr
	closeButton uintptr
	title       string
	message     string
}

func ShowErrorDialog(title, message string) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	state := &errorDialogState{
		title:   title,
		message: normalizeErrorDialogText(message),
	}
	currentErrorDialog = state
	defer func() { currentErrorDialog = nil }()

	if !createErrorDialogWindow(state) {
		showErrorMessageBox(title, message)
		return
	}

	procShowWindow.Call(state.hwnd, uintptr(swShow))
	procUpdateWindow.Call(state.hwnd)

	for {
		var msg MSG
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) == -1 {
			break
		}
		if ret == 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func normalizeErrorDialogText(message string) string {
	message = strings.ReplaceAll(message, "\r\n", "\n")
	return strings.ReplaceAll(message, "\n", "\r\n")
}

func createErrorDialogWindow(state *errorDialogState) bool {
	hInst, _, _ := procGetModuleHandle.Call(0)
	hInstance = windows.Handle(hInst)

	className, _ := windows.UTF16PtrFromString(errorDialogClassName)
	arrowCursor, _, _ := procLoadCursor.Call(0, 32512) // IDC_ARROW
	wc := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc:   syscall.NewCallback(errorDialogWndProc),
		HInstance:     uintptr(hInstance),
		HCursor:       arrowCursor,
		HbrBackground: uintptr(6), // COLOR_WINDOW + 1
		LpszClassName: className,
	}
	if ret, _, _ := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); ret == 0 {
		return false
	}

	title, _ := windows.UTF16PtrFromString(state.title)
	hwnd, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		uintptr(wsCaption|wsSysMenu|wsThickFrame),
		0x80000000, // CW_USEDEFAULT
		0x80000000, // CW_USEDEFAULT
		errorDialogWidth,
		errorDialogHeight,
		0,
		0,
		uintptr(hInstance),
		0,
	)
	if hwnd == 0 {
		return false
	}
	state.hwnd = hwnd
	return true
}

func errorDialogWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	state := currentErrorDialog
	switch msg {
	case wmCreate:
		if state == nil {
			return ^uintptr(0)
		}
		state.hwnd = hwnd
		if !createErrorDialogChildren(state, hwnd) {
			return ^uintptr(0)
		}
		return 0
	case wmSize:
		layoutErrorDialog(state)
		return 0
	case wmCommand:
		switch uint16(wParam & 0xFFFF) {
		case errorDialogCopyID:
			copyErrorDialogText(state)
		case errorDialogCloseID:
			procDestroyWindow.Call(hwnd)
		}
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return ret
}

func createErrorDialogChildren(state *errorDialogState, parent uintptr) bool {
	editableText, _ := windows.UTF16PtrFromString(state.message)
	editClass, _ := windows.UTF16PtrFromString("Edit")
	buttonClass, _ := windows.UTF16PtrFromString("Button")
	copyText, _ := windows.UTF16PtrFromString("Copy")
	closeText, _ := windows.UTF16PtrFromString("Close")

	state.edit, _, _ = procCreateWindowEx.Call(
		uintptr(wsExClientEdge),
		uintptr(unsafe.Pointer(editClass)),
		uintptr(unsafe.Pointer(editableText)),
		uintptr(wsChild|wsVisible|wsBorder|wsVScroll|wsTabStop|esMultiline|esAutovscroll|esAutoHscroll|esReadOnly|esWantReturn),
		errorDialogMargin,
		errorDialogMargin,
		errorDialogWidth-(errorDialogMargin*2),
		errorDialogHeight-(errorDialogMargin*3)-errorDialogButtonH,
		parent,
		0,
		uintptr(hInstance),
		0,
	)
	if state.edit == 0 {
		return false
	}

	state.copyButton, _, _ = procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(buttonClass)),
		uintptr(unsafe.Pointer(copyText)),
		uintptr(wsChild|wsVisible|wsTabStop|bsPushbutton),
		errorDialogWidth-(errorDialogMargin*2)-errorDialogButtonW*2-errorDialogGap,
		errorDialogHeight-(errorDialogMargin*2)-errorDialogButtonH,
		errorDialogButtonW,
		errorDialogButtonH,
		parent,
		errorDialogCopyID,
		uintptr(hInstance),
		0,
	)
	if state.copyButton == 0 {
		return false
	}

	state.closeButton, _, _ = procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(buttonClass)),
		uintptr(unsafe.Pointer(closeText)),
		uintptr(wsChild|wsVisible|wsTabStop|bsPushbutton),
		errorDialogWidth-(errorDialogMargin*2)-errorDialogButtonW,
		errorDialogHeight-(errorDialogMargin*2)-errorDialogButtonH,
		errorDialogButtonW,
		errorDialogButtonH,
		parent,
		errorDialogCloseID,
		uintptr(hInstance),
		0,
	)
	if state.closeButton == 0 {
		return false
	}

	procSendMessage.Call(state.edit, emSetSel, 0, ^uintptr(0))
	procSetFocus.Call(state.edit)
	layoutErrorDialog(state)
	return true
}

func layoutErrorDialog(state *errorDialogState) {
	if state == nil || state.hwnd == 0 {
		return
	}

	var rect struct {
		Left   int32
		Top    int32
		Right  int32
		Bottom int32
	}
	if ret, _, _ := procGetClientRect.Call(state.hwnd, uintptr(unsafe.Pointer(&rect))); ret == 0 {
		return
	}

	width := int(rect.Right - rect.Left)
	height := int(rect.Bottom - rect.Top)
	editHeight := height - (errorDialogMargin * 3) - errorDialogButtonH
	buttonTop := height - errorDialogMargin - errorDialogButtonH
	copyLeft := width - errorDialogMargin - errorDialogButtonW*2 - errorDialogGap
	closeLeft := width - errorDialogMargin - errorDialogButtonW

	procMoveWindow.Call(state.edit,
		uintptr(errorDialogMargin),
		uintptr(errorDialogMargin),
		uintptr(width-(errorDialogMargin*2)),
		uintptr(editHeight),
		1,
	)
	procMoveWindow.Call(state.copyButton,
		uintptr(copyLeft),
		uintptr(buttonTop),
		errorDialogButtonW,
		errorDialogButtonH,
		1,
	)
	procMoveWindow.Call(state.closeButton,
		uintptr(closeLeft),
		uintptr(buttonTop),
		errorDialogButtonW,
		errorDialogButtonH,
		1,
	)
}

func copyErrorDialogText(state *errorDialogState) {
	if state == nil || state.edit == 0 {
		return
	}
	procSetFocus.Call(state.edit)
	procSendMessage.Call(state.edit, emSetSel, 0, ^uintptr(0))
	procSendMessage.Call(state.edit, wmCopy, 0, 0)
}

func showErrorMessageBox(title, message string) {
	titlePtr, _ := windows.UTF16PtrFromString(title)
	messagePtr, _ := windows.UTF16PtrFromString(message)
	procMessageBox.Call(
		0,
		uintptr(unsafe.Pointer(messagePtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(mbOK|mbIconError|mbTaskModal),
	)
}

const (
	mbOK        = 0x00000000
	mbIconError = 0x00000010
	mbTaskModal = 0x00002000
)
