package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"idata-client/internal/pairingprompt"
	"idata-client/internal/protocol"
)

const testDeviceToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestRunReturnsAuthenticationRejectionWithoutRetrying(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()
	application, err := New(Config{
		ServerURL: "ws" + strings.TrimPrefix(server.URL, "http"), AgentToken: "rejected-token",
		ClientID: "test-client", DeviceToken: testDeviceToken, OutputLimit: 1024,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := application.Run(context.Background()); !errors.Is(err, ErrAuthenticationRejected) {
		t.Fatalf("Run() error = %v, want authentication rejection", err)
	}
}

func TestRunTreatsDeviceCredentialBindingCloseAsAuthenticationRejection(t *testing.T) {
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
		_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(
			websocket.ClosePolicyViolation, "device credential does not match this client",
		), time.Now().Add(time.Second))
	}))
	defer server.Close()
	application, err := New(Config{
		ServerURL: "ws" + strings.TrimPrefix(server.URL, "http"), AgentToken: "stale-device-token",
		ClientID: "renamed-client", DeviceToken: testDeviceToken, OutputLimit: 1024,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := application.Run(context.Background()); !errors.Is(err, ErrAuthenticationRejected) {
		t.Fatalf("Run() error = %v, want authentication rejection", err)
	}
}

func TestRunStopsWhenContextIsCanceled(t *testing.T) {
	helloReceived := make(chan protocol.Message, 1)
	connectionStates := make(chan bool, 2)
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
		ConnectionState: func(connected bool) {
			connectionStates <- connected
		},
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
	select {
	case connected := <-connectionStates:
		if !connected {
			t.Fatal("connection callback did not report connected state")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("connection callback was not called")
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
	select {
	case connected := <-connectionStates:
		if connected {
			t.Fatal("connection callback did not report disconnected state")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("disconnection callback was not called")
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

func TestBrowserPairingRequiresVisibleApproverAndLimitsConcurrentPrompts(t *testing.T) {
	results := make(chan protocol.Message, 2)
	helloReceived := make(chan protocol.Message, 1)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		_ = connection.SetReadDeadline(time.Now().Add(5 * time.Second))
		var hello protocol.Message
		if err := connection.ReadJSON(&hello); err != nil {
			return
		}
		helloReceived <- hello
		expiresAt := time.Now().Add(time.Minute).UTC().Format(time.RFC3339)
		for _, pairingID := range []string{strings.Repeat("a", 32), strings.Repeat("b", 32)} {
			if err := connection.WriteJSON(protocol.Message{
				Type: protocol.TypePairingRequest, ProtocolVersion: protocol.Version,
				PairingID: pairingID, Challenge: "PAIR IDATA ABCD-2345",
				BrowserIP: "203.0.113.10", ServerHost: "idata.example", ExpiresAt: expiresAt, SessionTTL: 3600,
			}); err != nil {
				return
			}
		}
		for range 2 {
			var result protocol.Message
			if err := connection.ReadJSON(&result); err != nil {
				return
			}
			results <- result
		}
	}))
	defer server.Close()

	promptStarted := make(chan pairingprompt.Request, 1)
	approve := make(chan struct{})
	application, err := New(Config{
		ServerURL: "ws" + strings.TrimPrefix(server.URL, "http"), AgentToken: "test-token",
		ClientID: "test-client", Hostname: "test-host", DeviceToken: testDeviceToken, OutputLimit: 1024,
		PairingApprover: func(_ context.Context, request pairingprompt.Request) (bool, error) {
			promptStarted <- request
			<-approve
			return true, nil
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- application.Run(ctx) }()

	select {
	case hello := <-helloReceived:
		if !containsString(hello.Capabilities, "browser_pairing_v1") || hello.ClientVersion != Version {
			t.Fatalf("pairing capability missing from hello: %#v", hello)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Client did not connect")
	}
	select {
	case request := <-promptStarted:
		if request.Challenge != "PAIR IDATA ABCD-2345" || request.BrowserIP != "203.0.113.10" || request.ServerHost != "idata.example" || request.SessionTTL != 3600 {
			t.Fatalf("unexpected visible prompt request: %#v", request)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("visible pairing approver was not called")
	}

	var busy protocol.Message
	select {
	case busy = <-results:
		if busy.PairingID != strings.Repeat("b", 32) || busy.Approved || busy.Error != "busy" {
			t.Fatalf("second request was not rejected as busy: %#v", busy)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("busy result was not returned")
	}
	close(approve)
	select {
	case approved := <-results:
		if approved.PairingID != strings.Repeat("a", 32) || !approved.Approved || approved.Error != "" {
			t.Fatalf("approved result = %#v", approved)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("approved result was not returned")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Client did not stop")
	}
}

func TestValidPairingRequestRejectsUntrustedDisplayFields(t *testing.T) {
	now := time.Now().UTC()
	valid := protocol.Message{
		PairingID: strings.Repeat("a", 32), Challenge: "PAIR IDATA ABCD-2345",
		BrowserIP: "203.0.113.10", ServerHost: "idata.example",
		ExpiresAt: now.Add(time.Minute).Format(time.RFC3339), SessionTTL: 3600,
	}
	if !validPairingRequest(valid, now) {
		t.Fatal("valid pairing request rejected")
	}
	tests := map[string]func(*protocol.Message){
		"invalid pairing id":   func(message *protocol.Message) { message.PairingID = strings.Repeat("Z", 32) },
		"invalid phrase":       func(message *protocol.Message) { message.Challenge = "PAIR IDATA IIII-0000" },
		"invalid IP":           func(message *protocol.Message) { message.BrowserIP = "not-an-ip" },
		"injected server host": func(message *protocol.Message) { message.ServerHost = "idata.example\r\nspoofed" },
		"expired":              func(message *protocol.Message) { message.ExpiresAt = now.Add(-time.Second).Format(time.RFC3339) },
		"excessive session":    func(message *protocol.Message) { message.SessionTTL = 25 * 60 * 60 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			message := valid
			mutate(&message)
			if validPairingRequest(message, now) {
				t.Fatalf("invalid pairing request accepted: %#v", message)
			}
		})
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
