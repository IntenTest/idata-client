//go:build !windows

package main

import (
	"errors"
	"log/slog"
	"os"
)

type clientUIInitial struct {
	ServerURL, AgentToken, DeviceToken string
	AutoConnect                        bool
}
type clientUIAction struct{ Action, ServerURL, AgentToken, DeviceToken string }
type clientUIUpdate struct{ State, ServerIP, ServerPort, Message string }
type clientUI struct {
	actions chan clientUIAction
	done    chan error
}

func startClientUI(clientUIInitial, *slog.Logger, *os.File) (*clientUI, error) {
	return nil, errors.New("idata-client desktop UI only supports Windows")
}
func (ui *clientUI) update(clientUIUpdate) error { return nil }
func (ui *clientUI) close()                      {}
