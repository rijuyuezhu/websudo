package integration

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"websudo/internal/approverd"
	"websudo/internal/cli"
	"websudo/internal/client"
	"websudo/internal/config"
	"websudo/internal/model"
	"websudo/internal/rootd"
	"websudo/internal/store"
)

func TestWebsudoEndToEndCreateApproveExecuteAndReplay(t *testing.T) {
	stack, cleanup := startTestStack(t)
	defer cleanup()

	results := make(chan struct {
		exitCode int
		stdout   string
		stderr   string
		err      error
	}, 1)

	go func() {
		exitCode, stdout, stderr, err := cli.Run(context.Background(), client.New(stack.approverdURL, stack.httpClient), []string{"/usr/bin/sh", "-c", "printf ok; printf bad >&2"}, t.TempDir())
		results <- struct {
			exitCode int
			stdout   string
			stderr   string
			err      error
		}{exitCode: exitCode, stdout: stdout, stderr: stderr, err: err}
	}()

	requestID := waitForPendingRequest(t, stack.store)
	approveRequest(t, stack.approverdURL, stack.httpClient, requestID, "123456")

	select {
	case result := <-results:
		if result.err != nil {
			t.Fatalf("cli.Run() error = %v", result.err)
		}
		if result.exitCode != 0 {
			t.Fatalf("exitCode = %d, want 0", result.exitCode)
		}
		if result.stdout != "ok" {
			t.Fatalf("stdout = %q, want %q", result.stdout, "ok")
		}
		if result.stderr != "bad" {
			t.Fatalf("stderr = %q, want %q", result.stderr, "bad")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for cli.Run to finish")
	}
}

type testStack struct {
	approverdURL string
	httpClient   *http.Client
	store        *store.SQLiteStore
	listener     net.Listener
	server       *httptest.Server
}

func startTestStack(t *testing.T) (testStack, func()) {
	t.Helper()

	sqliteStore, err := store.Open(filepath.Join(t.TempDir(), "websudo.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}

	socketPath := filepath.Join(t.TempDir(), "websudo-rootd.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	go func() {
		_ = rootd.Server{}.Serve(listener)
	}()

	srv := approverd.NewServer(approverd.Dependencies{
		Config: config.Config{TokenHashHex: config.MustHashToken("123456"), RootSocketPath: socketPath},
		Store:  approverd.NewSQLiteStore(sqliteStore),
	})
	httpServer := httptest.NewServer(srv.Routes())

	return testStack{
			approverdURL: httpServer.URL,
			httpClient:   httpServer.Client(),
			store:        sqliteStore,
			listener:     listener,
			server:       httpServer,
		}, func() {
			httpServer.Close()
			_ = listener.Close()
			_ = sqliteStore.Close()
		}
}

func waitForPendingRequest(t *testing.T, sqliteStore *store.SQLiteStore) string {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		requests, err := sqliteStore.ListRequestsByStatus(context.Background(), model.StatusPending)
		if err != nil {
			t.Fatalf("ListRequestsByStatus() error = %v", err)
		}
		if len(requests) > 0 {
			return requests[0].ID()
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for pending request")
	return ""
}

func approveRequest(t *testing.T, baseURL string, httpClient *http.Client, requestID, token string) {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/requests/"+requestID+"/approve", strings.NewReader(`{"token":"`+token+`"}`))
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}
}
