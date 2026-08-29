//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

func showFatalError(message string) {
	text, err := syscall.UTF16PtrFromString(message)
	if err != nil {
		return
	}
	title, err := syscall.UTF16PtrFromString("iData Client 错误")
	if err != nil {
		return
	}
	messageBox := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")
	_, _, _ = messageBox.Call(
		0,
		uintptr(unsafe.Pointer(text)),
		uintptr(unsafe.Pointer(title)),
		0x00000010,
	)
}
