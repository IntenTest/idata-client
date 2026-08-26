package protocol

const Version = 1

const (
	TypeHello          = "hello"
	TypeCommand        = "command"
	TypeResult         = "result"
	TypeTerminalOpen   = "terminal_open"
	TypeTerminalOpened = "terminal_opened"
	TypeTerminalInput  = "terminal_input"
	TypeTerminalOutput = "terminal_output"
	TypeTerminalResize = "terminal_resize"
	TypeTerminalClose  = "terminal_close"
	TypeTerminalClosed = "terminal_closed"
	TypePairingRequest = "browser_pairing_request"
	TypePairingResult  = "browser_pairing_result"
)

type Message struct {
	Type            string   `json:"type"`
	ProtocolVersion int      `json:"protocol_version"`
	RequestID       string   `json:"request_id,omitempty"`
	SessionID       string   `json:"session_id,omitempty"`
	ClientID        string   `json:"client_id,omitempty"`
	Hostname        string   `json:"hostname,omitempty"`
	OS              string   `json:"os,omitempty"`
	Arch            string   `json:"arch,omitempty"`
	ClientVersion   string   `json:"client_version,omitempty"`
	DeviceTokenHash string   `json:"device_token_hash,omitempty"`
	Capabilities    []string `json:"capabilities,omitempty"`
	Command         string   `json:"command,omitempty"`
	TimeoutSeconds  int      `json:"timeout_seconds,omitempty"`
	ExitCode        int      `json:"exit_code,omitempty"`
	Stdout          string   `json:"stdout,omitempty"`
	Stderr          string   `json:"stderr,omitempty"`
	DurationMS      int64    `json:"duration_ms,omitempty"`
	TimedOut        bool     `json:"timed_out,omitempty"`
	StdoutTruncated bool     `json:"stdout_truncated,omitempty"`
	StderrTruncated bool     `json:"stderr_truncated,omitempty"`
	Error           string   `json:"error,omitempty"`
	Stream          string   `json:"stream,omitempty"`
	Data            []byte   `json:"data,omitempty"`
	Columns         int      `json:"columns,omitempty"`
	Rows            int      `json:"rows,omitempty"`
	PairingID       string   `json:"pairing_id,omitempty"`
	Challenge       string   `json:"challenge,omitempty"`
	BrowserIP       string   `json:"browser_ip,omitempty"`
	ServerHost      string   `json:"server_host,omitempty"`
	ExpiresAt       string   `json:"expires_at,omitempty"`
	SessionTTL      int      `json:"session_ttl_seconds,omitempty"`
	Approved        bool     `json:"approved,omitempty"`
}
