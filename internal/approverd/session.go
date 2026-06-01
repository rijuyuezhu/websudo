package approverd

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

const sessionTTL = 72 * time.Hour

type session struct {
	expiresAt time.Time
}

type SessionStore struct {
	mu       sync.Mutex
	ttl      time.Duration
	now      func() time.Time
	newID    func() (string, error)
	sessions map[string]session
}

func NewSessionStore() *SessionStore {
	return newSessionStoreForTest(sessionTTL, time.Now, randomSessionID)
}

func newSessionStoreForTest(ttl time.Duration, now func() time.Time, newID func() (string, error)) *SessionStore {
	return &SessionStore{
		ttl:      ttl,
		now:      now,
		newID:    newID,
		sessions: make(map[string]session),
	}
}

func (s *SessionStore) Create() (string, time.Time, error) {
	id, err := s.newID()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := s.now().UTC().Add(s.ttl)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = session{expiresAt: expiresAt}
	return id, expiresAt, nil
}

func (s *SessionStore) Valid(id string) bool {
	if id == "" {
		return false
	}
	now := s.now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.sessions[id]
	if !ok {
		return false
	}
	if !now.Before(stored.expiresAt) {
		delete(s.sessions, id)
		return false
	}
	return true
}

func (s *SessionStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

func randomSessionID() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
