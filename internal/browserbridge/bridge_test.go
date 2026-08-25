package browserbridge

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestSameOriginTreatsDefaultPortsAsEquivalent(t *testing.T) {
	if !sameOrigin("http://10.0.0.2", "http://10.0.0.2:80") {
		t.Fatal("equivalent HTTP origins were not matched")
	}
	if sameOrigin("https://10.0.0.2", "http://10.0.0.2") {
		t.Fatal("different schemes were matched")
	}
}
