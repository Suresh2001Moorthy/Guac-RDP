package session

import (
    "sync"

    "github.com/google/uuid"
)

type Manager struct {
    sessions map[string]*Session
    mu       sync.RWMutex
}

func NewManager() *Manager {
    return &Manager{
        sessions: make(map[string]*Session),
    }
}

func (m *Manager) GenerateID() string {
    return uuid.New().String()
}

func (m *Manager) Add(s *Session) {
    m.mu.Lock()
    m.sessions[s.ID] = s
    m.mu.Unlock()
}

func (m *Manager) Remove(id string) {
    m.mu.Lock()
    if s, ok := m.sessions[id]; ok {
        s.Close()
        delete(m.sessions, id)
    }
    m.mu.Unlock()
}

func (m *Manager) Get(id string) (*Session, bool) {
    m.mu.RLock()
    s, ok := m.sessions[id]
    m.mu.RUnlock()
    return s, ok
}

func (m *Manager) Count() int {
    m.mu.RLock()
    defer m.mu.RUnlock()
    return len(m.sessions)
}

func (m *Manager) CloseAll() {
    m.mu.Lock()
    for id, s := range m.sessions {
        s.Close()
        delete(m.sessions, id)
    }
    m.mu.Unlock()
}
