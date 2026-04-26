package portalloc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/auro/devlane/internal/config"
	"github.com/auro/devlane/internal/testutil"
)

func TestMutateRequiresCallback(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	if err := Mutate(nil); err == nil {
		t.Fatal("expected error for nil callback")
	}
}

func TestMutatePersistsCallbackChanges(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	seedCatalogForMutate(t,
		Allocation{App: "alpha", Lane: "main", Mode: "stable", Branch: "main", Service: "web", Port: 3000, RepoPath: "/repo/alpha", LastPrepared: "2026-04-25T00:00:00Z"},
		Allocation{App: "beta", Lane: "feature-x", Mode: "dev", Branch: "feature-x", Service: "api", Port: 4001, RepoPath: "/repo/beta", LastPrepared: "2026-04-25T00:00:00Z"},
	)

	err := Mutate(func(snap *Snapshot) error {
		filtered := snap.Allocations[:0]
		for _, row := range snap.Allocations {
			if row.App == "beta" {
				continue
			}
			filtered = append(filtered, row)
		}
		snap.Allocations = filtered
		return nil
	})
	if err != nil {
		t.Fatalf("Mutate: %v", err)
	}

	rows, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row after Mutate, got %d (%#v)", len(rows), rows)
	}
	if rows[0].App != "alpha" {
		t.Fatalf("expected only alpha row to survive, got %#v", rows[0])
	}
}

func TestMutateCallbackErrorDoesNotPublish(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	original := Allocation{App: "alpha", Lane: "main", Mode: "stable", Branch: "main", Service: "web", Port: 3000, RepoPath: "/repo/alpha", LastPrepared: "2026-04-25T00:00:00Z"}
	seedCatalogForMutate(t, original)

	wantErr := errors.New("forced callback failure")
	err := Mutate(func(snap *Snapshot) error {
		snap.Allocations[0].Port = 9999
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected callback error to propagate, got %v", err)
	}

	rows, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 || rows[0] != original {
		t.Fatalf("expected unchanged catalog after callback error, got %#v", rows)
	}
}

func TestMutateCallbackPanicDoesNotPublish(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	original := Allocation{App: "alpha", Lane: "main", Mode: "stable", Branch: "main", Service: "web", Port: 3000, RepoPath: "/repo/alpha", LastPrepared: "2026-04-25T00:00:00Z"}
	seedCatalogForMutate(t, original)

	err := Mutate(func(snap *Snapshot) error {
		snap.Allocations[0].Port = 9999
		panic("explode")
	})
	if err == nil {
		t.Fatal("expected Mutate to surface a recovered panic as an error")
	}
	if !strings.Contains(err.Error(), "explode") || !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("expected panic-aware error, got %v", err)
	}

	rows, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 || rows[0] != original {
		t.Fatalf("expected unchanged catalog after callback panic, got %#v", rows)
	}

	// Lock must be released after recover — a follow-up Mutate should not block.
	done := make(chan error, 1)
	go func() {
		done <- Mutate(func(_ *Snapshot) error { return nil })
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("post-panic Mutate: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("post-panic Mutate blocked — lock was not released after recover")
	}
}

func TestMutateSerializesWithBeginPrepare(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	adapter, lane := buildLaneForMutate(t, repo)

	session, err := BeginPrepare(adapter, lane)
	if err != nil {
		t.Fatalf("begin prepare: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	mutateStarted := make(chan struct{})
	mutateDone := make(chan error, 1)
	go func() {
		close(mutateStarted)
		mutateDone <- Mutate(func(_ *Snapshot) error { return nil })
	}()

	<-mutateStarted
	select {
	case err := <-mutateDone:
		t.Fatalf("expected Mutate to wait for catalog lock, returned err=%v", err)
	case <-time.After(200 * time.Millisecond):
	}

	if err := session.Commit(); err != nil {
		t.Fatalf("commit prepare: %v", err)
	}

	select {
	case err := <-mutateDone:
		if err != nil {
			t.Fatalf("Mutate after Prepare release: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Mutate did not acquire the lock after Prepare released it")
	}
}

func TestMutateContentionTimeoutSurfacesLockHolderPID(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	dir := filepath.Join(configHome, "devlane")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	lockPath := filepath.Join(dir, "catalog.json.lock")

	const fakeHolderPID = 424242
	holder, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open lock holder: %v", err)
	}
	t.Cleanup(func() {
		_ = syscall.Flock(int(holder.Fd()), syscall.LOCK_UN)
		_ = holder.Close()
	})
	if err := syscall.Flock(int(holder.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("flock holder: %v", err)
	}
	if _, err := fmt.Fprintf(holder, "%d\n", fakeHolderPID); err != nil {
		t.Fatalf("write holder pid: %v", err)
	}

	restore := overrideCatalogLockTimeout(50 * time.Millisecond)
	t.Cleanup(restore)

	err = Mutate(func(_ *Snapshot) error {
		t.Fatal("callback must not run when the lock cannot be acquired")
		return nil
	})
	if err == nil {
		t.Fatal("expected timeout error while another fd holds the lock")
	}
	wants := []string{"acquire catalog lock", "timed out", fmt.Sprintf("pid %d", fakeHolderPID)}
	for _, want := range wants {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}

var overrideMu sync.Mutex

func overrideCatalogLockTimeout(d time.Duration) func() {
	overrideMu.Lock()
	previous := catalogLockTimeout
	catalogLockTimeout = d
	overrideMu.Unlock()
	return func() {
		overrideMu.Lock()
		catalogLockTimeout = previous
		overrideMu.Unlock()
	}
}

func buildLaneForMutate(t *testing.T, repo string) (*config.AdapterConfig, Lane) {
	t.Helper()
	adapter, err := config.LoadAdapter(filepath.Join(repo, "devlane.yaml"))
	if err != nil {
		t.Fatalf("load adapter: %v", err)
	}
	return adapter, Lane{
		App:      adapter.App,
		RepoPath: repo,
		Name:     "feature/test-lane",
		Mode:     "dev",
		Branch:   "feature/test-lane",
		Stable:   false,
	}
}

func seedCatalogForMutate(t *testing.T, rows ...Allocation) {
	t.Helper()
	if err := Mutate(func(snap *Snapshot) error {
		snap.Allocations = append(snap.Allocations[:0], rows...)
		return nil
	}); err != nil {
		t.Fatalf("seed catalog: %v", err)
	}
}
