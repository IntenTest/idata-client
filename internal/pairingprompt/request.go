package pairingprompt

type Request struct {
	Challenge  string `json:"challenge"`
	BrowserIP  string `json:"browser_ip"`
	ServerHost string `json:"server_host"`
	SessionTTL int    `json:"session_ttl_seconds"`
}
