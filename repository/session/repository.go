package session

import (
	"fmt"
	"sort"
	"sync"

	"cposim/entity"
)

type Repository interface {
	Delete(sessionID string) error
	Get(sessionID string) (entity.Session, error)
	List() ([]entity.Session, error)
	Upsert(session entity.Session) error
}

type inMemoryRepository struct {
	sessionBySessionID map[string]entity.Session
	mu                 sync.RWMutex
}

func NewInMemoryRepository() Repository {
	return &inMemoryRepository{
		sessionBySessionID: map[string]entity.Session{},
	}
}

func (r *inMemoryRepository) Delete(sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.sessionBySessionID[sessionID]; !ok {
		return fmt.Errorf("session %q not found", sessionID)
	}

	delete(r.sessionBySessionID, sessionID)

	return nil
}

func (r *inMemoryRepository) Get(sessionID string) (entity.Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	session, ok := r.sessionBySessionID[sessionID]
	if !ok {
		return entity.Session{}, fmt.Errorf("session %q not found", sessionID)
	}

	return session, nil
}

// List returns entities ordered by ID, which for generated IDs is creation order.
func (r *inMemoryRepository) List() ([]entity.Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sessions := make([]entity.Session, 0, len(r.sessionBySessionID))
	for _, session := range r.sessionBySessionID {
		sessions = append(sessions, session)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].SessionID < sessions[j].SessionID
	})

	return sessions, nil
}

func (r *inMemoryRepository) Upsert(session entity.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if session.SessionID == "" {
		return fmt.Errorf("session has no ID")
	}

	r.sessionBySessionID[session.SessionID] = session

	return nil
}
