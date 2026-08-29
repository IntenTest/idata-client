package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"idata-client/internal/agent"
	"idata-client/internal/browserbridge"
	"idata-client/internal/pairingprompt"
	"idata-client/internal/urlprotocol"
)

const defaultBrowserBridgeAddress = "127.0.0.1:17891"

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
	deviceTokenFromEnvironment := os.Getenv("IDATA_DEVICE_TOKEN")
	deviceTokenGenerated := false
	if deviceTokenFromEnvironment != "" && fileConfig.DeviceToken == "" {
		fileConfig.DeviceToken = deviceTokenFromEnvironment
	} else if fileConfig.DeviceToken == "" {
		fileConfig.DeviceToken, err = newDeviceToken()
		if err != nil {
			return err
		}
		deviceTokenGenerated = true
	}
	if deviceTokenGenerated {
		if err := saveFileConfig(fileConfig); err != nil {
			return fmt.Errorf("persist generated device token: %w", err)
		}
	}

	hostname, _ := os.Hostname()
	serverURL := flag.String("server", envOr("IDATA_SERVER_URL", fileConfig.ServerURL), "WebSocket server URL")
	clientID := flag.String("id", envOr("IDATA_CLIENT_ID", valueOr(fileConfig.ClientID, hostname)), "stable client ID")
	outputLimit := flag.Int64("output-limit", envInt64("IDATA_OUTPUT_LIMIT", positiveOr(fileConfig.OutputLimit, 1<<20)), "maximum bytes captured per output stream")
	allowInsecure := flag.Bool("allow-insecure", envBool("IDATA_ALLOW_INSECURE", boolOr(fileConfig.AllowInsecure, true)), "allow unencrypted ws:// outside localhost")
	browserBridgeAddress := flag.String("browser-bridge", envOr("IDATA_BROWSER_BRIDGE_ADDR", valueOr(fileConfig.BrowserBridgeAddress, defaultBrowserBridgeAddress)), "loopback browser pairing address, or off")
	confirmBrowserPairing := flag.Bool("confirm-browser-pairing", envBool("IDATA_CONFIRM_BROWSER_PAIRING", boolOr(fileConfig.ConfirmBrowserPairing, false)), "show a legacy local confirmation window for v0.4 browser pairing requests")
	registerURLProtocol := flag.Bool("register-url-protocol", envBool("IDATA_REGISTER_URL_PROTOCOL", boolOr(fileConfig.RegisterURLProtocol, true)), "register the idata:// browser launcher for the current Windows user")
	unregisterURLProtocol := flag.Bool("unregister-url-protocol", false, "remove the idata:// browser launcher for the current Windows user and exit")
	browserLogin := flag.Bool("browser-login", false, "start from an idata:// browser login link")
	flag.Parse()
	if *unregisterURLProtocol {
		if runtime.GOOS != "windows" {
			return errors.New("URL protocol removal is only available on Windows")
		}
		return urlprotocol.Unregister()
	}
	if runtime.GOOS == "windows" && *registerURLProtocol {
		if err := urlprotocol.Register(); err != nil {
			return err
		}
	}
	if *browserLogin && *browserBridgeAddress != "off" && localClientRunning(*browserBridgeAddress) {
		return nil
	}
	if runtime.GOOS != "windows" {
		return errors.New("idata-client desktop application only supports Windows")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	deviceToken := envOr("IDATA_DEVICE_TOKEN", fileConfig.DeviceToken)
	var pairingApprover func(context.Context, pairingprompt.Request) (bool, error)
	if *confirmBrowserPairing {
		pairingApprover = pairingprompt.Confirm
	}
	agentToken := envOr("IDATA_AGENT_TOKEN", fileConfig.AgentToken)
	serverIP, serverPort := serverEndpoint(*serverURL)
	setupError := ""
	if agentToken == "" {
		setupError = "客户端缺少认证配置，请联系管理员。"
	}
	ui, err := startClientUI(clientUIInitial{
		ServerIP: serverIP, ServerPort: serverPort, AutoConnect: *browserLogin, SetupError: setupError,
	})
	if err != nil {
		return err
	}
	defer ui.close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	type connectionEvent struct {
		generation int
		kind       string
		err        error
		retryIn    time.Duration
	}
	events := make(chan connectionEvent, 16)
	var cancelConnection context.CancelFunc
	generation := 0
	activeIP, activePort, activeURL := "", "", ""

	stopConnection := func() {
		if cancelConnection != nil {
			cancelConnection()
			cancelConnection = nil
		}
	}
	startConnection := func(action clientUIAction) {
		stopConnection()
		candidate, buildErr := serverURLFromEndpoint(action.ServerIP, action.ServerPort, *serverURL)
		if buildErr == nil {
			buildErr = validateServerURL(candidate, *allowInsecure)
		}
		if buildErr != nil {
			_ = ui.update(clientUIUpdate{State: "error", Message: "服务器地址或端口无效。"})
			return
		}
		generation++
		currentGeneration := generation
		connectionCtx, cancel := context.WithCancel(ctx)
		application, newErr := agent.New(agent.Config{
			ServerURL: candidate, AgentToken: agentToken, ClientID: *clientID, Hostname: hostname,
			DeviceToken: deviceToken, OutputLimit: *outputLimit, PairingApprover: pairingApprover,
			ConnectionState: func(connected bool) {
				kind := "disconnected"
				if connected {
					kind = "connected"
				}
				select {
				case events <- connectionEvent{generation: currentGeneration, kind: kind}:
				case <-connectionCtx.Done():
				}
			},
			Retrying: func(retryErr error, retryIn time.Duration) {
				select {
				case events <- connectionEvent{generation: currentGeneration, kind: "retrying", err: retryErr, retryIn: retryIn}:
				case <-connectionCtx.Done():
				}
			},
		}, logger)
		if newErr != nil {
			cancel()
			_ = ui.update(clientUIUpdate{State: "error", Message: "客户端配置不完整，请联系管理员。"})
			return
		}
		activeIP, activePort = serverEndpoint(candidate)
		activeURL = candidate
		cancelConnection = cancel
		if *browserBridgeAddress != "off" {
			startBrowserBridge(connectionCtx, candidate, *browserBridgeAddress, *clientID, deviceToken, logger)
		}
		go func() {
			if runErr := application.Run(connectionCtx); runErr != nil && connectionCtx.Err() == nil {
				logger.Warn("client stopped", "error", runErr)
			}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			stopConnection()
			return nil
		case err := <-ui.done:
			stopConnection()
			return err
		case action, ok := <-ui.actions:
			if !ok {
				stopConnection()
				return nil
			}
			switch action.Action {
			case "connect":
				startConnection(action)
			case "disconnect":
				stopConnection()
				generation++
				_ = ui.update(clientUIUpdate{State: "idle"})
			case "quit":
				stopConnection()
				return nil
			}
		case event := <-events:
			if event.generation != generation {
				continue
			}
			switch event.kind {
			case "connected":
				fileConfig.ServerURL = activeURL
				if err := saveFileConfig(fileConfig); err != nil {
					logger.Warn("failed to remember server address", "error", err)
				}
				_ = ui.update(clientUIUpdate{State: "connected", ServerIP: activeIP, ServerPort: activePort})
			case "retrying":
				seconds := int(event.retryIn.Round(time.Second) / time.Second)
				_ = ui.update(clientUIUpdate{State: "retrying", Message: fmt.Sprintf("连接中断，%d 秒后自动重试。", seconds)})
			}
		}
	}
}

