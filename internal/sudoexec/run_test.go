package sudoexec

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"websudo/internal/config"
)

func TestRunExecutesCommandThroughSudoAskpass(t *testing.T) {
	argsPath := filepath.Join(t.TempDir(), "args.txt")
	envPath := filepath.Join(t.TempDir(), "env.txt")
	fakeSudo := writeFakeSudo(t, `#!/bin/sh
printf '%s\n' "$*" > "$WEBSUDO_TEST_ARGS"
printf '%s\n' "$SUDO_ASKPASS" > "$WEBSUDO_TEST_ENV"
printf 'sudo-stdout'
printf 'sudo-stderr' >&2
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode, err := Run(context.Background(), Dependencies{
		Config: config.Config{SudoPath: fakeSudo, AskpassPath: "/tmp/websudo-askpass"},
		Environ: func() []string {
			return []string{"WEBSUDO_TEST_ARGS=" + argsPath, "WEBSUDO_TEST_ENV=" + envPath}
		},
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  strings.NewReader("stdin"),
	}, []string{"/usr/bin/true", "--flag"}, t.TempDir())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", exitCode)
	}
	if stdout.String() != "sudo-stdout" {
		t.Fatalf("stdout = %q, want sudo-stdout", stdout.String())
	}
	if stderr.String() != "sudo-stderr" {
		t.Fatalf("stderr = %q, want sudo-stderr", stderr.String())
	}
	if got := strings.TrimSpace(readFile(t, argsPath)); got != "-A -- /usr/bin/true --flag" {
		t.Fatalf("sudo args = %q, want %q", got, "-A -- /usr/bin/true --flag")
	}
	if got := strings.TrimSpace(readFile(t, envPath)); got != "/tmp/websudo-askpass" {
		t.Fatalf("SUDO_ASKPASS = %q, want %q", got, "/tmp/websudo-askpass")
	}
}

func TestRunValidateUsesSudoValidate(t *testing.T) {
	argsPath := filepath.Join(t.TempDir(), "args.txt")
	fakeSudo := writeFakeSudo(t, `#!/bin/sh
printf '%s\n' "$*" > "$WEBSUDO_TEST_ARGS"
`)

	exitCode, err := Run(context.Background(), Dependencies{
		Config: config.Config{SudoPath: fakeSudo, AskpassPath: "/tmp/websudo-askpass"},
		Environ: func() []string {
			return []string{"WEBSUDO_TEST_ARGS=" + argsPath}
		},
	}, []string{"-v"}, t.TempDir())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", exitCode)
	}
	if got := strings.TrimSpace(readFile(t, argsPath)); got != "-A -v" {
		t.Fatalf("sudo args = %q, want %q", got, "-A -v")
	}
}

func TestRunReturnsErrorWhenCommandMissing(t *testing.T) {
	_, err := Run(context.Background(), Dependencies{Config: config.Config{SudoPath: "/usr/bin/sudo", AskpassPath: "/tmp/websudo-askpass"}}, nil, t.TempDir())
	if err == nil {
		t.Fatal("Run() error = nil, want missing command error")
	}
}

func TestRunPropagatesSudoExitCode(t *testing.T) {
	fakeSudo := writeFakeSudo(t, `#!/bin/sh
exit 7
`)

	exitCode, err := Run(context.Background(), Dependencies{
		Config: config.Config{SudoPath: fakeSudo, AskpassPath: "/tmp/websudo-askpass"},
	}, []string{"/usr/bin/false"}, t.TempDir())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if exitCode != 7 {
		t.Fatalf("exitCode = %d, want 7", exitCode)
	}
}

func writeFakeSudo(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sudo")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(data)
}
