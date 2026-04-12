package rootd

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestExecutorRunsFrozenCommandAndCapturesExitCode(t *testing.T) {
	exec := Executor{}
	result, err := exec.Run(context.Background(), ExecRequest{
		ResolvedPath: "/usr/bin/sh",
		Argv:         []string{"/usr/bin/sh", "-c", "printf ok; printf bad >&2; exit 7"},
		Cwd:          t.TempDir(),
		Timeout:      5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("expected exit code 7, got %d", result.ExitCode)
	}
	if result.Stdout != "ok" {
		t.Fatalf("stdout mismatch: %q", result.Stdout)
	}
	if result.Stderr != "bad" {
		t.Fatalf("stderr mismatch: %q", result.Stderr)
	}
}

func TestServerRunsRequestOverUnixSocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "websudo-rootd.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- Server{}.Serve(listener)
	}()

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}

	request := ExecRequest{
		ResolvedPath: "/usr/bin/sh",
		Argv:         []string{"/usr/bin/sh", "-c", "printf socket-ok; printf socket-bad >&2; exit 9"},
		Cwd:          t.TempDir(),
		Timeout:      5 * time.Second,
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		conn.Close()
		t.Fatalf("Encode() error = %v", err)
	}

	var result ExecResponse
	if err := json.NewDecoder(conn).Decode(&result); err != nil {
		conn.Close()
		t.Fatalf("Decode() error = %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if result.Result.ExitCode != 9 {
		t.Fatalf("exit code = %d, want 9", result.Result.ExitCode)
	}
	if result.Result.Stdout != "socket-ok" {
		t.Fatalf("stdout = %q, want %q", result.Result.Stdout, "socket-ok")
	}
	if result.Result.Stderr != "socket-bad" {
		t.Fatalf("stderr = %q, want %q", result.Result.Stderr, "socket-bad")
	}
	if result.Error != "" {
		t.Fatalf("error = %q, want empty", result.Error)
	}

	if err := listener.Close(); err != nil {
		t.Fatalf("listener Close() error = %v", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
}
