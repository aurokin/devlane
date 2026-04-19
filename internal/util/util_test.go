package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAdapterPathRejectsSymlinkEscapeForExistingPath(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "app.env.tmpl"), []byte("APP={{app}}\n"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, "linked")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	_, err := ResolveAdapterPath(repo, repo, "linked/app.env.tmpl")
	if err == nil {
		t.Fatal("expected symlink escape error")
	}
}

func TestResolveAdapterPathRejectsSymlinkEscapeForNonexistentPath(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, "linked")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	_, err := ResolveAdapterPath(repo, repo, "linked/generated/app.env")
	if err == nil {
		t.Fatal("expected symlink escape error")
	}
}

func TestResolveAdapterPathAllowsInternalSymlinkTargets(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	internal := filepath.Join(repo, "templates")
	if err := os.MkdirAll(internal, 0o755); err != nil {
		t.Fatalf("mkdir internal target: %v", err)
	}
	if err := os.Symlink(internal, filepath.Join(repo, "linked")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	resolved, err := ResolveAdapterPath(repo, repo, "linked/app.env.tmpl")
	if err != nil {
		t.Fatalf("ResolveAdapterPath returned error: %v", err)
	}
	if resolved != filepath.Join(repo, "linked", "app.env.tmpl") {
		t.Fatalf("unexpected resolved path: %s", resolved)
	}
}
