//go:build windows

package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

func redirectRuntimeErrors(file *os.File) error {
	os.Stderr = file
	setStdHandle := syscall.NewLazyDLL("kernel32.dll").NewProc("SetStdHandle")
	result, _, callErr := setStdHandle.Call(^uintptr(11), file.Fd())
	if result == 0 {
		return fmt.Errorf("redirect Windows standard error: %w", callErr)
	}
	syscall.Stderr = syscall.Handle(file.Fd())
	return nil
}

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
