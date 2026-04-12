package model

import (
	"reflect"
	"testing"
	"time"
)

func TestNewRequestFreezesCommandDetails(t *testing.T) {
	timestamp := time.Date(2026, 4, 12, 3, 0, 0, 0, time.UTC)
	argv := []string{"/usr/bin/pacman", "-Syu"}

	req := NewRequest(
		"req-1",
		timestamp,
		Requester{UID: 1000, GID: 1000, Username: "rijuyuezhu", Hostname: "rjyz-linux"},
		Command{ResolvedPath: "/usr/bin/pacman", Argv: argv, Cwd: "/tmp"},
	)

	argv[0] = "/usr/bin/false"

	if got := req.Command().ResolvedPath; got != "/usr/bin/pacman" {
		t.Fatalf("resolved path = %q, want %q", got, "/usr/bin/pacman")
	}

	if got := req.Command().Cwd; got != "/tmp" {
		t.Fatalf("cwd = %q, want %q", got, "/tmp")
	}

	if got := req.Command().Argv; !reflect.DeepEqual(got, []string{"/usr/bin/pacman", "-Syu"}) {
		t.Fatalf("argv = %v, want %v", got, []string{"/usr/bin/pacman", "-Syu"})
	}

	returned := req.Command().Argv
	returned[1] = "-Qi"

	if got := req.Command().Argv; !reflect.DeepEqual(got, []string{"/usr/bin/pacman", "-Syu"}) {
		t.Fatalf("argv mutated through accessor: got %v", got)
	}

	if got := req.Status(); got != StatusPending {
		t.Fatalf("status = %q, want %q", got, StatusPending)
	}
	if got := req.CreatedAt(); !got.Equal(timestamp) {
		t.Fatalf("createdAt = %v, want %v", got, timestamp)
	}
	if got := req.ID(); got != "req-1" {
		t.Fatalf("id = %q, want %q", got, "req-1")
	}
}

func TestRequestTransitionRejectsInvalidStateChange(t *testing.T) {
	req := NewRequest(
		"req-1",
		time.Date(2026, 4, 12, 3, 0, 0, 0, time.UTC),
		Requester{UID: 1000, GID: 1000, Username: "rijuyuezhu", Hostname: "rjyz-linux"},
		Command{ResolvedPath: "/usr/bin/pacman", Argv: []string{"/usr/bin/pacman", "-Syu"}, Cwd: "/tmp"},
	)

	if _, err := req.Transition(StatusRunning); err == nil {
		t.Fatal("Transition(StatusRunning) error = nil, want invalid transition error")
	}

	approved, err := req.Transition(StatusApproved)
	if err != nil {
		t.Fatalf("Transition(StatusApproved) error = %v, want nil", err)
	}

	if _, err := approved.Transition(StatusDenied); err == nil {
		t.Fatal("approved Transition(StatusDenied) error = nil, want invalid transition error")
	}

	running, err := approved.Transition(StatusRunning)
	if err != nil {
		t.Fatalf("Transition(StatusRunning) error = %v, want nil", err)
	}

	succeeded, err := running.Transition(StatusSucceeded)
	if err != nil {
		t.Fatalf("Transition(StatusSucceeded) error = %v, want nil", err)
	}

	if _, err := succeeded.Transition(StatusFailed); err == nil {
		t.Fatal("succeeded Transition(StatusFailed) error = nil, want invalid transition error")
	}
}

func TestNewStoredRequestPreservesStatus(t *testing.T) {
	req := NewStoredRequest(
		"req-3",
		time.Date(2026, 4, 12, 5, 0, 0, 0, time.UTC),
		Requester{},
		Command{ResolvedPath: "/usr/bin/true", Argv: []string{"/usr/bin/true"}, Cwd: "/tmp"},
		StatusApproved,
		nil,
	)

	if got := req.Status(); got != StatusApproved {
		t.Fatalf("status = %q, want %q", got, StatusApproved)
	}
}

func TestRequestWithResultStoresCompletedOutput(t *testing.T) {
	req := NewRequest(
		"req-4",
		time.Date(2026, 4, 12, 5, 10, 0, 0, time.UTC),
		Requester{},
		Command{ResolvedPath: "/usr/bin/true", Argv: []string{"/usr/bin/true"}, Cwd: "/tmp"},
	)
	approved, err := req.Transition(StatusApproved)
	if err != nil {
		t.Fatalf("Transition(StatusApproved) error = %v", err)
	}
	running, err := approved.Transition(StatusRunning)
	if err != nil {
		t.Fatalf("Transition(StatusRunning) error = %v", err)
	}
	completed, err := running.WithResult(Result{Stdout: "ok"})
	if err != nil {
		t.Fatalf("WithResult() error = %v", err)
	}
	if completed.Status() != StatusSucceeded {
		t.Fatalf("status = %q, want %q", completed.Status(), StatusSucceeded)
	}
	if completed.Result() == nil || completed.Result().Stdout != "ok" {
		t.Fatalf("result = %#v, want stdout to be preserved", completed.Result())
	}
}
