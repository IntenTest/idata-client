//go:build !windows

package main

import "os"

func redirectRuntimeErrors(file *os.File) error {
	os.Stderr = file
	return nil
}

func showFatalError(string) {}
