package update

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// The Store build must not merely switch the updater off: the update code is not compiled in.
func TestStoreBuildExcludesUpdaterCode(t *testing.T) {
	out, err := exec.Command("go", "list", "-tags", "store", "-f", `{{join .GoFiles "\n"}}`, ".").Output()
	if err != nil {
		t.Skipf("go list: %v", err)
	}
	files := strings.Fields(string(out))
	if !slices.Equal(files, []string{"store.go"}) {
		t.Fatalf("Store build compiles %v, want only store.go", files)
	}
	out, err = exec.Command("go", "list", "-tags", "store", "-deps", "-f", "{{.ImportPath}}", "../../app").Output()
	if err != nil {
		t.Skipf("go list: %v", err)
	}
	for _, p := range strings.Fields(string(out)) {
		if strings.HasSuffix(p, "internal/update/updatesig") {
			t.Errorf("the Store app links %s", p)
		}
	}
}
