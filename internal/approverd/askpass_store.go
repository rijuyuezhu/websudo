package approverd

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

type AskpassStatus string

const (
	AskpassPending   AskpassStatus = "pending"
	AskpassCompleted AskpassStatus = "completed"
	AskpassDenied    AskpassStatus = "denied"
	AskpassExpired   AskpassStatus = "expired"
)

type AskpassRequest struct {
	ID        string        `json:"id"`
	Prompt    string        `json:"prompt"`
	CreatedAt time.Time     `json:"createdAt"`
	Status    AskpassStatus `json:"status"`
}

type askpassEntry struct {
	request  AskpassRequest
	password string
}

type AskpassStore struct {
	mu    sync.Mutex
	now   func() time.Time
	newID func() string
	items map[string]askpassEntry
	order []string
}

func NewAskpassStore() *AskpassStore {
	return newAskpassStoreForTest(time.Now, randomAskpassID)
}

func newAskpassStoreForTest(now func() time.Time, newID func() string) *AskpassStore {
	return &AskpassStore{
		now:   now,
		newID: newID,
		items: make(map[string]askpassEntry),
	}
}

func (s *AskpassStore) Create(prompt string) AskpassRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.newID()
	for {
		if _, exists := s.items[id]; !exists {
			break
		}
		id = s.newID()
	}
	req := AskpassRequest{
		ID:        id,
		Prompt:    prompt,
		CreatedAt: s.now().UTC(),
		Status:    AskpassPending,
	}
	s.items[id] = askpassEntry{request: req}
	s.order = append(s.order, id)
	return req
}

func (s *AskpassStore) Get(id string) (AskpassRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.items[id]
	if !ok {
		return AskpassRequest{}, errors.New("askpass request not found")
	}
	return entry.request, nil
}

func (s *AskpassStore) ListPending() []AskpassRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	pending := make([]AskpassRequest, 0)
	for _, id := range s.order {
		entry, ok := s.items[id]
		if !ok || entry.request.Status != AskpassPending {
			continue
		}
		pending = append(pending, entry.request)
	}
	sort.SliceStable(pending, func(i, j int) bool {
		return pending[i].CreatedAt.After(pending[j].CreatedAt)
	})
	return pending
}

func (s *AskpassStore) Complete(id, password string) (AskpassRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.items[id]
	if !ok {
		return AskpassRequest{}, errors.New("askpass request not found")
	}
	if entry.request.Status != AskpassPending {
		return AskpassRequest{}, errors.New("askpass request is not pending")
	}
	entry.request.Status = AskpassCompleted
	entry.password = password
	s.items[id] = entry
	return entry.request, nil
}

func (s *AskpassStore) Deny(id string) (AskpassRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.items[id]
	if !ok {
		return AskpassRequest{}, errors.New("askpass request not found")
	}
	if entry.request.Status != AskpassPending {
		return AskpassRequest{}, errors.New("askpass request is not pending")
	}
	entry.request.Status = AskpassDenied
	s.items[id] = entry
	return entry.request, nil
}

func (s *AskpassStore) Consume(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.items[id]
	if !ok {
		return "", errors.New("askpass request not found")
	}
	if entry.request.Status != AskpassCompleted {
		return "", fmt.Errorf("askpass request is %s", entry.request.Status)
	}
	delete(s.items, id)
	for i, orderedID := range s.order {
		if orderedID == id {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	return entry.password, nil
}

func (s *AskpassStore) ExpireBefore(cutoff time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	expired := 0
	for _, id := range s.order {
		entry, ok := s.items[id]
		if !ok || entry.request.Status != AskpassPending || entry.request.CreatedAt.After(cutoff) {
			continue
		}
		entry.request.Status = AskpassExpired
		s.items[id] = entry
		expired++
	}
	return expired
}

func randomAskpassID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return "askpass-" + hex.EncodeToString(b[:])
}
