package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsUseLocalhostAndTenMinuteTimeout(t *testing.T) {
	t.Setenv("WEBSUDO_ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))

	cfg := Default()

	if cfg.WebAddr != "127.0.0.1:17878" {
		t.Fatalf("unexpected web addr: %q", cfg.WebAddr)
	}
	if cfg.ApprovalTimeoutSeconds != 600 {
		t.Fatalf("unexpected timeout: %d", cfg.ApprovalTimeoutSeconds)
	}
	if cfg.SudoPath != "/usr/bin/sudo" {
		t.Fatalf("sudo path = %q, want %q", cfg.SudoPath, "/usr/bin/sudo")
	}
	if cfg.AskpassPath != "" {
		t.Fatalf("askpass path = %q, want empty default for PATH lookup", cfg.AskpassPath)
	}
}

func TestDefaultsHonorEnvironmentOverrides(t *testing.T) {
	t.Setenv("WEBSUDO_WEB_ADDR", "127.0.0.1:19999")
	t.Setenv("WEBSUDO_APPROVAL_TIMEOUT_SECONDS", "42")
	t.Setenv("WEBSUDO_SUDO_PATH", "/custom/sudo")
	t.Setenv("WEBSUDO_ASKPASS_PATH", "/custom/websudo-askpass")

	cfg := Default()

	if cfg.WebAddr != "127.0.0.1:19999" {
		t.Fatalf("web addr = %q, want %q", cfg.WebAddr, "127.0.0.1:19999")
	}
	if cfg.ApprovalTimeoutSeconds != 42 {
		t.Fatalf("approval timeout = %d, want %d", cfg.ApprovalTimeoutSeconds, 42)
	}
	if cfg.SudoPath != "/custom/sudo" {
		t.Fatalf("sudo path = %q, want %q", cfg.SudoPath, "/custom/sudo")
	}
	if cfg.AskpassPath != "/custom/websudo-askpass" {
		t.Fatalf("askpass path = %q, want %q", cfg.AskpassPath, "/custom/websudo-askpass")
	}
}

func TestDefaultsLoadEnvironmentFileOverrides(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), "websudo.env")
	if err := os.WriteFile(envPath, []byte(fmt.Sprintf("WEBSUDO_WEB_ADDR=127.0.0.1:19999\nWEBSUDO_APPROVAL_TIMEOUT_SECONDS=12\nWEBSUDO_SUDO_PATH=%s\nWEBSUDO_ASKPASS_PATH=%s\n", "/env/sudo", "/env/websudo-askpass")), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("WEBSUDO_ENV_FILE", envPath)
	t.Setenv("WEBSUDO_WEB_ADDR", "")
	t.Setenv("WEBSUDO_APPROVAL_TIMEOUT_SECONDS", "")
	t.Setenv("WEBSUDO_SUDO_PATH", "")
	t.Setenv("WEBSUDO_ASKPASS_PATH", "")

	cfg := Default()

	if cfg.WebAddr != "127.0.0.1:19999" {
		t.Fatalf("web addr = %q, want %q", cfg.WebAddr, "127.0.0.1:19999")
	}
	if cfg.ApprovalTimeoutSeconds != 12 {
		t.Fatalf("approval timeout = %d, want %d", cfg.ApprovalTimeoutSeconds, 12)
	}
	if cfg.SudoPath != "/env/sudo" {
		t.Fatalf("sudo path = %q, want %q", cfg.SudoPath, "/env/sudo")
	}
	if cfg.AskpassPath != "/env/websudo-askpass" {
		t.Fatalf("askpass path = %q, want %q", cfg.AskpassPath, "/env/websudo-askpass")
	}
}
