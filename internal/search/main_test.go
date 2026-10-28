package search

import (
	"testing"

	"go.uber.org/goleak"
)

// Every test must stop the goroutines it starts and close what it opens (ADR 0008).
func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }
