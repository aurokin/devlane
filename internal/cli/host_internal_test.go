package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRepoAdaptersGenuinelyMissingIsRemovable(t *testing.T) {
	// The worktree root is gone but its parent (the tempdir) is present — the
	// signature of a worktree deleted in place — so it classifies as the
	// removable missing-repoPath, which loadRepoAdapters signals with os.ErrNotExist.
	repoPath := filepath.Join(t.TempDir(), "web")

	_, err := loadRepoAdapters(repoPath)
	if err == nil {
		t.Fatalf("expected an error for an absent worktree root")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a genuinely-missing worktree (parent present) must classify as missing-repoPath (os.ErrNotExist), got: %v", err)
	}
}

func TestLoadRepoAdaptersUnreachableParentIsNotRemovable(t *testing.T) {
	// Both the worktree root AND its parent are absent — the signature of an
	// unmounted/unreachable volume rather than a deletion. The absence must NOT
	// classify as the removable missing-repoPath (os.ErrNotExist); returning a
	// non-ErrNotExist error routes it to the surfaced, non-removable bad-adapter
	// bucket so host gc can never delete a row whose volume was merely offline.
	repoPath := filepath.Join(t.TempDir(), "missing-parent", "web")

	_, err := loadRepoAdapters(repoPath)
	if err == nil {
		t.Fatalf("expected an error for an absent worktree root")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an unreachable worktree (parent also missing) must NOT classify as removable missing-repoPath, got os.ErrNotExist: %v", err)
	}
}
