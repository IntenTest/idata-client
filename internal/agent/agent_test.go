package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"idata-client/internal/protocol"
)

const testDeviceToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestRunStopsWhenContextIsCanceled(t *testing.T) {
	helloReceived := make(chan protocol.Message, 1)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		var hello protocol.Message
		if err := connection.ReadJSON(&hello); err != nil {
			return
		}
		helloReceived <- hello
		_, _, _ = connection.ReadMessage()
	}))
	defer server.Close()

	application, err := New(Config{
		ServerURL:   "ws" + strings.TrimPrefix(server.URL, "http"),
		AgentToken:  "test-token",
		ClientID:    "test-client",
		Hostname:    "test-host",
		DeviceToken: testDeviceToken,
		OutputLimit: 1024,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- application.Run(ctx) }()

	select {
	case hello := <-helloReceived:
		if hello.DeviceTokenHash != deviceTokenHash(testDeviceToken) {
			t.Fatalf("device token hash = %q, want %q", hello.DeviceTokenHash, deviceTokenHash(testDeviceToken))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("client did not connect")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("client did not stop after cancellation")
	}
}

func TestInteractiveTerminalStreamsOutput(t *testing.T) {
	terminalOutput := make(chan string, 1)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		_ = connection.SetReadDeadline(time.Now().Add(5 * time.Second))
		if _, _, err := connection.ReadMessage(); err != nil {
			return
		}
		if err := connection.WriteJSON(protocol.Message{
			Type: protocol.TypeTerminalOpen, ProtocolVersion: protocol.Version, SessionID: "test-terminal",
		}); err != nil {
			return
		}
		for {
			var message protocol.Message
			if err := connection.ReadJSON(&message); err != nil {
				return
			}
			if message.Type == protocol.TypeTerminalOpened {
				newline := "\n"
				if runtime.GOOS == "windows" {
					newline = "\r\n"
				}
				_ = connection.WriteJSON(protocol.Message{
					Type: protocol.TypeTerminalInput, ProtocolVersion: protocol.Version,
					SessionID: "test-terminal", Data: []byte("echo idata-terminal-ok" + newline),
				})
			}
			if message.Type == protocol.TypeTerminalOutput && strings.Contains(string(message.Data), "idata-terminal-ok") {
				terminalOutput <- string(message.Data)
				_ = connection.WriteJSON(protocol.Message{
					Type: protocol.TypeTerminalClose, ProtocolVersion: protocol.Version, SessionID: "test-terminal",
				})
				return
			}
		}
	}))
	defer server.Close()

	application, err := New(Config{
		ServerURL: "ws" + strings.TrimPrefix(server.URL, "http"), AgentToken: "test-token",
		ClientID: "test-client", Hostname: "test-host", DeviceToken: testDeviceToken, OutputLimit: 1024,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- application.Run(ctx) }()
	select {
	case output := <-terminalOutput:
		if !strings.Contains(output, "idata-terminal-ok") {
			t.Fatalf("unexpected terminal output %q", output)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("terminal output was not streamed")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("client did not stop")
	}
}
