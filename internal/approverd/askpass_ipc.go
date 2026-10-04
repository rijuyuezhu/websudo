package approverd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"websudo/internal/askpass"
)

const maxAskpassIPCRequestBytes = 8 << 10
const askpassIPCHandshakeTimeout = 5 * time.Second

func ListenAskpassIPC(socketPath string) (net.Listener, error) {
	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create askpass socket directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("protect askpass socket directory: %w", err)
	}

	if info, err := os.Lstat(socketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("askpass socket path exists and is not a socket: %s", socketPath)
		}
		conn, dialErr := net.DialTimeout("unix", socketPath, askpassIPCHandshakeTimeout)
		if dialErr == nil {
			_ = conn.Close()
			return nil, fmt.Errorf("askpass socket is already in use: %s", socketPath)
		}
		if !errors.Is(dialErr, syscall.ECONNREFUSED) {
			return nil, fmt.Errorf("probe existing askpass socket: %w", dialErr)
		}
		if err := os.Remove(socketPath); err != nil {
			return nil, fmt.Errorf("remove stale askpass socket: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect askpass socket: %w", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen on askpass socket: %w", err)
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("protect askpass socket: %w", err)
	}
	return listener, nil
}

func (s *Server) ServeAskpassIPC(listener net.Listener) error {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		go s.handleAskpassIPC(conn)
	}
}

func (s *Server) handleAskpassIPC(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	if err := conn.SetReadDeadline(time.Now().Add(askpassIPCHandshakeTimeout)); err != nil {
		return
	}
	var incoming askpass.IPCRequest
	if err := json.NewDecoder(io.LimitReader(conn, maxAskpassIPCRequestBytes)).Decode(&incoming); err != nil {
		return
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return
	}

	req := s.askpassStore.Create(incoming.Prompt)
	result, err := s.askpassStore.Result(req.ID)
	if err != nil {
		return
	}
	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(askpass.IPCCreated{ID: req.ID}); err != nil {
		return
	}

	outcome := <-result
	response := askpass.IPCResult{}
	switch outcome.status {
	case AskpassCompleted:
		response.Password = outcome.password
	case AskpassDenied:
		response.Error = "askpass request denied"
	case AskpassExpired:
		response.Error = "askpass request expired"
	default:
		response.Error = "askpass request ended unexpectedly"
	}
	_ = encoder.Encode(response)
}
