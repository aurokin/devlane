package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/auro/devlane/internal/testutil"
)

func siblingPath(t *testing.T, repo, slug string) string {
	t.Helper()
	return filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"-"+slug)
}

func catalogPathFor(repo string) string {
	return filepath.Join(repo, ".xdg", "devlane", "catalog.json")
}

func readCatalogRepoPaths(t *testing.T, repo string) []string {
	t.Helper()
	payload, err := os.ReadFile(catalogPathFor(repo))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read catalog: %v", err)
	}
	var stored struct {
		Allocations []struct {
			RepoPath string `json:"repoPath"`
		} `json:"allocations"`
	}
	if err := json.Unmarshal(payload, &stored); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	paths := make([]string, 0, len(stored.Allocations))
	for _, row := range stored.Allocations {
		paths = append(paths, row.RepoPath)
	}
	return paths
}

func TestWorktreeCreateAddsCheckoutAndRegistersCatalog(t *testing.T) {
	repo := testutil.InitDemoRepo(t)

	code, stdout, stderr := runCLI(t, []string{"worktree", "create", "feature/new-lane", "--cwd", repo})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "created worktree for lane") {
		t.Fatalf("expected creation confirmation, got stdout:\n%s", stdout)
	}

	sibling := siblingPath(t, repo, "feature-new-lane")
	if _, err := os.Stat(filepath.Join(sibling, "devlane.yaml")); err != nil {
		t.Fatalf("expected adapter in the new worktree: %v", err)
	}

	assertCatalogAllocations(t, catalogPathFor(repo), 1)
	got := readCatalogRepoPaths(t, repo)
	if len(got) != 1 || resolvePath(t, got[0]) != resolvePath(t, sibling) {
		t.Fatalf("expected catalog row for new worktree %s, got %v", sibling, got)
	}
}

func TestWorktreeCreateRejectsStableLane(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	code, _, stderr := runCLI(t, []string{"worktree", "create", "stable", "--cwd", repo})
	if code != 1 {
		t.Fatalf("expected exit 1 for stable lane, got %d", code)
	}
	if !strings.Contains(stderr, "stable lane") {
		t.Fatalf("expected stable-lane rejection, got stderr:\n%s", stderr)
	}
}

func TestWorktreeCreateRejectsStableBranchLane(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	// "master" is declared in lane.stable_branches but is not yet a branch;
	// without the guard, create would branch it and prepare the new worktree
	// as the stable lane.
	code, _, stderr := runCLI(t, []string{"worktree", "create", "master", "--cwd", repo})
	if code != 1 {
		t.Fatalf("expected exit 1 for a stable-branch lane, got %d", code)
	}
	if !strings.Contains(stderr, "stable_branches") {
		t.Fatalf("expected stable-branch rejection, got stderr:\n%s", stderr)
	}
	if _, err := os.Stat(siblingPath(t, repo, "master")); !os.IsNotExist(err) {
		t.Fatal("a stable-branch lane must be rejected before the checkout is created")
	}
}

func TestWorktreeCreateRejectsExistingBranch(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	// feature/test-lane already exists (the current branch) and is not a stable
	// branch, so it exercises the existing-branch guard rather than the
	// stable-branch one.
	code, _, stderr := runCLI(t, []string{"worktree", "create", "feature/test-lane", "--cwd", repo})
	if code != 1 {
		t.Fatalf("expected exit 1 for existing branch, got %d", code)
	}
	if !strings.Contains(stderr, "already exists") {
		t.Fatalf("expected existing-branch rejection, got stderr:\n%s", stderr)
	}
}

func TestWorktreeCreateRejectsExistingPath(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	sibling := siblingPath(t, repo, "dup")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatalf("mkdir sibling: %v", err)
	}
	code, _, stderr := runCLI(t, []string{"worktree", "create", "dup", "--cwd", repo})
	if code != 1 {
		t.Fatalf("expected exit 1 for existing path, got %d", code)
	}
	if !strings.Contains(stderr, "already exists") {
		t.Fatalf("expected existing-path rejection, got stderr:\n%s", stderr)
	}
}

