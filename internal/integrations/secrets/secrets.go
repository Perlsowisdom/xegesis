// Package secrets keeps API keys and tokens in the operating system keychain (Windows Credential
// Manager, macOS Keychain, Secret Service on Linux). Nothing here writes to a database or file.
package secrets

import (
	"errors"
	"sync"

	"github.com/zalando/go-keyring"
)

// Service is the keychain service name all Xegesis secrets are filed under.
const Service = "xegesis"

// ErrNotFound means no secret is stored under that name.
var ErrNotFound = errors.New("secrets: not found")

// Store is a keychain. Names are short identifiers such as "apibible.key".
type Store interface {
	Get(name string) (string, error)
	Set(name, value string) error
	Delete(name string) error
}

type osStore struct{}

// OS returns the operating system keychain.
func OS() Store { return osStore{} }

func (osStore) Get(name string) (string, error) {
	v, err := keyring.Get(Service, name)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return v, err
}

func (osStore) Set(name, value string) error { return keyring.Set(Service, name, value) }

func (osStore) Delete(name string) error {
	err := keyring.Delete(Service, name)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// Memory is an in-memory Store for tests.
type Memory struct {
	mu sync.Mutex
	m  map[string]string
}

func NewMemory() *Memory { return &Memory{m: map[string]string{}} }

func (s *Memory) Get(name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[name]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (s *Memory) Set(name, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[name] = value
	return nil
}

func (s *Memory) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, name)
	return nil
}
