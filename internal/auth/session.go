package auth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"pg-admin-pro/internal/engine"
)

const SessionCookieName = "pg_admin_pro_session"

type sessionEntry struct {
	driver    engine.Driver
	createdAt time.Time
	lastSeen  time.Time
}

// Manager stores active authenticated database sessions in memory with automatic TTL eviction.
type Manager struct {
	ttl      time.Duration
	mu       sync.RWMutex
	sessions map[string]*sessionEntry
	stopCh   chan struct{}
}

// NewManager creates a session manager and starts background cleanup.
func NewManager(ttl time.Duration) *Manager {
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	m := &Manager{
		ttl:      ttl,
		sessions: make(map[string]*sessionEntry),
		stopCh:   make(chan struct{}),
	}
	go m.reapLoop()
	return m
}

// Create generates a cryptographically random token and registers the driver.
func (m *Manager) Create(drv engine.Driver) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := hex.EncodeToString(buf)
	now := time.Now()

	m.mu.Lock()
	m.sessions[token] = &sessionEntry{
		driver:    drv,
		createdAt: now,
		lastSeen:  now,
	}
	m.mu.Unlock()
	return token, nil
}

// Get retrieves the active driver for a session token and refreshes lastSeen.
func (m *Manager) Get(token string) (engine.Driver, bool) {
	if token == "" {
		return nil, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.sessions[token]
	if !ok {
		return nil, false
	}
	if time.Since(entry.lastSeen) > m.ttl {
		_ = entry.driver.Close()
		delete(m.sessions, token)
		return nil, false
	}
	entry.lastSeen = time.Now()
	return entry.driver, true
}

// Replace swaps the active database driver for an existing session and closes the old driver.
func (m *Manager) Replace(token string, newDrv engine.Driver) bool {
	if token == "" || newDrv == nil {
		return false
	}
	m.mu.Lock()
	entry, ok := m.sessions[token]
	if !ok {
		m.mu.Unlock()
		return false
	}
	oldDrv := entry.driver
	entry.driver = newDrv
	entry.lastSeen = time.Now()
	m.mu.Unlock()

	if oldDrv != nil {
		_ = oldDrv.Close()
	}
	return true
}

// Delete closes the underlying database driver and invalidates the session.
func (m *Manager) Delete(token string) {
	if token == "" {
		return
	}
	m.mu.Lock()
	entry, ok := m.sessions[token]
	if ok {
		delete(m.sessions, token)
	}
	m.mu.Unlock()
	if ok && entry.driver != nil {
		_ = entry.driver.Close()
	}
}

// Close shuts down the reaper and closes all active driver pools.
func (m *Manager) Close() {
	close(m.stopCh)
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, e := range m.sessions {
		if e.driver != nil {
			_ = e.driver.Close()
		}
		delete(m.sessions, k)
	}
}

func (m *Manager) reapLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.mu.Lock()
			for k, e := range m.sessions {
				if time.Since(e.lastSeen) > m.ttl {
					if e.driver != nil {
						_ = e.driver.Close()
					}
					delete(m.sessions, k)
				}
			}
			m.mu.Unlock()
		}
	}
}

// LoginLimiter provides lightweight per-IP login throttling.
type LoginLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

// NewLoginLimiter creates a per-IP login rate limiter.
func NewLoginLimiter() *LoginLimiter {
	return &LoginLimiter{attempts: make(map[string][]time.Time)}
}

// Allow checks whether an IP may attempt login (max 15 attempts per minute).
func (l *LoginLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := time.Now().Add(-1 * time.Minute)
	prev := l.attempts[ip]
	kept := prev[:0]
	for _, ts := range prev {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= 15 {
		l.attempts[ip] = kept
		return false
	}
	l.attempts[ip] = append(kept, time.Now())
	return true
}
