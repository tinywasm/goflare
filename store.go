package goflare

import (
	"sync"

	"github.com/zalando/go-keyring"
)

// Store abstracts keyring access for testability.
type Store interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// KeyringStore is the real implementation using go-keyring.
type KeyringStore struct {
	ProjectName string
}

func (s *KeyringStore) Get(key string) (string, error) {
	return keyring.Get("goflare/"+s.ProjectName, key)
}

func (s *KeyringStore) Set(key, value string) error {
	return keyring.Set("goflare/"+s.ProjectName, key, value)
}

// MemoryStore is an in-memory Store exported for use by library consumers in tests.
// Safe for concurrent use.
type MemoryStore struct {
	mu   sync.Mutex
	data map[string]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: make(map[string]string),
	}
}

func (s *MemoryStore) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	val, ok := s.data[key]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return val, nil
}

func (s *MemoryStore) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	return nil
}
