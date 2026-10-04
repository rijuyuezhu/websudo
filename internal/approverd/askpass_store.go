package approverd

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
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

type AskpassProvenance struct {
	Command []string `json:"command"`
	CWD     string   `json:"cwd"`
}

type AskpassRequest struct {
	ID         string            `json:"id"`
	Prompt     string            `json:"prompt"`
	Provenance AskpassProvenance `json:"provenance"`
	CreatedAt  time.Time         `json:"createdAt"`
	Status     AskpassStatus     `json:"status"`
}

type askpassResult struct {
	status   AskpassStatus
	password string
}

type askpassEntry struct {
	request AskpassRequest
	result  chan askpassResult
}

type AskpassStore struct {
	mu                sync.Mutex
	now               func() time.Time
	newID             func() string
	expirationTimeout time.Duration
	items             map[string]askpassEntry
	order             []string
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

func (s *AskpassStore) setExpirationTimeout(timeout time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.expirationTimeout = timeout
	if timeout <= 0 {
		return
	}
	for _, entry := range s.items {
		if entry.request.Status == AskpassPending || entry.request.Status == AskpassCompleted {
			s.scheduleExpirationLocked(entry.request)
		}
	}
}

func (s *AskpassStore) Create(prompt string, provenance AskpassProvenance) AskpassRequest {
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
		ID:         id,
		Prompt:     prompt,
		Provenance: provenance,
		CreatedAt:  s.now().UTC(),
		Status:     AskpassPending,
	}
	s.items[id] = askpassEntry{
		request: req,
		result:  make(chan askpassResult, 1),
	}
	s.order = append(s.order, id)
	s.scheduleExpirationLocked(req)
	return req
}

func (s *AskpassStore) Result(id string) (<-chan askpassResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.items[id]
	if !ok {
		return nil, errors.New("askpass request not found")
	}
	return entry.result, nil
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
	s.items[id] = entry
	entry.result <- askpassResult{status: AskpassCompleted, password: password}
	entry = s.items[id]
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
	entry.result <- askpassResult{status: AskpassDenied}
	return entry.request, nil
}

func (s *AskpassStore) ExpireBefore(cutoff time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	expired := 0
	for _, id := range s.order {
		entry, ok := s.items[id]
		if !ok || entry.request.CreatedAt.After(cutoff) {
			continue
		}
		if entry.request.Status != AskpassPending && entry.request.Status != AskpassCompleted {
			continue
		}
		wasPending := entry.request.Status == AskpassPending
		entry.request.Status = AskpassExpired
		s.items[id] = entry
		if wasPending {
			entry.result <- askpassResult{status: AskpassExpired}
		}
		expired++
	}
	return expired
}

func (s *AskpassStore) scheduleExpirationLocked(req AskpassRequest) {
	timeout := s.expirationTimeout
	if timeout <= 0 {
		return
	}
	delay := req.CreatedAt.Add(timeout).Sub(s.now().UTC())
	if delay <= 0 {
		s.expireLocked(req.ID, req.CreatedAt)
		return
	}
	time.AfterFunc(delay, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.expireLocked(req.ID, req.CreatedAt)
	})
}

func (s *AskpassStore) expireLocked(id string, createdAt time.Time) bool {
	entry, ok := s.items[id]
	if !ok || !entry.request.CreatedAt.Equal(createdAt) {
		return false
	}
	if entry.request.Status != AskpassPending && entry.request.Status != AskpassCompleted {
		return false
	}
	wasPending := entry.request.Status == AskpassPending
	entry.request.Status = AskpassExpired
	s.items[id] = entry
	if wasPending {
		entry.result <- askpassResult{status: AskpassExpired}
	}
	return true
}

func randomAskpassID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return "askpass-" + hex.EncodeToString(b[:])
}
