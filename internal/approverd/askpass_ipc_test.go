package approverd

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"websudo/internal/askpass"
	"websudo/internal/config"
)

func TestAskpassIPCCompletesOverSingleConnection(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "runtime", "websudo", "askpass.sock")
	listener, err := ListenAskpassIPC(socketPath)
	if err != nil {
		t.Fatalf("ListenAskpassIPC() error = %v", err)
	}
	defer func() { _ = listener.Close() }()

	store := newAskpassStoreForTest(time.Now, func() string { return "askpass-ipc" })
	srv := NewServer(Dependencies{
		Config:       config.Config{ApprovalTimeoutSeconds: 60},
		AskpassStore: store,
	})
	go func() { _ = srv.ServeAskpassIPC(listener) }()

	client := askpass.New(socketPath)
	req, err := client.Create(context.Background(), "Password:")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if req.ID != "askpass-ipc" {
		t.Fatalf("request ID = %q, want askpass-ipc", req.ID)
	}
	if _, err := store.Complete(req.ID, "secret"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	password, err := client.WaitForPassword(context.Background(), req)
	if err != nil {
		t.Fatalf("WaitForPassword() error = %v", err)
	}
	if password != "secret" {
		t.Fatalf("password = %q, want secret", password)
	}
}

func TestAskpassIPCDenyReturnsTerminalError(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "runtime", "websudo", "askpass.sock")
	listener, err := ListenAskpassIPC(socketPath)
	if err != nil {
		t.Fatalf("ListenAskpassIPC() error = %v", err)
	}
	defer func() { _ = listener.Close() }()

	store := newAskpassStoreForTest(time.Now, func() string { return "askpass-deny-ipc" })
	srv := NewServer(Dependencies{AskpassStore: store})
	go func() { _ = srv.ServeAskpassIPC(listener) }()

	client := askpass.New(socketPath)
	req, err := client.Create(context.Background(), "Password:")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := store.Deny(req.ID); err != nil {
		t.Fatalf("Deny() error = %v", err)
	}
	if _, err := client.WaitForPassword(context.Background(), req); err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("WaitForPassword() error = %v, want denied", err)
	}
}

func TestAskpassIPCExpirationReturnsTerminalError(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "runtime", "websudo", "askpass.sock")
	listener, err := ListenAskpassIPC(socketPath)
	if err != nil {
		t.Fatalf("ListenAskpassIPC() error = %v", err)
	}
	defer func() { _ = listener.Close() }()

	store := newAskpassStoreForTest(time.Now, func() string { return "askpass-expire-ipc" })
	srv := NewServer(Dependencies{AskpassStore: store})
	store.setExpirationTimeout(20 * time.Millisecond)
	go func() { _ = srv.ServeAskpassIPC(listener) }()

	client := askpass.New(socketPath)
	req, err := client.Create(context.Background(), "Password:")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := client.WaitForPassword(context.Background(), req); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("WaitForPassword() error = %v, want expired", err)
	}
}

func TestListenAskpassIPCProtectsSocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "nested", "askpass.sock")
	listener, err := ListenAskpassIPC(socketPath)
	if err != nil {
		t.Fatalf("ListenAskpassIPC() error = %v", err)
	}
	defer func() { _ = listener.Close() }()

	dirInfo, err := os.Stat(filepath.Dir(socketPath))
	if err != nil {
		t.Fatalf("Stat(socket dir) error = %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("socket dir mode = %o, want 700", got)
	}
	socketInfo, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("Stat(socket) error = %v", err)
	}
	if got := socketInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("socket mode = %o, want 600", got)
	}
}

func TestListenAskpassIPCRefusesNonSocketPath(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "askpass.sock")
	if err := os.WriteFile(socketPath, []byte("do not remove"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := ListenAskpassIPC(socketPath); err == nil {
		t.Fatal("ListenAskpassIPC() error = nil, want existing-file error")
	}
	data, err := os.ReadFile(socketPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "do not remove" {
		t.Fatalf("existing file was modified: %q", data)
	}
}

func TestListenAskpassIPCRefusesActiveSocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "askpass.sock")
	first, err := ListenAskpassIPC(socketPath)
	if err != nil {
		t.Fatalf("first ListenAskpassIPC() error = %v", err)
	}
	defer func() { _ = first.Close() }()

	if _, err := ListenAskpassIPC(socketPath); err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("second ListenAskpassIPC() error = %v, want already in use", err)
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("original socket became unreachable: %v", err)
	}
	_ = conn.Close()
}

func TestListenAskpassIPCReplacesStaleSocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "askpass.sock")
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		t.Fatalf("ListenUnix() error = %v", err)
	}
	stale.SetUnlinkOnClose(false)
	if err := stale.Close(); err != nil {
		t.Fatalf("Close(stale) error = %v", err)
	}
	if _, err := os.Stat(socketPath); err != nil {
		t.Fatalf("stale socket missing before recovery: %v", err)
	}

	listener, err := ListenAskpassIPC(socketPath)
	if err != nil {
		t.Fatalf("ListenAskpassIPC(stale) error = %v", err)
	}
	defer func() { _ = listener.Close() }()
}
