package approverd

import (
	"strings"
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
	token, err := store.ConsumeToken("askpass-1")
	if err != nil {
		t.Fatalf("ConsumeToken() error = %v", err)
	}

	password, err := store.Consume("askpass-1", token)
	if err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	if password != "secret" {
		t.Fatalf("password = %q, want secret", password)
	}
	if _, err := store.Consume("askpass-1", token); err == nil {
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
	denyToken, err := store.ConsumeToken("askpass-deny")
	if err != nil {
		t.Fatalf("ConsumeToken() error = %v", err)
	}
	if _, err := store.Consume("askpass-deny", denyToken); err == nil {
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

func TestAskpassStoreConsumeReportsCurrentStatus(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	current := now.Add(time.Second)
	ids := []string{"askpass-pending", "askpass-denied", "askpass-expired"}
	store := newAskpassStoreForTest(func() time.Time { return current }, func() string {
		id := ids[0]
		ids = ids[1:]
		return id
	})

	store.Create("pending")
	pendingToken, err := store.ConsumeToken("askpass-pending")
	if err != nil {
		t.Fatalf("ConsumeToken() error = %v", err)
	}
	if _, err := store.Consume("askpass-pending", pendingToken); err == nil || !strings.Contains(err.Error(), string(AskpassPending)) {
		t.Fatalf("Consume(pending) error = %v, want status %q", err, AskpassPending)
	}

	current = now
	store.Create("denied")
	if _, err := store.Deny("askpass-denied"); err != nil {
		t.Fatalf("Deny() error = %v", err)
	}
	store.Create("expired")
	if expired := store.ExpireBefore(now); expired != 1 {
		t.Fatalf("expired = %d, want 1", expired)
	}

	for _, tc := range []struct {
		id     string
		status AskpassStatus
	}{
		{id: "askpass-denied", status: AskpassDenied},
		{id: "askpass-expired", status: AskpassExpired},
	} {
		token, err := store.ConsumeToken(tc.id)
		if err != nil {
			t.Fatalf("ConsumeToken(%q) error = %v", tc.id, err)
		}
		if _, err := store.Consume(tc.id, token); err == nil || !strings.Contains(err.Error(), string(tc.status)) {
			t.Fatalf("Consume(%q) error = %v, want status %q", tc.id, err, tc.status)
		}
	}
}

func TestAskpassStoreConsumeRequiresValidTokenWithoutConsuming(t *testing.T) {
	store := newAskpassStoreForTest(func() time.Time { return time.Now().UTC() }, func() string { return "askpass-token" })
	store.Create("Password:")
	if _, err := store.Complete("askpass-token", "secret"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	token, err := store.ConsumeToken("askpass-token")
	if err != nil {
		t.Fatalf("ConsumeToken() error = %v", err)
	}

	for _, badToken := range []string{"", "wrong"} {
		if _, err := store.Consume("askpass-token", badToken); err == nil || !strings.Contains(err.Error(), "invalid") {
			t.Fatalf("Consume(invalid token %q) error = %v, want invalid token", badToken, err)
		}
	}

	password, err := store.Consume("askpass-token", token)
	if err != nil {
		t.Fatalf("Consume(valid token) error = %v", err)
	}
	if password != "secret" {
		t.Fatalf("password = %q, want secret", password)
	}
}

func TestAskpassStoreExpiresCompletedRequestsAndClearsPassword(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newAskpassStoreForTest(func() time.Time { return now }, func() string { return "askpass-completed-expire" })
	store.Create("Password:")
	token, err := store.ConsumeToken("askpass-completed-expire")
	if err != nil {
		t.Fatalf("ConsumeToken() error = %v", err)
	}
	if _, err := store.Complete("askpass-completed-expire", "secret"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if expired := store.ExpireBefore(now); expired != 1 {
		t.Fatalf("expired = %d, want 1", expired)
	}
	req, err := store.Get("askpass-completed-expire")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if req.Status != AskpassExpired {
		t.Fatalf("status = %q, want expired", req.Status)
	}
	if _, err := store.Consume("askpass-completed-expire", token); err == nil || !strings.Contains(err.Error(), string(AskpassExpired)) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("Consume(expired completed) error = %v, want expired without password", err)
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

func TestAskpassStoreListPendingNewestFirst(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	now := base
	ids := []string{"askpass-oldest", "askpass-newest", "askpass-middle"}
	store := newAskpassStoreForTest(func() time.Time { return now }, func() string {
		id := ids[0]
		ids = ids[1:]
		return id
	})

	store.Create("oldest")
	now = base.Add(2 * time.Second)
	store.Create("newest")
	now = base.Add(time.Second)
	store.Create("middle")

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
