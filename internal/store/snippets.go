package store

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// SQLSnippet represents a saved SQL query snippet reusable across sessions.
type SQLSnippet struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Tag       string `json:"tag"`
	SQL       string `json:"sql"`
	Engine    string `json:"engine"`
	UpdatedAt string `json:"updatedAt"`
}

// SnippetStore manages saved SQL snippets on disk.
type SnippetStore struct {
	path string
	mu   sync.RWMutex
	list []SQLSnippet
}

// NewSnippetStore loads or initializes snippets.json inside dataDir.
func NewSnippetStore(dataDir string) (*SnippetStore, error) {
	ss := &SnippetStore{
		path: filepath.Join(dataDir, "snippets.json"),
	}
	if err := ReadJSONFile(ss.path, &ss.list); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return ss, nil
}

// List returns saved snippets sorted newest updated first.
func (ss *SnippetStore) List() []SQLSnippet {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	out := make([]SQLSnippet, len(ss.list))
	copy(out, ss.list)
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	return out
}

// Save creates or updates a snippet.
func (ss *SnippetStore) Save(req SQLSnippet) (SQLSnippet, error) {
	title := strings.TrimSpace(req.Title)
	sqlText := strings.TrimSpace(req.SQL)
	if title == "" || sqlText == "" {
		return SQLSnippet{}, errors.New("title and sql are required")
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	tag := strings.TrimSpace(req.Tag)
	if tag == "" {
		tag = "general"
	}

	if req.ID != "" {
		for i := range ss.list {
			if ss.list[i].ID == req.ID {
				ss.list[i].Title = title
				ss.list[i].Tag = tag
				ss.list[i].SQL = sqlText
				ss.list[i].Engine = req.Engine
				ss.list[i].UpdatedAt = now
				if err := WriteJSONAtomic(ss.path, ss.list, 0o640); err != nil {
					return SQLSnippet{}, err
				}
				return ss.list[i], nil
			}
		}
	}

	item := SQLSnippet{
		ID:        newID(),
		Title:     title,
		Tag:       tag,
		SQL:       sqlText,
		Engine:    req.Engine,
		UpdatedAt: now,
	}
	ss.list = append(ss.list, item)
	if err := WriteJSONAtomic(ss.path, ss.list, 0o640); err != nil {
		return SQLSnippet{}, err
	}
	return item, nil
}

// Delete removes a snippet by ID.
func (ss *SnippetStore) Delete(id string) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	for i := range ss.list {
		if ss.list[i].ID == id {
			ss.list = append(ss.list[:i], ss.list[i+1:]...)
			return WriteJSONAtomic(ss.path, ss.list, 0o640)
		}
	}
	return os.ErrNotExist
}
