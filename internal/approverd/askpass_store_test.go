package approverd

import (
	"testing"
	"time"
)

func TestAskpassStoreCreateCompleteConsumeOnce(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newAskpassStoreForTest(func() time.Time { return now }, func() string { return "askpass-1" })

	req := store.Create("[sudo] password for alice:")
	if req.ID != "askpass-1" {
		t.Fatalf("id = %q, want askpass-1", req.ID)
	}
	if req.Prompt != "[sudo] password for alice:" {
		t.Fatalf("prompt = %q", req.Prompt)
	}
	if req.Status != AskpassPending {
		t.Fatalf("status = %q, want %q", req.Status, AskpassPending)
	}

	completed, err := store.Complete("askpass-1", "secret")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if completed.Status != AskpassCompleted {
		t.Fatalf("status = %q, want %q", completed.Status, AskpassCompleted)
	}

	password, err := store.Consume("askpass-1")
	if err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	if password != "secret" {
		t.Fatalf("password = %q, want secret", password)
	}
	if _, err := store.Consume("askpass-1"); err == nil {
		t.Fatal("second Consume() error = nil, want missing request")
	}
}

func TestAskpassStoreDoesNotExposePasswordInGet(t *testing.T) {
	store := newAskpassStoreForTest(func() time.Time { return time.Now().UTC() }, func() string { return "askpass-2" })
	store.Create("Password:")
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
}

func TestAskpassStoreDenyAndExpire(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	ids := []string{"askpass-deny", "askpass-expire"}
	store := newAskpassStoreForTest(func() time.Time { return now }, func() string {
		id := ids[0]
		ids = ids[1:]
		return id
	})

	store.Create("deny")
	denied, err := store.Deny("askpass-deny")
	if err != nil {
		t.Fatalf("Deny() error = %v", err)
	}
	if denied.Status != AskpassDenied {
		t.Fatalf("status = %q, want denied", denied.Status)
	}
	if _, err := store.Consume("askpass-deny"); err == nil {
		t.Fatal("Consume(denied) error = nil, want terminal status error")
	}

	store.Create("expire")
	expired := store.ExpireBefore(now.Add(time.Second))
	if expired != 1 {
		t.Fatalf("expired = %d, want 1", expired)
	}
	req, err := store.Get("askpass-expire")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if req.Status != AskpassExpired {
		t.Fatalf("status = %q, want expired", req.Status)
	}
}

func TestAskpassStoreListsOnlyPending(t *testing.T) {
	ids := []string{"askpass-a", "askpass-b"}
	store := newAskpassStoreForTest(func() time.Time { return time.Now().UTC() }, func() string {
		id := ids[0]
		ids = ids[1:]
		return id
	})
	store.Create("a")
	store.Create("b")
	if _, err := store.Complete("askpass-b", "secret"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	pending := store.ListPending()
	if len(pending) != 1 || pending[0].ID != "askpass-a" {
		t.Fatalf("pending = %#v, want only askpass-a", pending)
	}
}
