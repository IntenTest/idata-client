package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"idata-client/internal/agent"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "idata-client:", err)
		os.Exit(1)
	}
}

func run() error {
	fileConfig, err := loadFileConfig()
	if err != nil {
		return err
	}

	hostname, _ := os.Hostname()
	serverURL := flag.String("server", envOr("IDATA_SERVER_URL", fileConfig.ServerURL), "WebSocket server URL")
	clientID := flag.String("id", envOr("IDATA_CLIENT_ID", valueOr(fileConfig.ClientID, hostname)), "stable client ID")
	outputLimit := flag.Int64("output-limit", envInt64("IDATA_OUTPUT_LIMIT", positiveOr(fileConfig.OutputLimit, 1<<20)), "maximum bytes captured per output stream")
	allowInsecure := flag.Bool("allow-insecure", envBool("IDATA_ALLOW_INSECURE", boolOr(fileConfig.AllowInsecure, true)), "allow unencrypted ws:// outside localhost")
	flag.Parse()

	if err := validateServerURL(*serverURL, *allowInsecure); err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	app, err := agent.New(agent.Config{
		ServerURL:   *serverURL,
		AgentToken:  envOr("IDATA_AGENT_TOKEN", fileConfig.AgentToken),
		ClientID:    *clientID,
		Hostname:    hostname,
		OutputLimit: *outputLimit,
	}, logger)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return app.Run(ctx)
}

type clientFileConfig struct {
	ServerURL     string `json:"server_url"`
	AgentToken    string `json:"agent_token"`
	ClientID      string `json:"client_id,omitempty"`
	OutputLimit   int64  `json:"output_limit,omitempty"`
	AllowInsecure *bool  `json:"allow_insecure,omitempty"`
}

func loadFileConfig() (clientFileConfig, error) {
	executable, err := os.Executable()
	if err != nil {
		return clientFileConfig{}, fmt.Errorf("locate executable: %w", err)
	}
	path := filepath.Join(filepath.Dir(executable), "idata-client.json")
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return clientFileConfig{}, nil
	}
	if err != nil {
		return clientFileConfig{}, fmt.Errorf("open config %s: %w", path, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(io.LimitReader(file, 64<<10))
	decoder.DisallowUnknownFields()
	var config clientFileConfig
	if err := decoder.Decode(&config); err != nil {
		return clientFileConfig{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return clientFileConfig{}, fmt.Errorf("parse config %s: multiple JSON values", path)
		}
		return clientFileConfig{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return config, nil
}

func validateServerURL(raw string, allowInsecure bool) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return errors.New("IDATA_SERVER_URL must be a valid ws:// or wss:// URL")
	}
	if parsed.Scheme != "ws" && parsed.Scheme != "wss" {
		return errors.New("IDATA_SERVER_URL must use ws:// or wss://")
	}
	if parsed.Scheme == "ws" && !allowInsecure && !isLoopbackHost(parsed.Hostname()) {
		return errors.New("unencrypted ws:// is only allowed for localhost; use wss:// or explicitly set --allow-insecure")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func valueOr(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func positiveOr(value, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}

func boolOr(value *bool, fallback bool) bool {
	if value != nil {
		return *value
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
