package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

const (
	maxSessions  = 4
	maxInputSize = 64 << 10
	outputChunk  = 16 << 10
)

type Event struct {
	Type      string
	SessionID string
	Stream    string
	Data      []byte
	ExitCode  int
	Error     string
}

type EmitFunc func(Event) error

type Manager struct {
	ctx      context.Context
	emit     EmitFunc
	mu       sync.Mutex
	sessions map[string]*session
}

type session struct {
	id        string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	cancel    context.CancelFunc
	writeMu   sync.Mutex
	closeOnce sync.Once
}

func NewManager(ctx context.Context, emit EmitFunc) *Manager {
	return &Manager{ctx: ctx, emit: emit, sessions: make(map[string]*session)}
}

func (m *Manager) Open(sessionID string) error {
	if sessionID == "" || len(sessionID) > 64 {
		return errors.New("invalid terminal session ID")
	}

	m.mu.Lock()
	if _, exists := m.sessions[sessionID]; exists {
		m.mu.Unlock()
		return errors.New("terminal session already exists")
	}
	if len(m.sessions) >= maxSessions {
		m.mu.Unlock()
		return errors.New("terminal session limit reached")
	}
	m.mu.Unlock()

	ctx, cancel := context.WithCancel(m.ctx)
	cmd := shellCommand(ctx)
	configureCommand(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("create shell input: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("create shell output: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("create shell error output: %w", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start shell: %w", err)
	}

	item := &session{id: sessionID, cmd: cmd, stdin: stdin, cancel: cancel}
	m.mu.Lock()
	if _, exists := m.sessions[sessionID]; exists || len(m.sessions) >= maxSessions {
		m.mu.Unlock()
		item.close()
		return errors.New("terminal session could not be registered")
	}
	m.sessions[sessionID] = item
	m.mu.Unlock()

	if input := initialInput(); len(input) != 0 {
		_, _ = stdin.Write(input)
	}
	if err := m.emit(Event{Type: "opened", SessionID: sessionID}); err != nil {
		item.close()
		return err
	}

	var output sync.WaitGroup
	output.Add(2)
	go func() {
		defer output.Done()
		m.copyOutput(item, "stdout", stdout)
	}()
	go func() {
		defer output.Done()
		m.copyOutput(item, "stderr", stderr)
	}()
	go m.wait(item, &output)
	return nil
}

func (m *Manager) Input(sessionID string, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if len(data) > maxInputSize {
		return errors.New("terminal input is too large")
	}
	item := m.lookup(sessionID)
	if item == nil {
		return errors.New("terminal session not found")
	}
	item.writeMu.Lock()
	defer item.writeMu.Unlock()
	_, err := item.stdin.Write(data)
	return err
}

func (m *Manager) Close(sessionID string) {
	if item := m.lookup(sessionID); item != nil {
		item.close()
	}
}

func (m *Manager) CloseAll() {
	m.mu.Lock()
	items := make([]*session, 0, len(m.sessions))
	for _, item := range m.sessions {
		items = append(items, item)
	}
	m.mu.Unlock()
	for _, item := range items {
		item.close()
	}
}

func (m *Manager) lookup(sessionID string) *session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[sessionID]
}

func (m *Manager) copyOutput(item *session, stream string, reader io.Reader) {
	buffer := make([]byte, outputChunk)
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			data := append([]byte(nil), buffer[:count]...)
			if emitErr := m.emit(Event{Type: "output", SessionID: item.id, Stream: stream, Data: data}); emitErr != nil {
				item.close()
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (m *Manager) wait(item *session, output *sync.WaitGroup) {
	output.Wait()
	err := item.cmd.Wait()
	item.cancel()

	exitCode := 0
	errorText := ""
	if item.cmd.ProcessState != nil {
		exitCode = item.cmd.ProcessState.ExitCode()
	}
	var exitError *exec.ExitError
	if err != nil && !errors.As(err, &exitError) && !errors.Is(err, context.Canceled) {
		errorText = err.Error()
	}

	m.mu.Lock()
	if m.sessions[item.id] == item {
		delete(m.sessions, item.id)
	}
	m.mu.Unlock()
	_ = m.emit(Event{Type: "closed", SessionID: item.id, ExitCode: exitCode, Error: errorText})
}

func (s *session) close() {
	s.closeOnce.Do(func() {
		_ = s.stdin.Close()
		s.cancel()
	})
}
