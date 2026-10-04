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
	websudoPath, askpassPath := writeFakeWebsudoPair(t)
	fakeSudo := writeFakeSudo(t, `#!/bin/sh
printf '%s\n' "$*" > "$WEBSUDO_TEST_ARGS"
printf '%s\n' "$SUDO_ASKPASS" > "$WEBSUDO_TEST_ENV"
printf 'sudo-stdout'
printf 'sudo-stderr' >&2
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode, err := Run(context.Background(), Dependencies{
		Config: config.Config{SudoPath: fakeSudo},
		Executable: func() (string, error) {
			return websudoPath, nil
		},
		Environ: func() []string {
			return []string{
				"WEBSUDO_TEST_ARGS=" + argsPath,
				"WEBSUDO_TEST_ENV=" + envPath,
				"SUDO_ASKPASS=/attacker/websudo-askpass",
				"WEBSUDO_ASKPASS_PATH=/attacker/websudo-askpass",
				"PATH=/attacker",
			}
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
	if got := strings.TrimSpace(readFile(t, envPath)); got != askpassPath {
		t.Fatalf("SUDO_ASKPASS = %q, want %q", got, askpassPath)
	}
}

func TestRunValidateUsesSudoValidate(t *testing.T) {
	argsPath := filepath.Join(t.TempDir(), "args.txt")
	websudoPath, _ := writeFakeWebsudoPair(t)
	fakeSudo := writeFakeSudo(t, `#!/bin/sh
printf '%s\n' "$*" > "$WEBSUDO_TEST_ARGS"
`)

	exitCode, err := Run(context.Background(), Dependencies{
		Config:     config.Config{SudoPath: fakeSudo},
		Executable: func() (string, error) { return websudoPath, nil },
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
	_, err := Run(context.Background(), Dependencies{Config: config.Config{SudoPath: "/usr/bin/sudo"}}, nil, t.TempDir())
	if err == nil {
		t.Fatal("Run() error = nil, want missing command error")
	}
}

func TestRunPropagatesSudoExitCode(t *testing.T) {
	websudoPath, _ := writeFakeWebsudoPair(t)
	fakeSudo := writeFakeSudo(t, `#!/bin/sh
exit 7
`)

	exitCode, err := Run(context.Background(), Dependencies{
		Config:     config.Config{SudoPath: fakeSudo},
		Executable: func() (string, error) { return websudoPath, nil },
	}, []string{"/usr/bin/false"}, t.TempDir())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if exitCode != 7 {
		t.Fatalf("exitCode = %d, want 7", exitCode)
	}
}

func TestRunDoesNotStartSudoWhenAskpassSiblingIsMissing(t *testing.T) {
	markerPath := filepath.Join(t.TempDir(), "started")
	fakeSudo := writeFakeSudo(t, "#!/bin/sh\ntouch \"$WEBSUDO_TEST_MARKER\"\n")
	websudoPath := filepath.Join(t.TempDir(), "websudo")
	writeExecutable(t, websudoPath)

	_, err := Run(context.Background(), Dependencies{
		Config:     config.Config{SudoPath: fakeSudo},
		Executable: func() (string, error) { return websudoPath, nil },
		Environ: func() []string {
			return []string{"WEBSUDO_TEST_MARKER=" + markerPath}
		},
	}, []string{"/usr/bin/true"}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "websudo-askpass") {
		t.Fatalf("Run() error = %v, want missing askpass error", err)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("sudo started before askpass validation, stat error = %v", err)
	}
}

func TestResolveAskpassPathUsesResolvedExecutableSibling(t *testing.T) {
	realDir := t.TempDir()
	websudoPath := filepath.Join(realDir, "websudo")
	askpassPath := filepath.Join(realDir, "websudo-askpass")
	writeExecutable(t, websudoPath)
	writeExecutable(t, askpassPath)

	linkDir := t.TempDir()
	linkPath := filepath.Join(linkDir, "websudo")
	if err := os.Symlink(websudoPath, linkPath); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	got, err := resolveAskpassPath(func() (string, error) { return linkPath, nil })
	if err != nil {
		t.Fatalf("resolveAskpassPath() error = %v", err)
	}
	if got != askpassPath {
		t.Fatalf("resolveAskpassPath() = %q, want %q", got, askpassPath)
	}
}

func TestResolveAskpassPathResolvesSiblingSymlink(t *testing.T) {
	dir := t.TempDir()
	websudoPath := filepath.Join(dir, "websudo")
	writeExecutable(t, websudoPath)

	target := filepath.Join(t.TempDir(), "websudo-askpass-real")
	writeExecutable(t, target)
	if err := os.Symlink(target, filepath.Join(dir, "websudo-askpass")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	got, err := resolveAskpassPath(func() (string, error) { return websudoPath, nil })
	if err != nil {
		t.Fatalf("resolveAskpassPath() error = %v", err)
	}
	if got != target {
		t.Fatalf("resolveAskpassPath() = %q, want %q", got, target)
	}
}

func TestResolveAskpassPathRejectsMissingSibling(t *testing.T) {
	websudoPath := filepath.Join(t.TempDir(), "websudo")
	writeExecutable(t, websudoPath)

	if _, err := resolveAskpassPath(func() (string, error) { return websudoPath, nil }); err == nil || !strings.Contains(err.Error(), "sibling websudo-askpass") {
		t.Fatalf("resolveAskpassPath() error = %v, want missing sibling", err)
	}
}

func TestResolveAskpassPathRejectsNonExecutableSibling(t *testing.T) {
	dir := t.TempDir()
	websudoPath := filepath.Join(dir, "websudo")
	askpassPath := filepath.Join(dir, "websudo-askpass")
	writeExecutable(t, websudoPath)
	if err := os.WriteFile(askpassPath, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := resolveAskpassPath(func() (string, error) { return websudoPath, nil }); err == nil || !strings.Contains(err.Error(), "not executable") {
		t.Fatalf("resolveAskpassPath() error = %v, want non-executable sibling", err)
	}
}

func writeFakeWebsudoPair(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	websudoPath := filepath.Join(dir, "websudo")
	askpassPath := filepath.Join(dir, "websudo-askpass")
	writeExecutable(t, websudoPath)
	writeExecutable(t, askpassPath)
	return websudoPath, askpassPath
}

func writeExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
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
