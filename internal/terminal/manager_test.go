//go:build !windows

package terminal

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPersistentShellKeepsWorkingDirectoryAndStreamsOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan Event, 32)
	manager := NewManager(ctx, func(event Event) error {
		events <- event
		return nil
	})
	defer manager.CloseAll()

	if err := manager.Open("test-session"); err != nil {
		t.Fatal(err)
	}
	waitForEvent(t, events, func(event Event) bool { return event.Type == "opened" })

	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Input("test-session", []byte("cd "+directory+"\npwd\nprintf idata-stream-ok\n")); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var output strings.Builder
	deadline := time.After(3 * time.Second)
	for {
		select {
		case event := <-events:
			if event.Type != "output" {
				continue
			}
			mu.Lock()
			output.Write(event.Data)
			value := output.String()
			mu.Unlock()
			if strings.Contains(value, directory) && strings.Contains(value, "idata-stream-ok") {
				manager.Close("test-session")
				waitForEvent(t, events, func(event Event) bool { return event.Type == "closed" })
				return
			}
		case <-deadline:
			t.Fatalf("did not receive persistent shell output: %q", output.String())
		}
	}
}

func waitForEvent(t *testing.T, events <-chan Event, matches func(Event) bool) Event {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			if matches(event) {
				return event
			}
		case <-timer.C:
			t.Fatal("timed out waiting for terminal event")
		}
	}
}
