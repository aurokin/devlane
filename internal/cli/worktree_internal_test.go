package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/auro/devlane/internal/config"
)

func seedAdapter(seeds []string, generated []string) *config.AdapterConfig {
	out := make([]config.GeneratedOutput, 0, len(generated))
	for _, dest := range generated {
		out = append(out, config.GeneratedOutput{Destination: dest})
	}
	return &config.AdapterConfig{
		Worktree: config.WorktreeConfig{Seed: seeds},
		Outputs:  config.OutputsConfig{Generated: out},
	}
}

func TestPlanSeedsRejectsAbsolutePaths(t *testing.T) {
	repo := t.TempDir()
	_, err := planSeeds(seedAdapter([]string{"/etc/passwd"}, nil), repo)
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("expected absolute-path rejection, got %v", err)
	}
}

func TestPlanSeedsRejectsRepoEscape(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	_, err := planSeeds(seedAdapter([]string{"../escape.env"}, nil), repo)
	if err == nil {
		t.Fatal("expected a repo-escape rejection for ../escape.env")
	}
}

func TestPlanSeedsRejectsRepoRootEntry(t *testing.T) {
	repo := t.TempDir()
	_, err := planSeeds(seedAdapter([]string{"."}, nil), repo)
	if err == nil || !strings.Contains(err.Error(), "repo root") {
		t.Fatalf("expected repo-root rejection, got %v", err)
	}
}

func TestPlanSeedsMarksGeneratedDestinations(t *testing.T) {
	repo := t.TempDir()
	plan, err := planSeeds(seedAdapter([]string{".devlane/generated/app.env"}, []string{".devlane/generated/app.env"}), repo)
	if err != nil {
		t.Fatalf("planSeeds: %v", err)
	}
	if len(plan) != 1 || !plan[0].generated {
		t.Fatalf("expected the seed to be marked generated, got %+v", plan)
	}
}

func TestApplySeedsCopiesFilesDirsAndSymlinksPreservingMode(t *testing.T) {
	repo := t.TempDir()
	mustWrite(t, filepath.Join(repo, "secrets", ".env"), "TOKEN=real", 0o600)
	mustWrite(t, filepath.Join(repo, "cfg", "a.txt"), "A", 0o644)
	if err := os.Symlink("a.txt", filepath.Join(repo, "cfg", "link")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	plan, err := planSeeds(seedAdapter([]string{"secrets/.env", "cfg"}, nil), repo)
	if err != nil {
		t.Fatalf("planSeeds: %v", err)
	}

	target := t.TempDir()
	// Pre-existing destination must be overwritten.
	mustWrite(t, filepath.Join(target, "secrets", ".env"), "TOKEN=stale", 0o644)

	copied, skipped, missing, err := applySeeds(plan, target)
	if err != nil {
		t.Fatalf("applySeeds: %v", err)
	}
	sort.Strings(copied)
	if got := copied; len(got) != 2 || got[0] != "cfg" || got[1] != filepath.Join("secrets", ".env") {
		t.Fatalf("unexpected copied list: %v", got)
	}
	if len(skipped) != 0 || len(missing) != 0 {
		t.Fatalf("unexpected skipped=%v missing=%v", skipped, missing)
	}

	envPath := filepath.Join(target, "secrets", ".env")
	if data, _ := os.ReadFile(envPath); string(data) != "TOKEN=real" {
		t.Fatalf("expected overwritten env content, got %q", string(data))
	}
	if info, err := os.Stat(envPath); err != nil {
		t.Fatalf("stat copied env: %v", err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected mode 0600 preserved, got %v", info.Mode().Perm())
	}

	if data, _ := os.ReadFile(filepath.Join(target, "cfg", "a.txt")); string(data) != "A" {
		t.Fatalf("expected recursive dir copy of a.txt")
	}
	linkInfo, err := os.Lstat(filepath.Join(target, "cfg", "link"))
	if err != nil {
		t.Fatalf("lstat copied link: %v", err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatal("expected copied link to remain a symlink, not a dereferenced file")
	}
	if dest, _ := os.Readlink(filepath.Join(target, "cfg", "link")); dest != "a.txt" {
		t.Fatalf("expected symlink target preserved, got %q", dest)
	}
}

func TestApplySeedsWarnsOnMissingSourceAndSkipsGenerated(t *testing.T) {
	repo := t.TempDir()
	mustWrite(t, filepath.Join(repo, "gen", "out.txt"), "rendered-by-prepare", 0o644)

	plan, err := planSeeds(seedAdapter([]string{"absent.txt", "gen/out.txt"}, []string{"gen/out.txt"}), repo)
	if err != nil {
		t.Fatalf("planSeeds: %v", err)
	}

	target := t.TempDir()
	copied, skipped, missing, err := applySeeds(plan, target)
	if err != nil {
		t.Fatalf("applySeeds: %v", err)
	}
	if len(copied) != 0 {
		t.Fatalf("expected nothing copied, got %v", copied)
	}
	if len(missing) != 1 || missing[0] != "absent.txt" {
		t.Fatalf("expected absent.txt reported missing, got %v", missing)
	}
	if len(skipped) != 1 || skipped[0] != filepath.Join("gen", "out.txt") {
		t.Fatalf("expected gen/out.txt skipped as generated, got %v", skipped)
	}
	if pathExists(filepath.Join(target, "gen", "out.txt")) {
		t.Fatal("a generated-output seed must not be copied")
	}
}

func TestCopyFileReplacesSymlinkInsteadOfFollowing(t *testing.T) {
	dir := t.TempDir()
	// A file standing in for something outside the worktree that a leftover
	// destination symlink points at.
	outside := filepath.Join(dir, "outside.txt")
	mustWrite(t, outside, "original-outside", 0o644)

	src := filepath.Join(dir, "src.txt")
	mustWrite(t, src, "seed-content", 0o600)

	// Destination already exists as a symlink to the outside file.
	dst := filepath.Join(dir, "dst.txt")
	if err := os.Symlink(outside, dst); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	if err := copyFile(src, dst, 0o600); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	if data, _ := os.ReadFile(outside); string(data) != "original-outside" {
		t.Fatalf("copyFile followed the destination symlink and wrote outside the worktree: %q", string(data))
	}
	info, err := os.Lstat(dst)
	if err != nil {
		t.Fatalf("lstat dst: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("expected dst replaced by a regular file, still a symlink")
	}
	if data, _ := os.ReadFile(dst); string(data) != "seed-content" {
		t.Fatalf("expected dst to hold the seed content, got %q", string(data))
	}
}

func mustWrite(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
}
