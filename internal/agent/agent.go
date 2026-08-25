package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"idata-client/internal/executor"
	"idata-client/internal/protocol"
)

const Version = "0.1.0"

type Config struct {
	ServerURL   string
	AgentToken  string
	ClientID    string
	Hostname    string
	OutputLimit int64
}

type Agent struct {
	config Config
	logger *slog.Logger
}

func New(config Config, logger *slog.Logger) (*Agent, error) {
	if config.ServerURL == "" {
		return nil, errors.New("server URL is required")
	}
	if config.AgentToken == "" {
		return nil, errors.New("agent token is required")
	}
	if config.ClientID == "" {
		return nil, errors.New("client ID is required")
	}
	if config.OutputLimit <= 0 {
		return nil, errors.New("output limit must be positive")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Agent{config: config, logger: logger}, nil
}

func (a *Agent) Run(ctx context.Context) error {
	backoff := time.Second
	for {
		started := time.Now()
		err := a.connectAndServe(ctx)
		if ctx.Err() != nil {
			return nil
		}
		a.logger.Warn("connection ended; reconnecting", "error", err, "retry_in", backoff)
		jitter := time.Duration(rand.Int63n(int64(backoff/4 + 1)))
		timer := time.NewTimer(backoff + jitter)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
		if time.Since(started) >= 30*time.Second {
			backoff = time.Second
		} else if backoff < 30*time.Second {
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}
}

func (a *Agent) connectAndServe(parent context.Context) error {
	header := http.Header{}
	header.Set("Authorization", "Bearer "+a.config.AgentToken)
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, response, err := dialer.DialContext(parent, a.config.ServerURL, header)
	if err != nil {
		if response != nil {
			return fmt.Errorf("connect failed: HTTP %d", response.StatusCode)
		}
		return fmt.Errorf("connect failed: %w", err)
	}
	defer conn.Close()
	connectionDone := make(chan struct{})
	go func() {
		select {
		case <-parent.Done():
			_ = conn.Close()
		case <-connectionDone:
		}
	}()
	defer close(connectionDone)

	hello := protocol.Message{
		Type:            protocol.TypeHello,
		ProtocolVersion: protocol.Version,
		ClientID:        a.config.ClientID,
		Hostname:        a.config.Hostname,
		OS:              runtime.GOOS,
		Arch:            runtime.GOARCH,
		ClientVersion:   Version,
	}
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := conn.WriteJSON(hello); err != nil {
		return fmt.Errorf("send hello: %w", err)
	}
	a.logger.Info("connected", "server", a.config.ServerURL, "client_id", a.config.ClientID)

	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	var writeMu sync.Mutex
	var commands sync.WaitGroup
	semaphore := make(chan struct{}, 4)
	defer commands.Wait()

	for {
		var message protocol.Message
		if err := conn.ReadJSON(&message); err != nil {
			cancel()
			return err
		}
		if message.Type != protocol.TypeCommand || message.ProtocolVersion != protocol.Version || message.RequestID == "" {
			continue
		}
		if message.TimeoutSeconds <= 0 || message.TimeoutSeconds > 24*60*60 || len(message.Command) > 32<<10 {
			result := protocol.Message{
				Type: protocol.TypeResult, ProtocolVersion: protocol.Version, RequestID: message.RequestID,
				ExitCode: -1, Error: "invalid command request",
			}
			if err := writeJSON(conn, &writeMu, result); err != nil {
				cancel()
				return err
			}
			continue
		}

		commands.Add(1)
		go func(command protocol.Message) {
			defer commands.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}
			executed := executor.Run(ctx, command.Command, time.Duration(command.TimeoutSeconds)*time.Second, a.config.OutputLimit)
			result := protocol.Message{
				Type:            protocol.TypeResult,
				ProtocolVersion: protocol.Version,
				RequestID:       command.RequestID,
				ExitCode:        executed.ExitCode,
				Stdout:          executed.Stdout,
				Stderr:          executed.Stderr,
				DurationMS:      executed.Duration.Milliseconds(),
				TimedOut:        executed.TimedOut,
				StdoutTruncated: executed.StdoutTruncated,
				StderrTruncated: executed.StderrTruncated,
				Error:           executed.Error,
			}
			if ctx.Err() == nil {
				if err := writeJSON(conn, &writeMu, result); err != nil {
					a.logger.Warn("failed to send command result", "request_id", command.RequestID, "error", err)
				}
			}
		}(message)
	}
}

func writeJSON(conn *websocket.Conn, mu *sync.Mutex, value any) error {
	mu.Lock()
	defer mu.Unlock()
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return conn.WriteJSON(value)
}
