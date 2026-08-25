//go:build !windows

package main

func promptForConfig(current clientFileConfig) (clientFileConfig, bool, error) {
	return current, false, nil
}
