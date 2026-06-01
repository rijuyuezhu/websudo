package sudoexec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"websudo/internal/config"
)

type Dependencies struct {
	Config   config.Config
	Environ  func() []string
	LookPath func(string) (string, error)
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
}

func Run(ctx context.Context, dep Dependencies, argv []string, cwd string) (int, error) {
	sudoArgs, err := sudoArgs(argv)
	if err != nil {
		return 0, err
	}
	cfg := fillConfig(dep.Config)
	lookPath := dep.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	askpassPath, err := resolveAskpassPath(cfg.AskpassPath, lookPath)
	if err != nil {
		return 0, err
	}
	environ := dep.Environ
	if environ == nil {
		environ = os.Environ
	}

	cmd := exec.CommandContext(ctx, cfg.SudoPath, sudoArgs...)
	cmd.Dir = cwd
	cmd.Env = withEnv(environ(), "SUDO_ASKPASS", askpassPath)
	cmd.Stdin = dep.Stdin
	cmd.Stdout = dep.Stdout
	cmd.Stderr = dep.Stderr
	if cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		return exitCodeFromError(err)
	}
	return 0, nil
}

func fillConfig(cfg config.Config) config.Config {
	defaults := config.Default()
	if strings.TrimSpace(cfg.SudoPath) == "" {
		cfg.SudoPath = defaults.SudoPath
	}
	return cfg
}

func sudoArgs(argv []string) ([]string, error) {
	if len(argv) == 0 {
		return nil, errors.New("no command provided")
	}
	if argv[0] == "-v" {
		if len(argv) != 1 {
			return nil, errors.New("-v does not accept a command")
		}
		return []string{"-A", "-v"}, nil
	}
	args := []string{"-A", "--"}
	args = append(args, argv...)
	return args, nil
}

func resolveAskpassPath(configured string, lookPath func(string) (string, error)) (string, error) {
	if strings.TrimSpace(configured) != "" {
		return configured, nil
	}
	path, err := lookPath("websudo-askpass")
	if err != nil {
		return "", fmt.Errorf("resolve websudo-askpass: %w", err)
	}
	return path, nil
}

func withEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			out = append(out, prefix+value)
			replaced = true
			continue
		}
		out = append(out, item)
	}
	if !replaced {
		out = append(out, prefix+value)
	}
	return out
}

func exitCodeFromError(err error) (int, error) {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return 1, err
	}
	if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal()), nil
	}
	return exitErr.ExitCode(), nil
}
