//go:build !unix

package portalloc

import (
	"strings"
	"testing"
)

func TestAcquireCatalogLockStubMessage(t *testing.T) {
	lock, err := acquireCatalogLock(0)
	if err == nil {
		_ = lock.Close()
		t.Fatal("expected acquireCatalogLock to return an error on non-unix builds, got nil")
	}

	msg := err.Error()
	wants := []string{
		"Windows",
		"Windows support for catalog concurrency",
		"plans/phase-roadmap.md",
	}
	for _, want := range wants {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q\nfull message: %s", want, msg)
		}
	}
}
