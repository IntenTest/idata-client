//go:build !windows

package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
)

type clientUIInitial struct {
	ServerIP, Username, Hostname, LocalIP, MACAddress string
	AutoConnect                                       bool
}
type clientUIAction struct{ Action, ServerIP string }
type clientUIUpdate struct{ State, ServerIP, ServerPort, Message string }
type clientUI struct {
	actions chan clientUIAction
	done    chan error
}

func startClientUI(initial clientUIInitial, _ *slog.Logger, _ *os.File) (*clientUI, error) {
	if initial.ServerIP == "" {
		return nil, errors.New("IDATA_SERVER_URL is required outside Windows")
	}
	ui := &clientUI{
		actions: make(chan clientUIAction, 2),
		done:    make(chan error, 1),
	}
	fmt.Fprintf(os.Stdout, "iData Client 正在连接 %s；按 Ctrl+C 可随时退出。\n", initial.ServerIP)
	ui.actions <- clientUIAction{Action: "ready"}
	ui.actions <- clientUIAction{Action: "connect", ServerIP: initial.ServerIP}
	return ui, nil
}
func (ui *clientUI) update(update clientUIUpdate) error {
	switch update.State {
	case "connected":
		fmt.Fprintf(os.Stdout, "iData Client 已连接 %s:%s。\n", update.ServerIP, update.ServerPort)
	case "enrolling", "retrying", "error":
		fmt.Fprintln(os.Stdout, update.Message)
	case "idle":
		fmt.Fprintln(os.Stdout, "iData Client 已断开。")
	}
	return nil
}
func (ui *clientUI) close() {}
