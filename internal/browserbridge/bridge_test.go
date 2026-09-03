package browserbridge

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const testDeviceToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestBridgeRequiresLoopbackAddress(t *testing.T) {
	_, err := New(Config{
		Address: "0.0.0.0:17891", AllowedOrigin: "http://10.0.0.2",
		ClientID: "test-pc", DeviceToken: testDeviceToken,
	})
	if err == nil {
		t.Fatal("non-loopback browser bridge address was accepted")
	}
}

func TestIdentityIsLimitedToConfiguredOriginAndLoopback(t *testing.T) {
	bridge, err := New(Config{
		Address: "127.0.0.1:17891", AllowedOrigin: "http://10.0.0.2",
		ClientID: "test-pc", DeviceToken: testDeviceToken,
	})
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/identity", nil)
	request.RemoteAddr = "127.0.0.1:51000"
	request.Header.Set("Origin", "http://10.0.0.2")
	response := httptest.NewRecorder()
	bridge.handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var identity map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &identity); err != nil {
		t.Fatal(err)
	}
	if identity["client_id"] != "test-pc" || identity["device_token"] != testDeviceToken {
		t.Fatalf("unexpected identity: %#v", identity)
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/identity", nil)
	request.RemoteAddr = "127.0.0.1:51000"
	request.Header.Set("Origin", "http://evil.example")
	response = httptest.NewRecorder()
	bridge.handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("wrong-origin status = %d, want %d", response.Code, http.StatusForbidden)
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/identity", nil)
	request.RemoteAddr = "10.0.0.20:51000"
	request.Header.Set("Origin", "http://10.0.0.2")
	response = httptest.NewRecorder()
	bridge.handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-loopback status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestBridgeHandlesPrivateNetworkPreflight(t *testing.T) {
	bridge, err := New(Config{
		Address: "127.0.0.1:17891", AllowedOrigin: "https://idata.example",
		ClientID: "test-pc", DeviceToken: testDeviceToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodOptions, "/v1/identity", nil)
	request.RemoteAddr = "127.0.0.1:51000"
	request.Header.Set("Origin", "https://idata.example")
	request.Header.Set("Access-Control-Request-Private-Network", "true")
	response := httptest.NewRecorder()
	bridge.handler().ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	if response.Header().Get("Access-Control-Allow-Private-Network") != "true" {
		t.Fatal("private network preflight was not allowed")
	}
}

func TestNativeLaunchIsForwardedWithoutBrowserAccess(t *testing.T) {
	launched := ""
	bridge, err := New(Config{
		Address: "127.0.0.1:17891", AllowedOrigin: "http://10.0.0.2",
		ClientID: "test-pc", DeviceToken: testDeviceToken,
		Launch: func(serverURL string) error {
			launched = serverURL
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(bridge.handler())
	defer server.Close()
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := ForwardLaunch(serverURL.Host, "ws://192.168.8.87:12345/ws/agent"); err != nil {
		t.Fatal(err)
	}
	if launched != "ws://192.168.8.87:12345/ws/agent" {
		t.Fatalf("forwarded launch URL = %q", launched)
	}

	browserRequest := httptest.NewRequest(http.MethodPost, "/v1/launch", strings.NewReader(`{"server_url":"ws://192.168.8.87:12345/ws/agent"}`))
	browserRequest.RemoteAddr = "127.0.0.1:51000"
	browserRequest.Header.Set("Origin", "http://10.0.0.2")
	browserResponse := httptest.NewRecorder()
	bridge.handler().ServeHTTP(browserResponse, browserRequest)
	if browserResponse.Code != http.StatusForbidden {
		t.Fatalf("browser launch status = %d, want %d", browserResponse.Code, http.StatusForbidden)
	}
}

func TestNativeLaunchRejectionIsReturned(t *testing.T) {
	bridge, err := New(Config{
		Address: "127.0.0.1:17891", AllowedOrigin: "http://10.0.0.2",
		ClientID: "test-pc", DeviceToken: testDeviceToken,
		Launch: func(string) error { return errors.New("rejected") },
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/launch", strings.NewReader(`{"server_url":"ws://192.168.8.87:12345/ws/agent"}`))
	request.RemoteAddr = "127.0.0.1:51000"
	response := httptest.NewRecorder()
	bridge.handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestSameOriginTreatsDefaultPortsAsEquivalent(t *testing.T) {
	if !sameOrigin("http://10.0.0.2", "http://10.0.0.2:80") {
		t.Fatal("equivalent HTTP origins were not matched")
	}
	if sameOrigin("https://10.0.0.2", "http://10.0.0.2") {
		t.Fatal("different schemes were matched")
	}
}
