package enrollment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Identity struct {
	ClientID        string `json:"client_id"`
	Hostname        string `json:"hostname"`
	Username        string `json:"username"`
	LocalIP         string `json:"local_ip"`
	MACAddress      string `json:"mac_address"`
	OS              string `json:"os"`
	Arch            string `json:"arch"`
	ClientVersion   string `json:"client_version"`
	DeviceTokenHash string `json:"device_token_hash"`
}

type startResponse struct {
	EnrollmentID string `json:"enrollment_id"`
	PollToken    string `json:"poll_token"`
	ExpiresAt    string `json:"expires_at"`
}

type statusResponse struct {
	Status     string `json:"status"`
	AgentToken string `json:"agent_token"`
	Reason     string `json:"reason"`
	Error      string `json:"error"`
}

func Request(ctx context.Context, serverURL string, identity Identity, pending func()) (string, error) {
	baseURL, err := httpBaseURL(serverURL)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	body, err := json.Marshal(identity)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/enrollments", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("submit device request: %w", err)
	}
	var started startResponse
	if err := decodeResponse(response, &started); err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusAccepted || started.EnrollmentID == "" || started.PollToken == "" {
		return "", fmt.Errorf("server rejected device request: HTTP %d", response.StatusCode)
	}
	if pending != nil {
		pending()
	}
	deadline, err := time.Parse(time.RFC3339, started.ExpiresAt)
	if err != nil {
		deadline = time.Now().Add(10 * time.Minute)
	}
	for {
		token, done, err := poll(ctx, client, baseURL, started)
		if err != nil {
			return "", err
		}
		if done {
			return token, nil
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
			if time.Now().After(deadline) {
				return "", errors.New("device approval request expired")
			}
		}
	}
}

func poll(ctx context.Context, client *http.Client, baseURL string, started startResponse) (string, bool, error) {
	endpoint := fmt.Sprintf("%s/api/v1/enrollments/%s/status", baseURL, url.PathEscape(started.EnrollmentID))
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return "", false, err
	}
	request.Header.Set("Authorization", "Enrollment "+started.PollToken)
	response, err := client.Do(request)
	if err != nil {
		return "", false, fmt.Errorf("check device approval: %w", err)
	}
	var status statusResponse
	if err := decodeResponse(response, &status); err != nil {
		return "", false, err
	}
	if response.StatusCode == http.StatusAccepted && status.Status == "pending" {
		return "", false, nil
	}
	if response.StatusCode != http.StatusOK {
		if status.Error != "" {
			return "", false, errors.New(status.Error)
		}
		return "", false, fmt.Errorf("device approval failed: HTTP %d", response.StatusCode)
	}
	if status.Status == "denied" {
		return "", false, errors.New("device request was denied by the administrator")
	}
	if status.Status != "approved" || len(status.AgentToken) < 32 {
		return "", false, errors.New("server returned an invalid device credential")
	}
	return status.AgentToken, true, nil
}

func decodeResponse(response *http.Response, target any) error {
	defer response.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(response.Body, 64<<10))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode enrollment response: %w", err)
	}
	return nil
}

func httpBaseURL(serverURL string) (string, error) {
	parsed, err := url.Parse(serverURL)
	if err != nil || parsed.Host == "" {
		return "", errors.New("invalid server URL")
	}
	switch parsed.Scheme {
	case "ws":
		parsed.Scheme = "http"
	case "wss":
		parsed.Scheme = "https"
	default:
		return "", errors.New("server URL must use ws or wss")
	}
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimSuffix(parsed.String(), "/"), nil
}