type clientFileConfig struct {
	ServerURL             string `json:"server_url"`
	AgentToken            string `json:"agent_token"`
	ClientID              string `json:"client_id,omitempty"`
	DeviceToken           string `json:"device_token,omitempty"`
	OutputLimit           int64  `json:"output_limit,omitempty"`
	AllowInsecure         *bool  `json:"allow_insecure,omitempty"`
	BrowserBridgeAddress  string `json:"browser_bridge_address,omitempty"`
	ConfirmBrowserPairing *bool  `json:"confirm_browser_pairing,omitempty"`
	RegisterURLProtocol   *bool  `json:"register_url_protocol,omitempty"`
}

func newDeviceToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate device token: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}

func loadFileConfig() (clientFileConfig, error) {
	path, err := configFilePath()
	if err != nil {
		return clientFileConfig{}, err
	}
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

func saveFileConfig(config clientFileConfig) error {
	path, err := configFilePath()
	if err != nil {
		return err
	}
	contents, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	contents = append(contents, '\n')
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}

func configFilePath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate executable: %w", err)
	}
	return filepath.Join(filepath.Dir(executable), "idata-client.json"), nil
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

func webOriginFromServerURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", errors.New("cannot derive web origin from server URL")
	}
	switch parsed.Scheme {
	case "ws":
		parsed.Scheme = "http"
	case "wss":
		parsed.Scheme = "https"
	default:
		return "", errors.New("cannot derive web origin from non-WebSocket URL")
	}
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func serverEndpoint(raw string) (string, string) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return "", "80"
	}
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "wss" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return parsed.Hostname(), port
}

func serverURLFromEndpoint(host, port, previousURL string) (string, error) {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	port = strings.TrimSpace(port)
	if host == "" || strings.ContainsAny(host, "/\\?#@ \t\r\n") {
		return "", errors.New("invalid server host")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", errors.New("invalid server port")
	}
	scheme := "ws"
	if parsed, err := url.Parse(previousURL); err == nil && parsed.Scheme == "wss" {
		scheme = "wss"
	}
	return (&url.URL{Scheme: scheme, Host: net.JoinHostPort(host, port), Path: "/ws/agent"}).String(), nil
}

func startBrowserBridge(ctx context.Context, serverURL, address, clientID, deviceToken string, logger *slog.Logger) {
	webOrigin, err := webOriginFromServerURL(serverURL)
	if err != nil {
		logger.Warn("browser pairing bridge disabled", "error", err)
		return
	}
	bridge, err := browserbridge.New(browserbridge.Config{
		Address: address, AllowedOrigin: webOrigin, ClientID: clientID, DeviceToken: deviceToken,
	})
	if err != nil {
		logger.Warn("browser pairing bridge disabled", "error", err)
		return
	}
	go func() {
		if err := bridge.Run(ctx); err != nil && ctx.Err() == nil {
			logger.Warn("browser pairing bridge stopped", "error", err)
		}
	}()
	logger.Info("browser pairing enabled", "address", address, "origin", webOrigin)
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func localClientRunning(address string) bool {
	connection, err := net.DialTimeout("tcp", address, 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
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
