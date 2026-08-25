package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestRunStopsWhenContextIsCanceled(t *testing.T) {
	helloReceived := make(chan struct{})
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		if _, _, err := connection.ReadMessage(); err != nil {
			return
		}
		close(helloReceived)
		_, _, _ = connection.ReadMessage()
	}))
	defer server.Close()

	application, err := New(Config{
		ServerURL:   "ws" + strings.TrimPrefix(server.URL, "http"),
		AgentToken:  "test-token",
		ClientID:    "test-client",
		Hostname:    "test-host",
		OutputLimit: 1024,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- application.Run(ctx) }()

	select {
	case <-helloReceived:
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
