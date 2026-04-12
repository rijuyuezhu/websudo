package config

import "testing"

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
