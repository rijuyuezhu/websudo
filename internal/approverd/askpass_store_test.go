package approverd

import (
	"encoding/json"
	"fmt"
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
	req, result := store.Create("[sudo] password for alice:", provenance)
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

func TestAskpassStoreMovesCompletedRequestToSanitizedHistory(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newAskpassStoreForTest(func() time.Time { return now }, func() string { return "askpass-2" })
	_, _ = store.Create("Password:", AskpassProvenance{})
	now = now.Add(time.Second)
	if _, err := store.Complete("askpass-2", "secret"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	got, err := store.Get("askpass-2")
	if err != nil {
		t.Fatalf("Get(completed) error = %v", err)
	}
	if got.Status != AskpassCompleted || got.FinishedAt == nil {
		t.Fatalf("Get(completed) = %#v", got)
	}
	if pending := store.ListPending(); len(pending) != 0 {
		t.Fatalf("pending = %#v, want none after completion", pending)
	}
	recent := store.ListRecent()
	if len(recent) != 1 {
		t.Fatalf("recent len = %d, want 1", len(recent))
	}
	if recent[0].Status != AskpassCompleted || recent[0].FinishedAt == nil || !recent[0].FinishedAt.Equal(now) {
		t.Fatalf("recent request = %#v, want completed at %s", recent[0], now)
	}
	if strings.Contains(recent[0].Prompt, "secret") {
		t.Fatalf("history unexpectedly exposed password: %#v", recent[0])
	}
	encoded, err := json.Marshal(recent)
	if err != nil {
		t.Fatalf("Marshal(history) error = %v", err)
	}
	if strings.Contains(string(encoded), "secret") {
		t.Fatalf("history JSON unexpectedly contains password: %s", encoded)
	}
}

func TestAskpassStoreDenyDeliversTerminalResult(t *testing.T) {
	store := newAskpassStoreForTest(time.Now, func() string { return "askpass-deny" })
	req, result := store.Create("Password:", AskpassProvenance{})

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
	recent := store.ListRecent()
	if len(recent) != 1 || recent[0].Status != AskpassDenied || recent[0].FinishedAt == nil {
		t.Fatalf("recent = %#v, want one denied record", recent)
	}
}

func TestAskpassStoreRejectsRepeatedTerminalActions(t *testing.T) {
	store := newAskpassStoreForTest(time.Now, func() string { return "askpass-terminal" })
	_, _ = store.Create("Password:", AskpassProvenance{})
	if _, err := store.Deny("askpass-terminal"); err != nil {
		t.Fatalf("Deny() error = %v", err)
	}
	if _, err := store.Deny("askpass-terminal"); err == nil || !strings.Contains(err.Error(), "not pending") {
		t.Fatalf("second Deny() error = %v, want not pending", err)
	}
	if _, err := store.Complete("askpass-terminal", "secret"); err == nil || !strings.Contains(err.Error(), "not pending") {
		t.Fatalf("Complete(denied) error = %v, want not pending", err)
	}
}

func TestAskpassStoreActivelyExpiresPendingRequest(t *testing.T) {
	store := newAskpassStoreForTest(time.Now, func() string { return "askpass-pending-expire" })
	store.setExpirationTimeout(20 * time.Millisecond)
	_, result := store.Create("Password:", AskpassProvenance{})

	select {
	case outcome := <-result:
		if outcome.status != AskpassExpired {
			t.Fatalf("outcome status = %q, want expired", outcome.status)
		}
	case <-time.After(time.Second):
		t.Fatal("pending request did not expire")
	}
	if pending := store.ListPending(); len(pending) != 0 {
		t.Fatalf("pending = %#v, want none after expiration", pending)
	}
	recent := store.ListRecent()
	if len(recent) != 1 || recent[0].Status != AskpassExpired || recent[0].FinishedAt == nil {
		t.Fatalf("recent = %#v, want one expired record", recent)
	}
}

func TestAskpassStoreHistoryIsBoundedNewestFirst(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	now := base
	nextID := 0
	store := newAskpassStoreForTest(func() time.Time { return now }, func() string {
		nextID++
		return fmt.Sprintf("askpass-%03d", nextID)
	})

	for i := 0; i < askpassHistoryLimit+5; i++ {
		now = base.Add(time.Duration(i) * time.Second)
		req, _ := store.Create("Password:", AskpassProvenance{})
		now = now.Add(time.Millisecond)
		if _, err := store.Deny(req.ID); err != nil {
			t.Fatalf("Deny(%s) error = %v", req.ID, err)
		}
	}

	recent := store.ListRecent()
	if len(recent) != askpassHistoryLimit {
		t.Fatalf("recent len = %d, want %d", len(recent), askpassHistoryLimit)
	}
	if recent[0].ID != "askpass-055" || recent[len(recent)-1].ID != "askpass-006" {
		t.Fatalf("recent bounds = %q ... %q", recent[0].ID, recent[len(recent)-1].ID)
	}
}

func TestAskpassStoreListsOnlyPending(t *testing.T) {
	ids := []string{"askpass-a", "askpass-b"}
	store := newAskpassStoreForTest(func() time.Time { return time.Now().UTC() }, func() string {
		id := ids[0]
		ids = ids[1:]
		return id
	})
	_, _ = store.Create("a", AskpassProvenance{})
	_, _ = store.Create("b", AskpassProvenance{})
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

	_, _ = store.Create("oldest", AskpassProvenance{})
	now = base.Add(2 * time.Second)
	_, _ = store.Create("newest", AskpassProvenance{})
	now = base.Add(time.Second)
	_, _ = store.Create("middle", AskpassProvenance{})

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

func TestAskpassStoreListPendingBreaksTimestampTiesByID(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	ids := []string{"askpass-b", "askpass-a"}
	store := newAskpassStoreForTest(func() time.Time { return now }, func() string {
		id := ids[0]
		ids = ids[1:]
		return id
	})

	_, _ = store.Create("b", AskpassProvenance{})
	_, _ = store.Create("a", AskpassProvenance{})
	pending := store.ListPending()
	if len(pending) != 2 || pending[0].ID != "askpass-a" || pending[1].ID != "askpass-b" {
		t.Fatalf("pending = %#v, want deterministic ID tie-break", pending)
	}
}
