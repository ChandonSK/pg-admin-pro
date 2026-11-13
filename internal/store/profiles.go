package store

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"pg-admin-pro/internal/cryptoutil"
	"pg-admin-pro/internal/engine"
)

// ServerProfile describes a saved database server profile without exposing secrets.
type ServerProfile struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Env         string            `json:"env"` // local, staging, production, other
	Engine      engine.EngineKind `json:"engine"`
	Host        string            `json:"host"`
	Port        int               `json:"port"`
	Database    string            `json:"database"`
	Username    string            `json:"username"`
	SSLMode     string            `json:"sslMode"`
	HasPassword bool              `json:"hasPassword"`
	CreatedAt   string            `json:"createdAt"`
	LastUsedAt  string            `json:"lastUsedAt,omitempty"`
}

type storedProfile struct {
	ServerProfile
	EncryptedPassword string `json:"encryptedPassword,omitempty"`
}

// ProfileUpsertReq defines inputs for creating or updating a saved profile.
type ProfileUpsertReq struct {
	ID           string            `json:"id,omitempty"`
	Name         string            `json:"name"`
	Env          string            `json:"env"`
	Engine       engine.EngineKind `json:"engine"`
	Host         string            `json:"host"`
	Port         int               `json:"port"`
	Database     string            `json:"database"`
	Username     string            `json:"username"`
	Password     string            `json:"password"`
	SavePassword bool              `json:"savePassword"`
	KeepPassword bool              `json:"keepPassword"`
	SSLMode      string            `json:"sslMode"`
}

// ProfileStore manages encrypted server connection profiles on disk.
type ProfileStore struct {
	path string
	key  [32]byte
	mu   sync.RWMutex
	list []storedProfile
}

// NewProfileStore opens or creates profiles.json encrypted with key.
func NewProfileStore(dataDir string, key [32]byte) (*ProfileStore, error) {
	ps := &ProfileStore{
		path: filepath.Join(dataDir, "profiles.json"),
		key:  key,
	}
	if err := ReadJSONFile(ps.path, &ps.list); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return ps, nil
}

// List returns all saved profiles sorted by last used / name without secrets.
func (ps *ProfileStore) List() []ServerProfile {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	out := make([]ServerProfile, 0, len(ps.list))
	for _, p := range ps.list {
		pub := p.ServerProfile
		pub.HasPassword = p.EncryptedPassword != ""
		out = append(out, pub)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LastUsedAt != out[j].LastUsedAt {
			return out[i].LastUsedAt > out[j].LastUsedAt
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// GetConnection resolves a profile ID into an engine.ConnectionConfig with decrypted password.
func (ps *ProfileStore) GetConnection(id string) (ServerProfile, engine.ConnectionConfig, error) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	for _, p := range ps.list {
		if p.ID != id {
			continue
		}
		var pass string
		if p.EncryptedPassword != "" {
			dec, err := cryptoutil.Decrypt(ps.key, p.EncryptedPassword)
			if err != nil {
				return ServerProfile{}, engine.ConnectionConfig{}, err
			}
			pass = dec
		}
		pub := p.ServerProfile
		pub.HasPassword = p.EncryptedPassword != ""
		return pub, engine.ConnectionConfig{
			Engine:   p.Engine,
			Host:     p.Host,
			Port:     p.Port,
			Database: p.Database,
			Username: p.Username,
			Password: pass,
			SSLMode:  p.SSLMode,
		}, nil
	}
	return ServerProfile{}, engine.ConnectionConfig{}, os.ErrNotExist
}

// Upsert saves a new or existing profile with optional AES-256-GCM password encryption.
func (ps *ProfileStore) Upsert(req ProfileUpsertReq) (ServerProfile, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return ServerProfile{}, errors.New("profile name is required")
	}
	env := strings.ToLower(strings.TrimSpace(req.Env))
	switch env {
	case "local", "staging", "production", "other":
	default:
		env = "other"
	}
	if req.Engine == "" {
		req.Engine = engine.EnginePostgres
	}

	ps.mu.Lock()
	defer ps.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	idx := -1
	if req.ID != "" {
		for i, existing := range ps.list {
			if existing.ID == req.ID {
				idx = i
				break
			}
		}
	}

	var encPass string
	if req.SavePassword && req.Password != "" {
		enc, err := cryptoutil.Encrypt(ps.key, req.Password)
		if err != nil {
			return ServerProfile{}, err
		}
		encPass = enc
	} else if req.KeepPassword && idx >= 0 {
		encPass = ps.list[idx].EncryptedPassword
	}

	item := storedProfile{
		ServerProfile: ServerProfile{
			ID:          req.ID,
			Name:        name,
			Env:         env,
			Engine:      req.Engine,
			Host:        strings.TrimSpace(req.Host),
			Port:        req.Port,
			Database:    strings.TrimSpace(req.Database),
			Username:    strings.TrimSpace(req.Username),
			SSLMode:     strings.TrimSpace(req.SSLMode),
			HasPassword: encPass != "",
			CreatedAt:   now,
			LastUsedAt:  now,
		},
		EncryptedPassword: encPass,
	}

	if idx >= 0 {
		item.CreatedAt = ps.list[idx].CreatedAt
		ps.list[idx] = item
	} else {
		if item.ID == "" {
			item.ID = newID()
		}
		ps.list = append(ps.list, item)
	}

	if err := WriteJSONAtomic(ps.path, ps.list, 0o600); err != nil {
		return ServerProfile{}, err
	}
	return item.ServerProfile, nil
}

// MarkUsed updates the lastUsedAt timestamp on a profile.
func (ps *ProfileStore) MarkUsed(id string) {
	if id == "" {
		return
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	for i := range ps.list {
		if ps.list[i].ID == id {
			ps.list[i].LastUsedAt = time.Now().UTC().Format(time.RFC3339)
			_ = WriteJSONAtomic(ps.path, ps.list, 0o600)
			return
		}
	}
}

// Delete removes a saved profile by ID.
func (ps *ProfileStore) Delete(id string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	for i := range ps.list {
		if ps.list[i].ID == id {
			ps.list = append(ps.list[:i], ps.list[i+1:]...)
			return WriteJSONAtomic(ps.path, ps.list, 0o600)
		}
	}
	return os.ErrNotExist
}
