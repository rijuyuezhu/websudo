package approverd

import (
	"errors"
	"testing"
	"time"
)

func TestSessionStoreCreateValidateAndDelete(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	store := newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) { return "session-1", nil })

	id, expiresAt, err := store.Create()
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if id != "session-1" {
		t.Fatalf("id = %q, want %q", id, "session-1")
	}
	if !expiresAt.Equal(now.Add(72 * time.Hour)) {
		t.Fatalf("expiresAt = %s, want %s", expiresAt, now.Add(72*time.Hour))
	}
	if !store.Valid("session-1") {
		t.Fatal("created session should be valid")
	}

	store.Delete("session-1")
	if store.Valid("session-1") {
		t.Fatal("deleted session should be invalid")
	}
}

func TestSessionStoreExpiresSessions(t *testing.T) {
	current := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	store := newSessionStoreForTest(72*time.Hour, func() time.Time { return current }, func() (string, error) { return "session-expire", nil })

	if _, _, err := store.Create(); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	current = current.Add(72*time.Hour - time.Nanosecond)
	if !store.Valid("session-expire") {
		t.Fatal("session should be valid before absolute expiry")
	}

	current = current.Add(time.Nanosecond)
	if store.Valid("session-expire") {
		t.Fatal("session should be invalid at absolute expiry")
	}
	if _, ok := store.sessions["session-expire"]; ok {
		t.Fatal("expired session should be removed from store")
	}
}

func TestSessionStoreReturnsIDGenerationError(t *testing.T) {
	store := newSessionStoreForTest(72*time.Hour, time.Now, func() (string, error) { return "", errors.New("entropy unavailable") })

	_, _, err := store.Create()
	if err == nil || err.Error() != "entropy unavailable" {
		t.Fatalf("Create() error = %v, want entropy unavailable", err)
	}
}
