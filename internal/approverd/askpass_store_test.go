package approverd

import (
	"strings"
	"testing"
	"time"
)

func TestAskpassStoreCreateCompleteDeliversPassword(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newAskpassStoreForTest(func() time.Time { return now }, func() string { return "askpass-1" })

	provenance := AskpassProvenance{
		Command: []string{"/usr/bin/id", "-u"},
		CWD:     "/home/alice/project",
	}
	req := store.Create("[sudo] password for alice:", provenance)
	if req.ID != "askpass-1" {
		t.Fatalf("id = %q, want askpass-1", req.ID)
	}
	if req.Prompt != "[sudo] password for alice:" {
		t.Fatalf("prompt = %q", req.Prompt)
	}
	if len(req.Provenance.Command) != 2 || req.Provenance.Command[0] != "/usr/bin/id" || req.Provenance.CWD != "/home/alice/project" {
		t.Fatalf("provenance = %#v", req.Provenance)
	}
	if req.Status != AskpassPending {
		t.Fatalf("status = %q, want %q", req.Status, AskpassPending)
	}
	result, err := store.Result(req.ID)
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}

	completed, err := store.Complete(req.ID, "secret")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if completed.Status != AskpassCompleted {
		t.Fatalf("status = %q, want %q", completed.Status, AskpassCompleted)
	}
	outcome := <-result
	if outcome.status != AskpassCompleted || outcome.password != "secret" {
		t.Fatalf("outcome = %#v, want completed secret", outcome)
	}
}

func TestAskpassStoreDoesNotExposePasswordInGet(t *testing.T) {
	store := newAskpassStoreForTest(func() time.Time { return time.Now().UTC() }, func() string { return "askpass-2" })
	store.Create("Password:", AskpassProvenance{})
	if _, err := store.Complete("askpass-2", "secret"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	req, err := store.Get("askpass-2")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if req.Status != AskpassCompleted {
		t.Fatalf("status = %q, want completed", req.Status)
	}
	if strings.Contains(req.Prompt, "secret") {
		t.Fatalf("request unexpectedly exposed password: %#v", req)
	}
}

func TestAskpassStoreDenyDeliversTerminalResult(t *testing.T) {
	store := newAskpassStoreForTest(time.Now, func() string { return "askpass-deny" })
	req := store.Create("Password:", AskpassProvenance{})
	result, err := store.Result(req.ID)
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}

	denied, err := store.Deny(req.ID)
	if err != nil {
		t.Fatalf("Deny() error = %v", err)
	}
	if denied.Status != AskpassDenied {
		t.Fatalf("status = %q, want denied", denied.Status)
	}
	outcome := <-result
	if outcome.status != AskpassDenied || outcome.password != "" {
		t.Fatalf("outcome = %#v, want denied without password", outcome)
	}
}

func TestAskpassStoreRejectsRepeatedTerminalActions(t *testing.T) {
	store := newAskpassStoreForTest(time.Now, func() string { return "askpass-terminal" })
	store.Create("Password:", AskpassProvenance{})
	if _, err := store.Deny("askpass-terminal"); err != nil {
		t.Fatalf("Deny() error = %v", err)
	}
	if _, err := store.Deny("askpass-terminal"); err == nil {
		t.Fatal("second Deny() error = nil, want conflict")
	}
	if _, err := store.Complete("askpass-terminal", "secret"); err == nil {
		t.Fatal("Complete(denied) error = nil, want conflict")
	}
}

func TestAskpassStoreResultMissingRequest(t *testing.T) {
	store := NewAskpassStore()
	if _, err := store.Result("missing"); err == nil {
		t.Fatal("Result(missing) error = nil")
	}
}

func TestAskpassStoreActivelyExpiresCompletedMetadata(t *testing.T) {
	store := newAskpassStoreForTest(time.Now, func() string { return "askpass-active-expire" })
	store.setExpirationTimeout(20 * time.Millisecond)
	store.Create("Password:", AskpassProvenance{})
	if _, err := store.Complete("askpass-active-expire", "secret"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	time.Sleep(60 * time.Millisecond)
	req, err := store.Get("askpass-active-expire")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if req.Status != AskpassExpired {
		t.Fatalf("status = %q, want expired", req.Status)
	}
}

func TestAskpassStoreActivelyExpiresPendingRequest(t *testing.T) {
	store := newAskpassStoreForTest(time.Now, func() string { return "askpass-pending-expire" })
	store.setExpirationTimeout(20 * time.Millisecond)
	req := store.Create("Password:", AskpassProvenance{})
	result, err := store.Result(req.ID)
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}

	select {
	case outcome := <-result:
		if outcome.status != AskpassExpired {
			t.Fatalf("outcome status = %q, want expired", outcome.status)
		}
	case <-time.After(time.Second):
		t.Fatal("pending request did not expire")
	}
}

func TestAskpassStoreListsOnlyPending(t *testing.T) {
	ids := []string{"askpass-a", "askpass-b"}
	store := newAskpassStoreForTest(func() time.Time { return time.Now().UTC() }, func() string {
		id := ids[0]
		ids = ids[1:]
		return id
	})
	store.Create("a", AskpassProvenance{})
	store.Create("b", AskpassProvenance{})
	if _, err := store.Complete("askpass-b", "secret"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	pending := store.ListPending()
	if len(pending) != 1 || pending[0].ID != "askpass-a" {
		t.Fatalf("pending = %#v, want only askpass-a", pending)
	}
}

func TestAskpassStoreListPendingNewestFirst(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	now := base
	ids := []string{"askpass-oldest", "askpass-newest", "askpass-middle"}
	store := newAskpassStoreForTest(func() time.Time { return now }, func() string {
		id := ids[0]
		ids = ids[1:]
		return id
	})

	store.Create("oldest", AskpassProvenance{})
	now = base.Add(2 * time.Second)
	store.Create("newest", AskpassProvenance{})
	now = base.Add(time.Second)
	store.Create("middle", AskpassProvenance{})

	pending := store.ListPending()
	if len(pending) != 3 {
		t.Fatalf("pending len = %d, want 3", len(pending))
	}
	want := []string{"askpass-newest", "askpass-middle", "askpass-oldest"}
	for i, id := range want {
		if pending[i].ID != id {
			t.Fatalf("pending[%d].ID = %q, want %q; pending = %#v", i, pending[i].ID, id, pending)
		}
	}
}