func TestWorktreeCreateRejectsSubtreeAdapter(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	payload, err := os.ReadFile(filepath.Join(repo, "devlane.yaml"))
	if err != nil {
		t.Fatalf("read adapter: %v", err)
	}
	subAdapter := filepath.Join(repo, "svc", "devlane.yaml")
	if err := os.MkdirAll(filepath.Dir(subAdapter), 0o755); err != nil {
		t.Fatalf("mkdir svc: %v", err)
	}
	if err := os.WriteFile(subAdapter, payload, 0o644); err != nil {
		t.Fatalf("write subtree adapter: %v", err)
	}

	code, _, stderr := runCLI(t, []string{"worktree", "create", "x", "--cwd", repo, "--config", subAdapter})
	if code != 1 {
		t.Fatalf("expected exit 1 for subtree adapter, got %d", code)
	}
	if !strings.Contains(stderr, "subtree adapter") {
		t.Fatalf("expected subtree-adapter rejection, got stderr:\n%s", stderr)
	}
}

func TestWorktreeCreateCopiesSeedsAndSkipsGenerated(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	appendWorktreeSeeds(t, repo, []string{"secrets/.env", ".devlane/generated/app.env", "absent.env"})
	mustWriteRepoFile(t, filepath.Join(repo, "secrets", ".env"), "TOKEN=real")
	mustWriteRepoFile(t, filepath.Join(repo, ".devlane", "generated", "app.env"), "stale-source")

	code, stdout, stderr := runCLI(t, []string{"worktree", "create", "feat", "--cwd", repo})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	sibling := siblingPath(t, repo, "feat")
	if data, _ := os.ReadFile(filepath.Join(sibling, "secrets", ".env")); string(data) != "TOKEN=real" {
		t.Fatalf("expected seeded secret in the new worktree, got %q", string(data))
	}
	for _, want := range []string{"seeded secrets/.env", "skipped seed", "warning: seed source absent.env"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in stdout:\n%s", want, stdout)
		}
	}
}

func TestWorktreeCreateRejectsAbsoluteSeedBeforeCreatingCheckout(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	appendWorktreeSeeds(t, repo, []string{"/etc/passwd"})

	code, _, stderr := runCLI(t, []string{"worktree", "create", "feat", "--cwd", repo})
	if code != 1 {
		t.Fatalf("expected exit 1 for absolute seed, got %d", code)
	}
	if !strings.Contains(stderr, "absolute") {
		t.Fatalf("expected absolute-seed rejection, got stderr:\n%s", stderr)
	}
	if _, err := os.Stat(siblingPath(t, repo, "feat")); !os.IsNotExist(err) {
		t.Fatal("a statically invalid seed must fail before the checkout is created")
	}
}

