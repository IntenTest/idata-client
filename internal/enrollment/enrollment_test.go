package enrollment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRequestWaitsForApprovalAndReturnsCredential(t *testing.T) {
	const agentToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/enrollments":
			if r.Method != http.MethodPost || r.Header.Get("Origin") != "" {
				t.Fatalf("unexpected enrollment request")
			}
			var identity Identity
			if err := json.NewDecoder(r.Body).Decode(&identity); err != nil || identity.ClientID != "office-pc" || identity.Username != `CORP\alice` {
				t.Fatalf("unexpected identity: %#v, %v", identity, err)
			}
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"enrollment_id": strings.Repeat("a", 32), "poll_token": strings.Repeat("b", 64),
				"expires_at": time.Now().Add(time.Minute).UTC().Format(time.RFC3339),
			})
		case "/api/v1/enrollments/" + strings.Repeat("a", 32) + "/status":
			if r.Header.Get("Authorization") != "Enrollment "+strings.Repeat("b", 64) {
				t.Fatalf("missing enrollment polling authorization")
			}
			polls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "approved", "agent_token": agentToken})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	pendingCalled := false
	token, err := Request(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws/agent", Identity{
		ClientID: "office-pc", Username: `CORP\alice`,
	}, func() { pendingCalled = true })
	if err != nil {
		t.Fatal(err)
	}
	if token != agentToken || !pendingCalled || polls.Load() != 1 {
		t.Fatalf("token=%q pending=%v polls=%d", token, pendingCalled, polls.Load())
	}
}
