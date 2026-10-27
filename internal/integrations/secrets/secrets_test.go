package secrets

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

func exercise(t *testing.T, s Store) {
	t.Helper()
	if _, err := s.Get("k"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := s.Set("k", "v"); err != nil {
		t.Fatal(err)
	}
	if v, err := s.Get("k"); err != nil || v != "v" {
		t.Fatalf("got %q, %v", v, err)
	}
	if err := s.Delete("k"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("k"); err != nil {
		t.Fatalf("deleting a missing secret should succeed: %v", err)
	}
	if _, err := s.Get("k"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound after delete, got %v", err)
	}
}

func TestMemory(t *testing.T) { exercise(t, NewMemory()) }

// The OS store runs against the keyring library's in-memory mock, never the real keychain.
func TestOSStoreWithMock(t *testing.T) {
	keyring.MockInit()
	exercise(t, OS())
}
