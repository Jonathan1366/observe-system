// Package auth provides API key generation, storage, validation,
// and Gin middleware for the observe-platform backend.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"sync"
	"time"
)

// ─── Model ────────────────────────────────────────────────────────────────────

// APIKey represents a single issued key with its metadata.
type APIKey struct {
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	// Target system this key monitors
	ArthurURL   string    `json:"arthur_url"`
	JaegerURL   string    `json:"jaeger_url"`
	// Lifecycle
	CreatedAt   time.Time `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	Revoked     bool      `json:"revoked"`
}

// ─── Store ────────────────────────────────────────────────────────────────────

// Store is a thread-safe in-memory key store backed by an optional JSON file.
type Store struct {
	mu       sync.RWMutex
	keys     map[string]*APIKey // key string → *APIKey
	filePath string             // optional persistence path
}

// NewStore creates an empty store. Pass filePath="" to disable persistence.
func NewStore(filePath string) *Store {
	s := &Store{
		keys:     make(map[string]*APIKey),
		filePath: filePath,
	}
	if filePath != "" {
		_ = s.load() // best-effort load; ignore if file doesn't exist yet
	}
	return s
}

// ─── CRUD ─────────────────────────────────────────────────────────────────────

// Create generates a new key, stores it, and returns it.
func (s *Store) Create(name, description, arthurURL, jaegerURL string) (*APIKey, error) {
	raw := make([]byte, 24) // 48 hex chars
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	k := &APIKey{
		Key:         "obs_" + hex.EncodeToString(raw),
		Name:        name,
		Description: description,
		ArthurURL:   arthurURL,
		JaegerURL:   jaegerURL,
		CreatedAt:   time.Now().UTC(),
	}

	s.mu.Lock()
	s.keys[k.Key] = k
	s.mu.Unlock()

	_ = s.save()
	return k, nil
}

// Get returns the key record if it exists and is not revoked.
func (s *Store) Get(key string) (*APIKey, bool) {
	s.mu.RLock()
	k, ok := s.keys[key]
	s.mu.RUnlock()
	if !ok || k.Revoked {
		return nil, false
	}
	return k, true
}

// Touch updates LastUsedAt for a key (called by middleware on valid requests).
func (s *Store) Touch(key string) {
	s.mu.Lock()
	if k, ok := s.keys[key]; ok {
		now := time.Now().UTC()
		k.LastUsedAt = &now
	}
	s.mu.Unlock()
	_ = s.save()
}

// List returns all keys (including revoked ones, so admin can see history).
func (s *Store) List() []*APIKey {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*APIKey, 0, len(s.keys))
	for _, k := range s.keys {
		cp := *k
		out = append(out, &cp)
	}
	return out
}

// Revoke marks a key as revoked. Returns false if key not found.
func (s *Store) Revoke(key string) bool {
	s.mu.Lock()
	k, ok := s.keys[key]
	if ok {
		k.Revoked = true
	}
	s.mu.Unlock()
	if ok {
		_ = s.save()
	}
	return ok
}

// Delete removes a key entirely. Returns false if not found.
func (s *Store) Delete(key string) bool {
	s.mu.Lock()
	_, ok := s.keys[key]
	if ok {
		delete(s.keys, key)
	}
	s.mu.Unlock()
	if ok {
		_ = s.save()
	}
	return ok
}

// Seed adds a pre-defined key (e.g. from environment variable). No-op if key already exists.
func (s *Store) Seed(k *APIKey) {
	s.mu.Lock()
	if _, exists := s.keys[k.Key]; !exists {
		s.keys[k.Key] = k
	}
	s.mu.Unlock()
	_ = s.save()
}

// ─── Persistence ──────────────────────────────────────────────────────────────

func (s *Store) save() error {
	if s.filePath == "" {
		return nil
	}
	s.mu.RLock()
	list := make([]*APIKey, 0, len(s.keys))
	for _, k := range s.keys {
		cp := *k
		list = append(list, &cp)
	}
	s.mu.RUnlock()

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0o600)
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err // file doesn't exist yet, that's OK
	}
	var list []*APIKey
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	s.mu.Lock()
	for _, k := range list {
		s.keys[k.Key] = k
	}
	s.mu.Unlock()
	return nil
}
