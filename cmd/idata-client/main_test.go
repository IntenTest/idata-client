package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
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
	contents := []byte(`{"server_url":"ws://127.0.0.1/ws/agent","agent_token":"test-token","client_id":"windows-test","device_token":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","output_limit":2048,"allow_insecure":false,"browser_bridge_address":"127.0.0.1:19000","confirm_browser_pairing":false,"register_url_protocol":false}`)
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
	if config.RegisterURLProtocol == nil || *config.RegisterURLProtocol {
		t.Fatalf("unexpected URL protocol config: %#v", config)
	}
}

func TestLocalClientRunning(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if !localClientRunning(listener.Addr().String()) {
		t.Fatal("listening local Client was not detected")
	}
	address := listener.Addr().String()
	listener.Close()
	if localClientRunning(address) {
		t.Fatal("closed local Client address was reported as running")
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

func TestValidDeviceToken(t *testing.T) {
	if !validDeviceToken(strings.Repeat("a", 32)) {
		t.Fatal("32-character device token was rejected")
	}
	if !validDeviceToken("  " + strings.Repeat("b", 64) + "  ") {
		t.Fatal("valid device token with surrounding whitespace was rejected")
	}
	if validDeviceToken(strings.Repeat("c", 31)) || validDeviceToken(strings.Repeat("d", 257)) {
		t.Fatal("device token outside the allowed length was accepted")
	}
}

func TestDefaultAgentToken(t *testing.T) {
	if defaultAgentToken != "Fields2012" {
		t.Fatalf("default agent token = %q", defaultAgentToken)
	}
}

func TestValidServerIP(t *testing.T) {
	for _, value := range []string{"10.0.0.8", "127.0.0.1", "::1", "[2001:db8::1]"} {
		if !validServerIP(value) {
			t.Fatalf("valid server IP %q was rejected", value)
		}
	}
	for _, value := range []string{"", "server.example", "10.0.0.999", "http://10.0.0.8"} {
		if validServerIP(value) {
			t.Fatalf("invalid server IP %q was accepted", value)
		}
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

func TestServerEndpoint(t *testing.T) {
	tests := map[string]struct {
		input string
		host  string
		port  string
	}{
		"empty uses UI default": {input: "", port: "80"},
		"plain websocket":       {input: "ws://10.0.0.8/ws/agent", host: "10.0.0.8", port: "80"},
		"secure websocket":      {input: "wss://idata.example/ws/agent", host: "idata.example", port: "443"},
		"explicit port":         {input: "ws://10.0.0.8:8080/ws/agent", host: "10.0.0.8", port: "8080"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			host, port := serverEndpoint(test.input)
			if host != test.host || port != test.port {
				t.Fatalf("serverEndpoint(%q) = %q, %q; want %q, %q", test.input, host, port, test.host, test.port)
			}
		})
	}
}

func TestServerURLFromEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		port     string
		previous string
		want     string
		wantErr  bool
	}{
		{name: "IPv4", host: "10.0.0.8", port: "80", want: "ws://10.0.0.8:80/ws/agent"},
		{name: "IPv6", host: "[::1]", port: "8080", want: "ws://[::1]:8080/ws/agent"},
		{name: "preserves secure scheme", host: "idata.example", port: "443", previous: "wss://old.example/ws/agent", want: "wss://idata.example:443/ws/agent"},
		{name: "rejects URL in IP field", host: "http://10.0.0.8", port: "80", wantErr: true},
		{name: "rejects invalid port", host: "10.0.0.8", port: "70000", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := serverURLFromEndpoint(test.host, test.port, test.previous)
			if (err != nil) != test.wantErr || got != test.want {
				t.Fatalf("serverURLFromEndpoint() = %q, %v; want %q, error=%v", got, err, test.want, test.wantErr)
			}
		})
	}
}
