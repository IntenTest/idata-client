//go:build windows

package urlprotocol

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const registryKey = `HKCU\Software\Classes\idata`

func Register() error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable for URL protocol: %w", err)
	}
	executable = strings.ReplaceAll(executable, `"`, "")
	command := fmt.Sprintf(`"%s" --browser-login "%%1"`, executable)
	commands := [][]string{
		{"add", registryKey, "/ve", "/d", "URL:iData Client", "/f"},
		{"add", registryKey, "/v", "URL Protocol", "/d", "", "/f"},
		{"add", registryKey + `\DefaultIcon`, "/ve", "/d", executable + ",0", "/f"},
		{"add", registryKey + `\shell\open\command`, "/ve", "/d", command, "/f"},
	}
	for _, arguments := range commands {
		if output, err := exec.Command("reg.exe", arguments...).CombinedOutput(); err != nil {
			return fmt.Errorf("register idata URL protocol: %w (%s)", err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func Unregister() error {
	output, err := exec.Command("reg.exe", "delete", registryKey, "/f").CombinedOutput()
	if err != nil {
		return fmt.Errorf("unregister idata URL protocol: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}
