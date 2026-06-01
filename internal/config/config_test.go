package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsUseLocalhostAndTenMinuteTimeout(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("WEBSUDO_ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
	t.Setenv("WEBSUDO_DATABASE_PATH", "")
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("WEBSUDO_ROOT_SOCKET_PATH", "")
	t.Setenv("WEBSUDO_ROOT_ALLOWED_UID", "")

	cfg := Default()

	if cfg.WebAddr != "127.0.0.1:17878" {
		t.Fatalf("unexpected web addr: %q", cfg.WebAddr)
	}
	if cfg.ApprovalTimeoutSeconds != 600 {
		t.Fatalf("unexpected timeout: %d", cfg.ApprovalTimeoutSeconds)
	}
	if cfg.TTYTimeoutSeconds != 300 {
		t.Fatalf("unexpected tty timeout: %d", cfg.TTYTimeoutSeconds)
	}
	if !VerifyToken(cfg.TokenHashHex, "123456") {
		t.Fatalf("expected default token hash to verify the default 6-digit token")
	}
	if cfg.DatabasePath != filepath.Join(homeDir, ".websudo", "websudo.db") {
		t.Fatalf("database path = %q, want %q", cfg.DatabasePath, filepath.Join(homeDir, ".websudo", "websudo.db"))
	}
	if cfg.TimestampDir != filepath.Join(homeDir, ".websudo", "timestamps") {
		t.Fatalf("timestamp dir = %q, want %q", cfg.TimestampDir, filepath.Join(homeDir, ".websudo", "timestamps"))
	}
}

func TestVerifyTokenMatchesHash(t *testing.T) {
	hash := MustHashToken("123456")
	if !VerifyToken(hash, "123456") {
		t.Fatalf("expected token to verify")
	}
	if VerifyToken(hash, "654321") {
		t.Fatalf("expected wrong token to fail")
	}
}

func TestDefaultsHonorEnvironmentOverrides(t *testing.T) {
	t.Setenv("WEBSUDO_ROOT_SOCKET_PATH", filepath.Join(t.TempDir(), "websudo-rootd.sock"))
	t.Setenv("WEBSUDO_ROOT_ALLOWED_UID", "1234")
	t.Setenv("WEBSUDO_APPROVAL_TIMEOUT_SECONDS", "42")
	t.Setenv("WEBSUDO_TTY_TIMEOUT_SECONDS", "99")
	t.Setenv("WEBSUDO_TIMESTAMP_DIR", filepath.Join(t.TempDir(), "timestamps"))

	cfg := Default()

	if cfg.RootSocketPath != os.Getenv("WEBSUDO_ROOT_SOCKET_PATH") {
		t.Fatalf("root socket path = %q, want %q", cfg.RootSocketPath, os.Getenv("WEBSUDO_ROOT_SOCKET_PATH"))
	}
	if cfg.RootAllowedUID != 1234 {
		t.Fatalf("root allowed uid = %d, want %d", cfg.RootAllowedUID, 1234)
	}
	if cfg.ApprovalTimeoutSeconds != 42 {
		t.Fatalf("approval timeout = %d, want %d", cfg.ApprovalTimeoutSeconds, 42)
	}
	if cfg.TTYTimeoutSeconds != 99 {
		t.Fatalf("tty timeout = %d, want %d", cfg.TTYTimeoutSeconds, 99)
	}
	if cfg.TimestampDir != os.Getenv("WEBSUDO_TIMESTAMP_DIR") {
		t.Fatalf("timestamp dir = %q, want %q", cfg.TimestampDir, os.Getenv("WEBSUDO_TIMESTAMP_DIR"))
	}
}

func TestDefaultsUseUserRuntimeSocketWhenAvailable(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join("/run/user", "1234"))
	t.Setenv("WEBSUDO_ROOT_SOCKET_PATH", "")
	t.Setenv("WEBSUDO_ROOT_ALLOWED_UID", "")

	cfg := Default()

	if cfg.RootSocketPath != "/run/user/1234/websudo-rootd.sock" {
		t.Fatalf("root socket path = %q, want %q", cfg.RootSocketPath, "/run/user/1234/websudo-rootd.sock")
	}
	if cfg.TimestampDir != "/run/user/1234/websudo" {
		t.Fatalf("timestamp dir = %q, want %q", cfg.TimestampDir, "/run/user/1234/websudo")
	}
	if cfg.RootAllowedUID != 1234 {
		t.Fatalf("root allowed uid = %d, want %d", cfg.RootAllowedUID, 1234)
	}
}

