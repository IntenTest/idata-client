package browserbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	Address       string
	AllowedOrigin string
	ClientID      string
	DeviceToken   string
	Launch        func(string) error
}

type Bridge struct {
	config Config
}

func ForwardLaunch(address, serverURL string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid browser bridge address: %w", err)
	}
	loopback, err := netip.ParseAddr(host)
	if err != nil || !loopback.IsLoopback() {
		return errors.New("browser bridge must use a numeric loopback address")
	}
	body, err := json.Marshal(map[string]string{"server_url": serverURL})
	if err != nil {
		return fmt.Errorf("encode launch request: %w", err)
	}
	request, err := http.NewRequest(http.MethodPost, "http://"+address+"/v1/launch", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create launch request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			Proxy:       nil,
			DialContext: (&net.Dialer{Timeout: time.Second}).DialContext,
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("contact running client: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("running client rejected launch: HTTP %d", response.StatusCode)
	}
	return nil
}

func New(config Config) (*Bridge, error) {
	host, _, err := net.SplitHostPort(config.Address)
	if err != nil {
		return nil, fmt.Errorf("invalid browser bridge address: %w", err)
	}
	address, err := netip.ParseAddr(host)
	if err != nil || !address.IsLoopback() {
		return nil, errors.New("browser bridge must bind to a numeric loopback address")
	}
	origin, err := url.Parse(config.AllowedOrigin)
	if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Host == "" || origin.User != nil {
		return nil, errors.New("browser bridge origin must be a valid HTTP origin")
	}
	if origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return nil, errors.New("browser bridge origin must not contain a path, query, or fragment")
	}
	if config.ClientID == "" || len(config.DeviceToken) < 32 || len(config.DeviceToken) > 256 {
		return nil, errors.New("browser bridge client ID and device token are required")
	}
	config.AllowedOrigin = origin.String()
	return &Bridge{config: config}, nil
}

func (b *Bridge) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", b.config.Address)
	if err != nil {
		return fmt.Errorf("listen on browser bridge: %w", err)
	}
	server := &http.Server{
		Handler:           b.handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownContext)
	}()
	err = server.Serve(listener)
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	<-shutdownDone
	return nil
}

func (b *Bridge) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Vary", "Origin")
		remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
		remoteAddress, parseErr := netip.ParseAddr(remoteHost)
		if err != nil || parseErr != nil || !remoteAddress.IsLoopback() {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/v1/launch" {
			b.handleLaunch(w, r)
			return
		}
		requestOrigin := r.Header.Get("Origin")
		if !sameOrigin(requestOrigin, b.config.AllowedOrigin) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", requestOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		if r.Header.Get("Access-Control-Request-Private-Network") == "true" {
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/v1/identity" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"client_id":    b.config.ClientID,
			"device_token": b.config.DeviceToken,
		})
	})
}

func (b *Bridge) handleLaunch(w http.ResponseWriter, r *http.Request) {
	// Browser requests always carry an Origin header. This endpoint is reserved
	// for a second, native idata-client process started by the URL protocol.
	if r.Header.Get("Origin") != "" || b.config.Launch == nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	var request struct {
		ServerURL string `json:"server_url"`
	}
	if err := decoder.Decode(&request); err != nil || request.ServerURL == "" {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := b.config.Launch(request.ServerURL); err != nil {
		http.Error(w, "launch rejected", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func sameOrigin(actual, expected string) bool {
	actualURL, actualErr := url.Parse(actual)
	expectedURL, expectedErr := url.Parse(expected)
	if actualErr != nil || expectedErr != nil || actualURL.User != nil || expectedURL.User != nil {
		return false
	}
	return strings.EqualFold(actualURL.Scheme, expectedURL.Scheme) &&
		strings.EqualFold(actualURL.Hostname(), expectedURL.Hostname()) &&
		effectivePort(actualURL) == effectivePort(expectedURL)
}

func effectivePort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}
	if strings.EqualFold(value.Scheme, "http") {
		return "80"
	}
	if strings.EqualFold(value.Scheme, "https") {
		return "443"
	}
	return ""
}
