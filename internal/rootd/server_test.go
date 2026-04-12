package rootd

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
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

func TestListenUnixSocketRestrictsPermissionsToOwner(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "websudo-rootd.sock")

	listener, err := listenUnixSocket(socketPath)
	if err != nil {
		t.Fatalf("listenUnixSocket() error = %v", err)
	}
	defer listener.Close()

	info, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("socket mode = %o, want 600", got)
	}
}

func TestServerReturnsTimeoutErrorForIncompleteRequest(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "websudo-rootd.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- Server{ConnectionTimeout: 50 * time.Millisecond}.Serve(listener)
	}()

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte(`{"resolvedPath":"/usr/bin/sh"`)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}

	var response ExecResponse
	err = json.NewDecoder(conn).Decode(&response)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if !strings.Contains(response.Error, "timeout") {
		t.Fatalf("error = %q, want timeout message", response.Error)
	}

	if err := listener.Close(); err != nil {
		t.Fatalf("listener Close() error = %v", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
}

func TestExecutorKillsProcessGroupOnTimeout(t *testing.T) {
	childPIDPath := filepath.Join(t.TempDir(), "child.pid")

	exec := Executor{}
	startedAt := time.Now()
	_, err := exec.Run(context.Background(), ExecRequest{
		ResolvedPath: "/usr/bin/sh",
		Argv: []string{"/usr/bin/sh", "-c",
			"sleep 30 & child=$!; printf %s \"$child\" > \"$1\"; wait",
			"sh", childPIDPath,
		},
		Cwd:     t.TempDir(),
		Timeout: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("Run() took %s, want timeout-driven return", elapsed)
	}

	childPIDBytes, err := os.ReadFile(childPIDPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	childPID, err := strconv.Atoi(strings.TrimSpace(string(childPIDBytes)))
	if err != nil {
		t.Fatalf("Atoi() error = %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for processExists(childPID) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if processExists(childPID) {
		_ = syscall.Kill(childPID, syscall.SIGKILL)
		t.Fatalf("child process %d still running after timeout", childPID)
	}
}

func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil
}
