package session

import (
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Manager keeps track of all active sessions.
type Manager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewManager creates a new session manager.
func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
	}
}

// CreateSession upgrades a new WebSocket connection into a tracked Session.
func (m *Manager) CreateSession(conn *websocket.Conn) *Session {
	id := uuid.New().String()
	
	sess := NewSession(id, conn)
	
	m.mu.Lock()
	m.sessions[id] = sess
	m.mu.Unlock()
	
	return sess
}

// RemoveSession removes a session from tracking.
func (m *Manager) RemoveSession(id string) {
	m.mu.Lock()
	if sess, ok := m.sessions[id]; ok {
		sess.Close()
		delete(m.sessions, id)
	}
	m.mu.Unlock()
}

// GetSession retrieves a session by ID.
func (m *Manager) GetSession(id string) (*Session, bool) {
	m.mu.RLock()
	sess, ok := m.sessions[id]
	m.mu.RUnlock()
	return sess, ok
}
