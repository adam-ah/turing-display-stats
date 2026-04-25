//go:build windows

package win

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procMessageBox = user32.NewProc("MessageBoxW")
)

const (
	mbOK        = 0x00000000
	mbIconError = 0x00000010
	mbTaskModal = 0x00002000
)

func ShowErrorDialog(title, message string) {
	titlePtr, _ := windows.UTF16PtrFromString(title)
	messagePtr, _ := windows.UTF16PtrFromString(message)
	procMessageBox.Call(
		0,
		uintptr(unsafe.Pointer(messagePtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(mbOK|mbIconError|mbTaskModal),
	)
}