func TestWorktreeRemoveForceCleansCheckoutAndCatalog(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	if code, _, stderr := runCLI(t, []string{"worktree", "create", "feat", "--cwd", repo}); code != 0 {
		t.Fatalf("create failed: %d\n%s", code, stderr)
	}
	sibling := siblingPath(t, repo, "feat")

	code, stdout, stderr := runCLI(t, []string{"worktree", "remove", "feat", "--cwd", repo, "--force"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "removed worktree") || !strings.Contains(stdout, "1 catalog allocation") {
		t.Fatalf("expected removal confirmation, got stdout:\n%s", stdout)
	}
	if _, err := os.Stat(sibling); !os.IsNotExist(err) {
		t.Fatalf("expected worktree dir removed, stat err: %v", err)
	}
	assertCatalogAllocations(t, catalogPathFor(repo), 0)
}

func TestWorktreeRemoveWithoutForceFailsOnDirtyAndPreservesCatalog(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	if code, _, stderr := runCLI(t, []string{"worktree", "create", "feat", "--cwd", repo}); code != 0 {
		t.Fatalf("create failed: %d\n%s", code, stderr)
	}
	sibling := siblingPath(t, repo, "feat")

	// prepare wrote untracked generated outputs, so the worktree is dirty and a
	// non-force remove must refuse without running scoped cleanup.
	code, _, stderr := runCLI(t, []string{"worktree", "remove", "feat", "--cwd", repo})
	if code != 1 {
		t.Fatalf("expected exit 1 for dirty worktree, got %d", code)
	}
	if !strings.Contains(stderr, "--force") {
		t.Fatalf("expected a --force hint, got stderr:\n%s", stderr)
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Fatalf("worktree must remain when removal fails: %v", err)
	}
	assertCatalogAllocations(t, catalogPathFor(repo), 1)
}

func TestWorktreeRemoveMissingDefaultPathFails(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	code, _, stderr := runCLI(t, []string{"worktree", "remove", "ghost", "--cwd", repo})
	if code != 1 {
		t.Fatalf("expected exit 1 for missing worktree, got %d", code)
	}
	if !strings.Contains(stderr, "no worktree found") {
		t.Fatalf("expected missing-worktree error, got stderr:\n%s", stderr)
	}
}

func TestWorktreeRemoveScopedCleanupSpareOtherCheckouts(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	// Register the main checkout in the catalog as well.
	if code, _, stderr := runCLI(t, []string{"prepare", "--cwd", repo}); code != 0 {
		t.Fatalf("prepare main failed: %d\n%s", code, stderr)
	}
	if code, _, stderr := runCLI(t, []string{"worktree", "create", "feat", "--cwd", repo}); code != 0 {
		t.Fatalf("create failed: %d\n%s", code, stderr)
	}
	assertCatalogAllocations(t, catalogPathFor(repo), 2)

	if code, _, stderr := runCLI(t, []string{"worktree", "remove", "feat", "--cwd", repo, "--force"}); code != 0 {
		t.Fatalf("remove failed: %d\n%s", code, stderr)
	}

	// Only the worktree's row is gone; the main checkout's row survives.
	assertCatalogAllocations(t, catalogPathFor(repo), 1)
	got := readCatalogRepoPaths(t, repo)
	if len(got) != 1 || resolvePath(t, got[0]) != resolvePath(t, repo) {
		t.Fatalf("expected only the main checkout row to remain, got %v", got)
	}
}

func TestWorktreeRemoveCanonicalizesSymlinkedPathForCleanup(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	if code, _, stderr := runCLI(t, []string{"worktree", "create", "feat", "--cwd", repo}); code != 0 {
		t.Fatalf("create failed: %d\n%s", code, stderr)
	}
	sibling := siblingPath(t, repo, "feat")

	// Address the worktree through a symlink so the captured path differs from
	// the canonical repoPath prepare stored. Cleanup must resolve the match
	// before git deletes the directory; it cannot resolve a deleted path after.
	link := filepath.Join(t.TempDir(), "wt-link")
	if err := os.Symlink(sibling, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	code, stdout, stderr := runCLI(t, []string{"worktree", "remove", "feat", "--cwd", repo, "--path", link, "--force"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "1 catalog allocation") {
		t.Fatalf("expected scoped cleanup to drop the row via the symlinked path, got stdout:\n%s", stdout)
	}
	if _, err := os.Stat(sibling); !os.IsNotExist(err) {
		t.Fatalf("expected worktree removed, stat err: %v", err)
	}
	assertCatalogAllocations(t, catalogPathFor(repo), 0)
}

func resolvePath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolve %s: %v", path, err)
	}
	return resolved
}

func TestWorktreeRemoveExplicitPath(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	if code, _, stderr := runCLI(t, []string{"worktree", "create", "feat", "--cwd", repo}); code != 0 {
		t.Fatalf("create failed: %d\n%s", code, stderr)
	}
	sibling := siblingPath(t, repo, "feat")

	code, stdout, stderr := runCLI(t, []string{"worktree", "remove", "ignored-lane", "--cwd", repo, "--path", sibling, "--force"})
	if code != 0 {
		t.Fatalf("expected exit 0 with explicit --path, got %d\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stdout, "removed worktree") {
		t.Fatalf("expected removal confirmation, got stdout:\n%s", stdout)
	}
	if _, err := os.Stat(sibling); !os.IsNotExist(err) {
		t.Fatalf("expected worktree removed via --path, stat err: %v", err)
	}
}

func appendWorktreeSeeds(t *testing.T, repo string, seeds []string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("\nworktree:\n  seed:\n")
	for _, seed := range seeds {
		b.WriteString("    - ")
		b.WriteString(seed)
		b.WriteString("\n")
	}
	f, err := os.OpenFile(filepath.Join(repo, "devlane.yaml"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open adapter: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(b.String()); err != nil {
		t.Fatalf("append seeds: %v", err)
	}
}

func mustWriteRepoFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
