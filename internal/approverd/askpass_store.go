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

const askpassHistoryLimit = 50

type AskpassProvenance struct {
	Command []string `json:"command"`
	CWD     string   `json:"cwd"`
}

type AskpassRequest struct {
	ID         string            `json:"id"`
	Prompt     string            `json:"prompt"`
	Provenance AskpassProvenance `json:"provenance"`
	CreatedAt  time.Time         `json:"createdAt"`
	FinishedAt *time.Time        `json:"finishedAt,omitempty"`
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
	active            map[string]askpassEntry
	history           []AskpassRequest
}

func NewAskpassStore() *AskpassStore {
	return newAskpassStoreForTest(time.Now, randomAskpassID)
}

func newAskpassStoreForTest(now func() time.Time, newID func() string) *AskpassStore {
	return &AskpassStore{
		now:    now,
		newID:  newID,
		active: make(map[string]askpassEntry),
	}
}

func (s *AskpassStore) setExpirationTimeout(timeout time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.expirationTimeout = timeout
	if timeout <= 0 {
		return
	}
	for _, entry := range s.active {
		s.scheduleExpirationLocked(entry.request)
	}
}

func (s *AskpassStore) Create(prompt string, provenance AskpassProvenance) (AskpassRequest, <-chan askpassResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.newID()
	for {
		if _, exists := s.active[id]; !exists {
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
	entry := askpassEntry{
		request: req,
		result:  make(chan askpassResult, 1),
	}
	s.active[id] = entry
	s.scheduleExpirationLocked(req)
	return req, entry.result
}

func (s *AskpassStore) Get(id string) (AskpassRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, ok := s.active[id]; ok {
		return entry.request, nil
	}
	if req, ok := s.historyRequestLocked(id); ok {
		return req, nil
	}
	return AskpassRequest{}, errors.New("askpass request not found")
}

func (s *AskpassStore) ListPending() []AskpassRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	pending := make([]AskpassRequest, 0, len(s.active))
	for _, entry := range s.active {
		pending = append(pending, entry.request)
	}
	sort.Slice(pending, func(i, j int) bool {
		if pending[i].CreatedAt.Equal(pending[j].CreatedAt) {
			return pending[i].ID < pending[j].ID
		}
		return pending[i].CreatedAt.After(pending[j].CreatedAt)
	})
	return pending
}

func (s *AskpassStore) ListRecent() []AskpassRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	recent := make([]AskpassRequest, 0, len(s.history))
	for i := len(s.history) - 1; i >= 0; i-- {
		recent = append(recent, s.history[i])
	}
	return recent
}

func (s *AskpassStore) Complete(id, password string) (AskpassRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.active[id]
	if !ok {
		if _, historical := s.historyRequestLocked(id); historical {
			return AskpassRequest{}, errors.New("askpass request is not pending")
		}
		return AskpassRequest{}, errors.New("askpass request not found")
	}
	finishedAt := s.now().UTC()
	entry.request.Status = AskpassCompleted
	entry.request.FinishedAt = &finishedAt
	s.finishLocked(id, entry.request)
	entry.result <- askpassResult{status: AskpassCompleted, password: password}
	return entry.request, nil
}

func (s *AskpassStore) Deny(id string) (AskpassRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.active[id]
	if !ok {
		if _, historical := s.historyRequestLocked(id); historical {
			return AskpassRequest{}, errors.New("askpass request is not pending")
		}
		return AskpassRequest{}, errors.New("askpass request not found")
	}
	finishedAt := s.now().UTC()
	entry.request.Status = AskpassDenied
	entry.request.FinishedAt = &finishedAt
	s.finishLocked(id, entry.request)
	entry.result <- askpassResult{status: AskpassDenied}
	return entry.request, nil
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
	entry, ok := s.active[id]
	if !ok || !entry.request.CreatedAt.Equal(createdAt) {
		return false
	}
	finishedAt := s.now().UTC()
	entry.request.Status = AskpassExpired
	entry.request.FinishedAt = &finishedAt
	s.finishLocked(id, entry.request)
	entry.result <- askpassResult{status: AskpassExpired}
	return true
}

func (s *AskpassStore) finishLocked(id string, req AskpassRequest) {
	delete(s.active, id)
	s.history = append(s.history, req)
	if len(s.history) > askpassHistoryLimit {
		s.history = s.history[len(s.history)-askpassHistoryLimit:]
	}
}

func (s *AskpassStore) historyRequestLocked(id string) (AskpassRequest, bool) {
	for i := len(s.history) - 1; i >= 0; i-- {
		if s.history[i].ID == id {
			return s.history[i], true
		}
	}
	return AskpassRequest{}, false
}

func randomAskpassID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return "askpass-" + hex.EncodeToString(b[:])
}
