//go:build !wasm

package goflare

import (
	"errors"
	"sync"
)

// Store abstracts access for testability.
type Store interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

var ErrNotFound = errors.New("not found")

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
		return "", ErrNotFound
	}
	return val, nil
}

func (s *MemoryStore) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if value == "" {
		delete(s.data, key)
		return nil
	}
	s.data[key] = value
	return nil
}
