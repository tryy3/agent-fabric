package runtime

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Session struct {
	ID       string
	Pin      SessionPin
	Messages []Message
}

type Store struct {
	mu       sync.RWMutex
	sessions map[string]Session
}

func NewStore() *Store {
	return &Store{sessions: make(map[string]Session)}
}

func (s *Store) Create(pin SessionPin) (string, error) {
	id, err := newID()
	if err != nil {
		return "", err
	}
	if pin.Models != nil {
		models := make([]ModelRef, len(pin.Models))
		copy(models, pin.Models)
		pin.Models = models
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = Session{ID: id, Pin: pin}
	return id, nil
}

func (s *Store) SetCurrentModel(id, model string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
	for _, m := range sess.Pin.Models {
		if m.ID == model {
			sess.Pin.CurrentModel = model
			s.sessions[id] = sess
			return nil
		}
	}
	return fmt.Errorf("model %q not in session pin", model)
}

func (s *Store) Get(id string) (Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	return sess, ok
}

func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

func (s *Store) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

func (s *Store) Append(id string, msg Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
	sess.Messages = append(sess.Messages, msg)
	s.sessions[id] = sess
	return nil
}

func (s *Store) Messages(id string) ([]Message, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, false
	}
	out := make([]Message, len(sess.Messages))
	copy(out, sess.Messages)
	return out, true
}

func newID() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("session id: %w", err)
	}
	return "sess_" + hex.EncodeToString(b[:]), nil
}
