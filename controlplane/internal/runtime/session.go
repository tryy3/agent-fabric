package runtime

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
)

type Session struct {
	ID         string
	Definition Definition
}

type Store struct {
	mu       sync.RWMutex
	sessions map[string]Session
}

func NewStore() *Store {
	return &Store{sessions: make(map[string]Session)}
}

func (s *Store) Create(def Definition) (string, error) {
	id, err := newID()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = Session{ID: id, Definition: def}
	return id, nil
}

func (s *Store) Get(id string) (Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	return sess, ok
}

func (s *Store) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

func newID() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("session id: %w", err)
	}
	return "sess_" + hex.EncodeToString(b[:]), nil
}
