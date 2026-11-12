//go:build store

package update

import (
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/command"
)

// The Microsoft Store build (go test -tags store) has no updater and no update commands.
func TestStoreBuildHasNoUpdater(t *testing.T) {
	if Available {
		t.Fatal("Available is true in the Store build")
	}
	r := command.NewRegistry()
	Register(r)
	for _, d := range r.List() {
		t.Errorf("the Store build registered %s", d.ID)
	}
}
