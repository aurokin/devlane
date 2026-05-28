package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsTerminalRejectsNonTerminalCharDevicesAndNil(t *testing.T) {
	// /dev/null is a character device, so an os.ModeCharDevice-based check would
	// wrongly call it interactive. term.IsTerminal must report it non-interactive
	// so callers reading from it take their non-interactive path.
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer devnull.Close()

	if IsTerminal(devnull) {
		t.Fatalf("%s must not be classified as an interactive terminal", os.DevNull)
	}
	if IsTerminal(nil) {
		t.Fatalf("nil file must not be classified as an interactive terminal")
	}

	// A regular file is also not a terminal.
	regular, err := os.CreateTemp(t.TempDir(), "regular")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer regular.Close()
	if IsTerminal(regular) {
		t.Fatalf("a regular file must not be classified as an interactive terminal")
	}
}

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

func TestIsWithinResolvedAllowsEquivalentSymlinkPrefix(t *testing.T) {
	root := t.TempDir()
	realRoot := filepath.Join(root, "real")
	linkRoot := filepath.Join(root, "link")
	if err := os.MkdirAll(filepath.Join(realRoot, ".devlane"), 0o755); err != nil {
		t.Fatalf("mkdir real root: %v", err)
	}
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	target := filepath.Join(linkRoot, ".devlane", "manifest.json")
	if !IsWithinResolved(realRoot, target) {
		t.Fatalf("expected %s to resolve within %s", target, realRoot)
	}
	if !IsWithinResolved(linkRoot, filepath.Join(realRoot, ".devlane", "manifest.json")) {
		t.Fatalf("expected real path to resolve within symlinked root")
	}
}
