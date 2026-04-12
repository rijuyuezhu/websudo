package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsUseLocalhostAndTenMinuteTimeout(t *testing.T) {
	cfg := Default()

	if cfg.WebAddr != "127.0.0.1:17878" {
		t.Fatalf("unexpected web addr: %q", cfg.WebAddr)
	}
	if cfg.ApprovalTimeoutSeconds != 600 {
		t.Fatalf("unexpected timeout: %d", cfg.ApprovalTimeoutSeconds)
	}
	if !VerifyToken(cfg.TokenHashHex, "123456") {
		t.Fatalf("expected default token hash to verify the default 6-digit token")
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
}

func TestDefaultsUseUserRuntimeSocketWhenAvailable(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join("/run/user", "1234"))
	t.Setenv("WEBSUDO_ROOT_SOCKET_PATH", "")
	t.Setenv("WEBSUDO_ROOT_ALLOWED_UID", "")

	cfg := Default()

	if cfg.RootSocketPath != "/run/user/1234/websudo-rootd.sock" {
		t.Fatalf("root socket path = %q, want %q", cfg.RootSocketPath, "/run/user/1234/websudo-rootd.sock")
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