func TestDefaultsDeriveAllowedUIDFromConfiguredRuntimeSocket(t *testing.T) {
	t.Setenv("WEBSUDO_ROOT_SOCKET_PATH", "/run/user/2345/websudo-rootd.sock")
	t.Setenv("WEBSUDO_ROOT_ALLOWED_UID", "")

	cfg := Default()

	if cfg.RootAllowedUID != 2345 {
		t.Fatalf("root allowed uid = %d, want %d", cfg.RootAllowedUID, 2345)
	}
}

func TestDefaultsLoadEnvironmentFileOverrides(t *testing.T) {
	homeDir := t.TempDir()
	envPath := filepath.Join(t.TempDir(), "websudo.env")
	if err := os.WriteFile(envPath, []byte(fmt.Sprintf("WEBSUDO_WEB_ADDR=127.0.0.1:19999\nWEBSUDO_DATABASE_PATH=%s\nWEBSUDO_TTY_TIMEOUT_SECONDS=12\nWEBSUDO_TIMESTAMP_DIR=%s\n", filepath.Join(homeDir, ".websudo", "from-env.db"), filepath.Join(homeDir, ".websudo", "ts"))), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("HOME", homeDir)
	t.Setenv("WEBSUDO_ENV_FILE", envPath)
	t.Setenv("WEBSUDO_WEB_ADDR", "")
	t.Setenv("WEBSUDO_DATABASE_PATH", "")

	cfg := Default()

	if cfg.WebAddr != "127.0.0.1:19999" {
		t.Fatalf("web addr = %q, want %q", cfg.WebAddr, "127.0.0.1:19999")
	}
	if cfg.DatabasePath != filepath.Join(homeDir, ".websudo", "from-env.db") {
		t.Fatalf("database path = %q, want %q", cfg.DatabasePath, filepath.Join(homeDir, ".websudo", "from-env.db"))
	}
	if cfg.TTYTimeoutSeconds != 12 {
		t.Fatalf("tty timeout = %d, want %d", cfg.TTYTimeoutSeconds, 12)
	}
	if cfg.TimestampDir != filepath.Join(homeDir, ".websudo", "ts") {
		t.Fatalf("timestamp dir = %q, want %q", cfg.TimestampDir, filepath.Join(homeDir, ".websudo", "ts"))
	}
}

func TestDefaultsExposeSudoAskpassConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("WEBSUDO_ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
	t.Setenv("WEBSUDO_SUDO_PATH", "")
	t.Setenv("WEBSUDO_ASKPASS_PATH", "")

	cfg := Default()

	if cfg.SudoPath != "/usr/bin/sudo" {
		t.Fatalf("sudo path = %q, want %q", cfg.SudoPath, "/usr/bin/sudo")
	}
	if cfg.AskpassPath != "" {
		t.Fatalf("askpass path = %q, want empty default for PATH lookup", cfg.AskpassPath)
	}
}

func TestDefaultsHonorSudoAskpassEnvironmentOverrides(t *testing.T) {
	t.Setenv("WEBSUDO_SUDO_PATH", "/custom/sudo")
	t.Setenv("WEBSUDO_ASKPASS_PATH", "/custom/websudo-askpass")

	cfg := Default()

	if cfg.SudoPath != "/custom/sudo" {
		t.Fatalf("sudo path = %q, want %q", cfg.SudoPath, "/custom/sudo")
	}
	if cfg.AskpassPath != "/custom/websudo-askpass" {
		t.Fatalf("askpass path = %q, want %q", cfg.AskpassPath, "/custom/websudo-askpass")
	}
}
