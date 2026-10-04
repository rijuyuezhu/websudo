package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadUsesDefaultsWhenFileIsMissing(t *testing.T) {
	cfg, err := load(filepath.Join(t.TempDir(), "missing.env"))
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}

	if cfg.WebAddr != "127.0.0.1:17878" {
		t.Fatalf("unexpected web addr: %q", cfg.WebAddr)
	}
	if cfg.ApprovalTimeout != 10*time.Minute {
		t.Fatalf("unexpected timeout: %v", cfg.ApprovalTimeout)
	}
	if cfg.SudoPath != "/usr/bin/sudo" {
		t.Fatalf("sudo path = %q, want %q", cfg.SudoPath, "/usr/bin/sudo")
	}
}

func TestLoadUsesDefaultsInsteadOfCallerEnvironmentWhenFileIsMissing(t *testing.T) {
	t.Setenv("WEBSUDO_ENV_FILE", filepath.Join(t.TempDir(), "attacker.env"))
	t.Setenv("WEBSUDO_WEB_ADDR", "attacker.invalid:9999")
	t.Setenv("WEBSUDO_APPROVAL_TIMEOUT_SECONDS", "1")
	t.Setenv("WEBSUDO_SUDO_PATH", "/tmp/fake-sudo")

	cfg, err := load(filepath.Join(t.TempDir(), "missing.env"))
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if cfg.WebAddr != "127.0.0.1:17878" {
		t.Fatalf("web addr = %q, want default", cfg.WebAddr)
	}
	if cfg.ApprovalTimeout != 10*time.Minute {
		t.Fatalf("approval timeout = %v, want default", cfg.ApprovalTimeout)
	}
	if cfg.SudoPath != "/usr/bin/sudo" {
		t.Fatalf("sudo path = %q, want default", cfg.SudoPath)
	}
}

func TestLoadUsesConfiguredFileValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "websudo.env")
	if err := os.WriteFile(path, []byte(
		"WEBSUDO_WEB_ADDR=127.0.0.1:19999\n"+
			"WEBSUDO_APPROVAL_TIMEOUT_SECONDS=12\n"+
			"WEBSUDO_SUDO_PATH=/usr/bin/sudo-rs\n",
	), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := load(path)
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if cfg.WebAddr != "127.0.0.1:19999" {
		t.Fatalf("web addr = %q, want %q", cfg.WebAddr, "127.0.0.1:19999")
	}
	if cfg.ApprovalTimeout != 12*time.Second {
		t.Fatalf("approval timeout = %v, want %v", cfg.ApprovalTimeout, 12*time.Second)
	}
	if cfg.SudoPath != "/usr/bin/sudo-rs" {
		t.Fatalf("sudo path = %q, want %q", cfg.SudoPath, "/usr/bin/sudo-rs")
	}
}

func TestLoadIgnoresCallerEnvironment(t *testing.T) {
	trustedPath := filepath.Join(t.TempDir(), "trusted.env")
	if err := os.WriteFile(trustedPath, []byte(
		"WEBSUDO_WEB_ADDR=127.0.0.1:18888\n"+
			"WEBSUDO_APPROVAL_TIMEOUT_SECONDS=30\n"+
			"WEBSUDO_SUDO_PATH=/usr/local/bin/sudo\n",
	), 0o600); err != nil {
		t.Fatalf("WriteFile(trusted) error = %v", err)
	}
	attackerPath := filepath.Join(t.TempDir(), "attacker.env")
	if err := os.WriteFile(attackerPath, []byte(
		"WEBSUDO_WEB_ADDR=attacker.invalid:9999\n"+
			"WEBSUDO_APPROVAL_TIMEOUT_SECONDS=1\n"+
			"WEBSUDO_SUDO_PATH=/tmp/fake-sudo\n",
	), 0o600); err != nil {
		t.Fatalf("WriteFile(attacker) error = %v", err)
	}

	t.Setenv("WEBSUDO_ENV_FILE", attackerPath)
	t.Setenv("WEBSUDO_WEB_ADDR", "attacker.invalid:9999")
	t.Setenv("WEBSUDO_APPROVAL_TIMEOUT_SECONDS", "1")
	t.Setenv("WEBSUDO_SUDO_PATH", "/tmp/fake-sudo")

	cfg, err := load(trustedPath)
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if cfg.WebAddr != "127.0.0.1:18888" {
		t.Fatalf("web addr = %q, want trusted file value", cfg.WebAddr)
	}
	if cfg.ApprovalTimeout != 30*time.Second {
		t.Fatalf("approval timeout = %v, want trusted file value", cfg.ApprovalTimeout)
	}
	if cfg.SudoPath != "/usr/local/bin/sudo" {
		t.Fatalf("sudo path = %q, want trusted file value", cfg.SudoPath)
	}
}

func TestLoadRejectsInvalidTimeout(t *testing.T) {
	for _, value := range []string{"0", "-1", "invalid", "", "9223372037"} {
		t.Run(value, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "websudo.env")
			if err := os.WriteFile(path, []byte("WEBSUDO_APPROVAL_TIMEOUT_SECONDS="+value+"\n"), 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			_, err := load(path)
			if err == nil || !strings.Contains(err.Error(), "must be a positive integer that fits a duration") {
				t.Fatalf("load() error = %v, want positive-integer error", err)
			}
		})
	}
}

func TestLoadRejectsRelativeSudoPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "websudo.env")
	if err := os.WriteFile(path, []byte("WEBSUDO_SUDO_PATH=sudo\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := load(path)
	if err == nil || !strings.Contains(err.Error(), "must be an absolute path") {
		t.Fatalf("load() error = %v, want absolute-path error", err)
	}
}

func TestLoadFailsWhenConfigCannotBeRead(t *testing.T) {
	_, err := load(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "read websudo config") {
		t.Fatalf("load() error = %v, want read error", err)
	}
}

func TestAskpassSocketPathUsesRunUser(t *testing.T) {
	want := filepath.Join("/run/user", fmt.Sprint(os.Getuid()), "websudo", "askpass.sock")
	if got := AskpassSocketPath(); got != want {
		t.Fatalf("AskpassSocketPath() = %q, want %q", got, want)
	}
}
