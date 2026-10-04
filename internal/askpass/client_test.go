package askpass

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestClientCreateAndWaitForPassword(t *testing.T) {
	listener, socketPath := testUnixListener(t)
	serverErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer func() { _ = conn.Close() }()

		var request IPCRequest
		if err := json.NewDecoder(conn).Decode(&request); err != nil {
			serverErr <- err
			return
		}
		if request.Prompt != "Password:" {
			serverErr <- errors.New("unexpected prompt")
			return
		}
		encoder := json.NewEncoder(conn)
		if err := encoder.Encode(IPCCreated{ID: "askpass-client"}); err != nil {
			serverErr <- err
			return
		}
		if err := encoder.Encode(IPCResult{Password: "secret"}); err != nil {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	client := New(socketPath)
	req, err := client.Create(context.Background(), "Password:")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if req.ID != "askpass-client" {
		t.Fatalf("request ID = %q, want askpass-client", req.ID)
	}
	password, err := client.WaitForPassword(context.Background(), req)
	if err != nil {
		t.Fatalf("WaitForPassword() error = %v", err)
	}
	if password != "secret" {
		t.Fatalf("password = %q, want secret", password)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("server error = %v", err)
	}
}

func TestClientWaitForPasswordReturnsTerminalError(t *testing.T) {
	listener, socketPath := testUnixListener(t)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		var request IPCRequest
		if json.NewDecoder(conn).Decode(&request) != nil {
			return
		}
		encoder := json.NewEncoder(conn)
		_ = encoder.Encode(IPCCreated{ID: "askpass-denied"})
		_ = encoder.Encode(IPCResult{Error: "askpass request denied"})
	}()

	client := New(socketPath)
	req, err := client.Create(context.Background(), "Password:")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := client.WaitForPassword(context.Background(), req); err == nil || err.Error() != "askpass request denied" {
		t.Fatalf("WaitForPassword() error = %v, want denied", err)
	}
}

func TestClientCreateHonorsContextCancellation(t *testing.T) {
	listener, socketPath := testUnixListener(t)
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	client := New(socketPath)
	_, err := client.Create(ctx, "Password:")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Create() error = %v, want deadline exceeded", err)
	}

	select {
	case conn := <-accepted:
		_ = conn.Close()
	case <-time.After(time.Second):
		t.Fatal("server did not accept connection")
	}
}

func TestClientWaitForPasswordHonorsContextCancellation(t *testing.T) {
	listener, socketPath := testUnixListener(t)
	release := make(chan struct{})
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		var request IPCRequest
		if json.NewDecoder(conn).Decode(&request) != nil {
			return
		}
		_ = json.NewEncoder(conn).Encode(IPCCreated{ID: "askpass-timeout"})
		<-release
	}()
	defer close(release)

	client := New(socketPath)
	req, err := client.Create(context.Background(), "Password:")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := client.WaitForPassword(ctx, req); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitForPassword() error = %v, want deadline exceeded", err)
	}
}

func testUnixListener(t *testing.T) (net.Listener, string) {
	t.Helper()
	socketPath := filepath.Join(t.TempDir(), "askpass.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener, socketPath
}
