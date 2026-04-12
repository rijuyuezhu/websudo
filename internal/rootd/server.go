package rootd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"
)

type ExecRequest struct {
	ResolvedPath string        `json:"resolvedPath"`
	Argv         []string      `json:"argv"`
	Cwd          string        `json:"cwd"`
	Timeout      time.Duration `json:"timeout"`
}

type ExecResult struct {
	ExitCode int    `json:"exitCode"`
	Signal   int    `json:"signal,omitempty"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
}

type ExecResponse struct {
	Result ExecResult `json:"result"`
	Error  string     `json:"error,omitempty"`
}

type Executor struct{}

func (Executor) Run(ctx context.Context, req ExecRequest) (ExecResult, error) {
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	args := req.Argv
	if len(args) > 0 {
		args = args[1:]
	}

	cmd := exec.CommandContext(ctx, req.ResolvedPath, args...)
	cmd.Dir = req.Cwd

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := ExecResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	if err == nil {
		return result, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			result.Signal = int(status.Signal())
		}
		return result, nil
	}

	return result, err
}

type Server struct {
	Executor Executor
}

func (s Server) Serve(listener net.Listener) error {
	executor := s.Executor
	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}

		go func() {
			defer conn.Close()

			var req ExecRequest
			if err := json.NewDecoder(conn).Decode(&req); err != nil {
				_ = json.NewEncoder(conn).Encode(ExecResponse{Error: err.Error()})
				return
			}

			result, err := executor.Run(context.Background(), req)
			response := ExecResponse{Result: result}
			if err != nil {
				response.Error = err.Error()
			}

			_ = json.NewEncoder(conn).Encode(response)
		}()
	}
}

func ListenAndServe(socketPath string) error {
	if err := os.Remove(socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	defer listener.Close()

	return Server{Executor: Executor{}}.Serve(listener)
}
