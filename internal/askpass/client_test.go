package askpass

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientCreateAndWaitForPassword(t *testing.T) {
	consumeCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/askpass":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			if body["prompt"] != "Password:" {
				t.Fatalf("prompt = %q, want Password:", body["prompt"])
			}
			writeTestJSON(w, http.StatusCreated, map[string]any{"id": "askpass-client", "prompt": "Password:", "status": "pending", "createdAt": "2026-06-01T12:00:00Z", "consumeToken": "token-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/askpass/askpass-client/consume":
			consumeCalls++
			if r.Header.Get("X-Websudo-Askpass-Token") != "token-1" {
				t.Fatalf("consume token = %q", r.Header.Get("X-Websudo-Askpass-Token"))
			}
			if consumeCalls == 1 {
				w.WriteHeader(http.StatusConflict)
				return
			}
			writeTestJSON(w, http.StatusOK, map[string]string{"password": "secret"})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := New(server.URL, server.Client())
	client.pollInterval = time.Millisecond
	req, err := client.Create(context.Background(), "Password:")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if req.ID != "askpass-client" || req.ConsumeToken != "token-1" {
		t.Fatalf("request = %#v", req)
	}
	password, err := client.WaitForPassword(context.Background(), req)
	if err != nil {
		t.Fatalf("WaitForPassword() error = %v", err)
	}
	if password != "secret" {
		t.Fatalf("password = %q, want secret", password)
	}
	if consumeCalls != 2 {
		t.Fatalf("consumeCalls = %d, want 2", consumeCalls)
	}
}

func TestClientWaitForPasswordExpiredReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/askpass/askpass-expired/consume" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Websudo-Askpass-Token") != "token-2" {
			t.Fatalf("consume token = %q", r.Header.Get("X-Websudo-Askpass-Token"))
		}
		w.WriteHeader(http.StatusGone)
	}))
	defer server.Close()

	client := New(server.URL, server.Client())
	_, err := client.WaitForPassword(context.Background(), Request{ID: "askpass-expired", ConsumeToken: "token-2"})
	if err == nil {
		t.Fatal("WaitForPassword() error = nil, want terminal error")
	}
}

func TestClientWaitForPasswordForbiddenReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/askpass/askpass-forbidden/consume" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := New(server.URL, server.Client())
	_, err := client.WaitForPassword(context.Background(), Request{ID: "askpass-forbidden", ConsumeToken: "token-3"})
	if err == nil {
		t.Fatal("WaitForPassword() error = nil, want forbidden error")
	}
}

func TestClientCreateServiceUnavailableReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/askpass" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := New(server.URL, server.Client())
	_, err := client.Create(context.Background(), "Password:")
	if err == nil {
		t.Fatal("Create() error = nil, want service unavailable error")
	}
}

func writeTestJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
