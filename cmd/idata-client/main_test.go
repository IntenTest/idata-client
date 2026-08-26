package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateServerURL(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		allowInsecure bool
		wantError     bool
	}{
		{name: "secure", url: "wss://idata.example.com/ws/agent"},
		{name: "localhost", url: "ws://127.0.0.1/ws/agent"},
		{name: "ipv6 localhost", url: "ws://[::1]/ws/agent"},
		{name: "remote plaintext rejected", url: "ws://10.0.0.2/ws/agent", wantError: true},
		{name: "remote plaintext explicitly allowed", url: "ws://10.0.0.2/ws/agent", allowInsecure: true},
		{name: "http rejected", url: "https://idata.example.com/ws/agent", wantError: true},
		{name: "empty rejected", url: "", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateServerURL(test.url, test.allowInsecure)
			if (err != nil) != test.wantError {
				t.Fatalf("validateServerURL() error = %v, wantError = %v", err, test.wantError)
			}
		})
	}
}

func TestLoadFileConfig(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(filepath.Dir(executable), "idata-client.json")
	contents := []byte(`{"server_url":"ws://127.0.0.1/ws/agent","agent_token":"test-token","client_id":"windows-test","device_token":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","output_limit":2048,"allow_insecure":false,"browser_bridge_address":"127.0.0.1:19000","confirm_browser_pairing":false}`)
	if err := os.WriteFile(configPath, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(configPath) })

	config, err := loadFileConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.ServerURL != "ws://127.0.0.1/ws/agent" || config.AgentToken != "test-token" || config.ClientID != "windows-test" {
		t.Fatalf("unexpected config: %#v", config)
	}
	if config.OutputLimit != 2048 || config.AllowInsecure == nil || *config.AllowInsecure {
		t.Fatalf("unexpected optional config: %#v", config)
	}
	if config.DeviceToken == "" || config.BrowserBridgeAddress != "127.0.0.1:19000" {
		t.Fatalf("unexpected device config: %#v", config)
	}
	if config.ConfirmBrowserPairing == nil || *config.ConfirmBrowserPairing {
		t.Fatalf("unexpected browser pairing confirmation config: %#v", config)
	}
}

func TestNewDeviceToken(t *testing.T) {
	first, err := newDeviceToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := newDeviceToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 64 || len(second) != 64 || first == second {
		t.Fatalf("generated tokens are not unique 256-bit hex values: %q %q", first, second)
	}
}

func TestWebOriginFromServerURL(t *testing.T) {
	tests := map[string]string{
		"ws://10.0.0.2/ws/agent":         "http://10.0.0.2",
		"wss://idata.example:8443/agent": "https://idata.example:8443",
	}
	for input, want := range tests {
		got, err := webOriginFromServerURL(input)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("webOriginFromServerURL(%q) = %q, want %q", input, got, want)
		}
	}
}
