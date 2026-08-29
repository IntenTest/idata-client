package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const applicationLogName = "idata-client.log"

func openApplicationLog() (*os.File, string, error) {
	var attempts []error
	executable, err := os.Executable()
	if err == nil {
		path := filepath.Join(filepath.Dir(executable), applicationLogName)
		file, openErr := openLogAt(path)
		if openErr == nil {
			return file, path, nil
		}
		attempts = append(attempts, openErr)
	} else {
		attempts = append(attempts, fmt.Errorf("locate executable: %w", err))
	}

	cacheDirectory, err := os.UserCacheDir()
	if err == nil {
		directory := filepath.Join(cacheDirectory, "iData")
		if mkdirErr := os.MkdirAll(directory, 0o700); mkdirErr == nil {
			path := filepath.Join(directory, applicationLogName)
			file, openErr := openLogAt(path)
			if openErr == nil {
				return file, path, nil
			}
			attempts = append(attempts, openErr)
		} else {
			attempts = append(attempts, fmt.Errorf("create log directory %s: %w", directory, mkdirErr))
		}
	} else {
		attempts = append(attempts, fmt.Errorf("locate user cache directory: %w", err))
	}
	return nil, "", errors.Join(attempts...)
}

func openLogAt(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log %s: %w", path, err)
	}
	return file, nil
}
