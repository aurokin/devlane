package cli_test

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/auro/devlane/internal/config"
	"github.com/auro/devlane/internal/portalloc"
	"github.com/auro/devlane/internal/testutil"
)

func TestReassignRequiresService(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	prepareRepo(t, repo)

	code, _, stderr := runCLI(t, []string{
		"reassign",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
	})
	if code != 2 {
		t.Fatalf("expected exit code 2 for missing service, got %d\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "usage: devlane reassign") {
		t.Fatalf("expected usage hint, got stderr:\n%s", stderr)
	}
}

func TestReassignRejectsUnknownService(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	prepareRepo(t, repo)

	code, _, stderr := runCLI(t, []string{
		"reassign",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
		"unknown-service",
	})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, `does not declare service "unknown-service"`) {
		t.Fatalf("expected unknown-service error, got stderr:\n%s", stderr)
	}
}

func TestReassignNoOpWhenCurrentPortBindable(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	prepareRepo(t, repo)

	configHome := xdgConfigHome(t)
	before := readCatalogRows(t, configHome)

	code, stdout, stderr := runCLI(t, []string{
		"reassign",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
		"web",
	})
	if code != 0 {
		t.Fatalf("expected no-op exit 0, got %d\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stdout, "no change") {
		t.Fatalf("expected no-change message, got stdout:\n%s", stdout)
	}

	after := readCatalogRows(t, configHome)
	if len(before) != len(after) {
		t.Fatalf("expected catalog row count to be stable; before=%d after=%d", len(before), len(after))
	}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("row %d changed during no-op reassign:\n before=%#v\n after =%#v", i, before[i], after[i])
		}
	}
}

func TestReassignForceMovesOffTheCurrentPort(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	prepareRepo(t, repo)

	configHome := xdgConfigHome(t)
	beforePort := portForService(t, configHome, "demoapp", repo, "web")

	code, stdout, stderr := runCLI(t, []string{
		"reassign",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
		"--force",
		"web",
	})
	if code != 0 {
		t.Fatalf("expected --force reassign to exit 0, got %d\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stdout, "reassigned service") {
		t.Fatalf("expected reassign confirmation, got stdout:\n%s", stdout)
	}

	afterPort := portForService(t, configHome, "demoapp", repo, "web")
	if afterPort == beforePort {
		t.Fatalf("expected --force to allocate a different port, both before/after = %d", beforePort)
	}
}

func TestReassignSingleServiceScopeLeavesOtherRowsUntouched(t *testing.T) {
	repoA := testutil.InitDemoRepo(t)
	repoB := testutil.InitDemoRepo(t)

	cmd := exec.Command("git", "checkout", "-B", "feature/other-lane")
	cmd.Dir = repoB
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout: %v\n%s", err, output)
	}

	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	prepareRepoOnly(t, repoA)
	prepareRepoOnly(t, repoB)

	beforeA := portForService(t, configHome, "demoapp", repoA, "web")
	beforeB := portForService(t, configHome, "demoapp", repoB, "web")

	code, _, stderr := runCLI(t, []string{
		"reassign",
		"--cwd", repoB,
		"--config", filepath.Join(repoB, "devlane.yaml"),
		"--force",
		"web",
	})
	if code != 0 {
		t.Fatalf("expected reassign to succeed, got %d\nstderr:\n%s", code, stderr)
	}

	afterA := portForService(t, configHome, "demoapp", repoA, "web")
	afterB := portForService(t, configHome, "demoapp", repoB, "web")

	if afterA != beforeA {
		t.Fatalf("expected repoA web row untouched: before=%d after=%d", beforeA, afterA)
	}
	if afterB == beforeB {
		t.Fatalf("expected repoB web row reassigned: before=%d after=%d", beforeB, afterB)
	}
}

func TestReassignLaneHappyPathFromAnotherCheckout(t *testing.T) {
	mainRepo := testutil.InitDemoRepo(t)
	worktreeRepo := testutil.InitDemoRepo(t)

	if output, err := exec.Command("git", "-C", worktreeRepo, "checkout", "-B", "feature/worktree-lane").CombinedOutput(); err != nil {
		t.Fatalf("git checkout: %v\n%s", err, output)
	}

	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	prepareRepoOnly(t, mainRepo)
	prepareRepoOnly(t, worktreeRepo)

	worktreePort := portForService(t, configHome, "demoapp", worktreeRepo, "web")

	// Hold the worktree's current port from outside so the row's existing port
	// is unbindable; reassign --lane should then move the worktree onto a fresh
	// pool port even without --force.
	listener, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", worktreePort))
	if err != nil {
		t.Fatalf("listen on %d: %v", worktreePort, err)
	}
	defer listener.Close()

	code, stdout, stderr := runCLI(t, []string{
		"reassign",
		"--cwd", mainRepo,
		"--config", filepath.Join(mainRepo, "devlane.yaml"),
		"--lane", "feature/worktree-lane",
		"web",
	})
	if code != 0 {
		t.Fatalf("expected --lane reassign to succeed, got %d\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stdout, "reassigned service") || !strings.Contains(stdout, "feature/worktree-lane") {
		t.Fatalf("expected confirmation naming target lane, got stdout:\n%s", stdout)
	}

	afterWorktreePort := portForService(t, configHome, "demoapp", worktreeRepo, "web")
	if afterWorktreePort == worktreePort {
		t.Fatalf("expected worktree port to move off %d, still on it", worktreePort)
	}

	// Main repo lane should be unchanged because reassign targeted the worktree.
	mainPort := portForService(t, configHome, "demoapp", mainRepo, "web")
	if mainPort != 3000 {
		t.Fatalf("expected main repo lane to stay on 3000, got %d", mainPort)
	}
}

func TestReassignLaneAmbiguityEnumeratesCheckouts(t *testing.T) {
	repoA := testutil.InitDemoRepo(t)
	repoB := testutil.InitDemoRepo(t)
	thirdRepo := testutil.InitDemoRepo(t)

	if output, err := exec.Command("git", "-C", thirdRepo, "checkout", "-B", "feature/third-lane").CombinedOutput(); err != nil {
		t.Fatalf("git checkout: %v\n%s", err, output)
	}

	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	// repoA and repoB both stay on the testutil-default branch "feature/test-lane"
	// so two checkouts share the lane name. The third repo runs reassign from
	// a different lane so the current-checkout tiebreak doesn't pick either.
	prepareRepoOnly(t, repoA)
	prepareRepoOnly(t, repoB)

	code, _, stderr := runCLI(t, []string{
		"reassign",
		"--cwd", thirdRepo,
		"--config", filepath.Join(thirdRepo, "devlane.yaml"),
		"--lane", "feature/test-lane",
		"web",
	})
	if code != 1 {
		t.Fatalf("expected ambiguity exit 1, got %d\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "ambiguous") {
		t.Fatalf("expected ambiguity error, got stderr:\n%s", stderr)
	}
	canonicalA := canonicalPath(t, repoA)
	canonicalB := canonicalPath(t, repoB)
	if !strings.Contains(stderr, canonicalA) || !strings.Contains(stderr, canonicalB) {
		t.Fatalf("expected ambiguity to enumerate both canonical repoPaths %q / %q, got stderr:\n%s", canonicalA, canonicalB, stderr)
	}
}

func TestReassignLaneNotFoundExitsWithClearError(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	prepareRepo(t, repo)

	code, _, stderr := runCLI(t, []string{
		"reassign",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
		"--lane", "ghost-lane",
		"web",
	})
	if code != 1 {
		t.Fatalf("expected not-found exit 1, got %d\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, `no allocation found for lane "ghost-lane"`) {
		t.Fatalf("expected not-found error, got stderr:\n%s", stderr)
	}
}

func TestReassignLaneCurrentCheckoutWinsTiebreak(t *testing.T) {
	repoA := testutil.InitDemoRepo(t)
	repoB := testutil.InitDemoRepo(t)

	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	prepareRepoOnly(t, repoA)
	prepareRepoOnly(t, repoB)

	beforeA := portForService(t, configHome, "demoapp", repoA, "web")
	beforeB := portForService(t, configHome, "demoapp", repoB, "web")

	// Both checkouts share lane "feature/test-lane" — a tiebreak by current
	// repoPath should silently pick repoB because the command runs from there.
	code, stdout, stderr := runCLI(t, []string{
		"reassign",
		"--cwd", repoB,
		"--config", filepath.Join(repoB, "devlane.yaml"),
		"--lane", "feature/test-lane",
		"--force",
		"web",
	})
	if code != 0 {
		t.Fatalf("expected tiebreak to succeed, got %d\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stdout, "reassigned service") {
		t.Fatalf("expected reassign confirmation, got stdout:\n%s", stdout)
	}

	afterA := portForService(t, configHome, "demoapp", repoA, "web")
	afterB := portForService(t, configHome, "demoapp", repoB, "web")
	if afterA != beforeA {
		t.Fatalf("expected repoA row untouched after current-checkout tiebreak: before=%d after=%d", beforeA, afterA)
	}
	if afterB == beforeB {
		t.Fatalf("expected repoB row to move under --force: before=%d after=%d", beforeB, afterB)
	}
}

func TestReassignSerializesWithConcurrentPrepare(t *testing.T) {
	repoA := testutil.InitDemoRepo(t)
	repoB := testutil.InitDemoRepo(t)

	if output, err := exec.Command("git", "-C", repoB, "checkout", "-B", "feature/other-lane").CombinedOutput(); err != nil {
		t.Fatalf("git checkout: %v\n%s", err, output)
	}

	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	prepareRepoOnly(t, repoA)

	// Begin a Prepare on repoB inside the test process so its catalog lock is
	// held while we kick off a reassign on repoA in parallel. The reassign
	// must wait for the lock instead of racing.
	adapterB, laneB := loadLaneFromRepo(t, repoB)
	session, err := portalloc.BeginPrepare(adapterB, laneB)
	if err != nil {
		t.Fatalf("begin prepare repoB: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	type cliResult struct {
		code   int
		stdout string
		stderr string
	}
	done := make(chan cliResult, 1)
	go func() {
		code, stdout, stderr := runCLI(t, []string{
			"reassign",
			"--cwd", repoA,
			"--config", filepath.Join(repoA, "devlane.yaml"),
			"--force",
			"web",
		})
		done <- cliResult{code, stdout, stderr}
	}()

	select {
	case r := <-done:
		t.Fatalf("expected reassign to wait for catalog lock, got %#v", r)
	case <-time.After(200 * time.Millisecond):
	}

	if err := session.Commit(); err != nil {
		t.Fatalf("commit prepare repoB: %v", err)
	}

	select {
	case r := <-done:
		if r.code != 0 {
			t.Fatalf("reassign after Prepare release exit %d\nstderr:\n%s", r.code, r.stderr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("reassign did not complete after Prepare released the lock")
	}
}

// helpers --------------------------------------------------------------------

// xdgConfigHome returns the XDG_CONFIG_HOME the test set on its environment.
// It must be called after testutil.InitDemoRepo (which seeds XDG_CONFIG_HOME)
// or after an explicit t.Setenv.
func xdgConfigHome(t *testing.T) string {
	t.Helper()
	value := os.Getenv("XDG_CONFIG_HOME")
	if value == "" {
		t.Fatal("XDG_CONFIG_HOME is unset; call testutil.InitDemoRepo or t.Setenv first")
	}
	return value
}

// prepareRepo runs prepare against the given repo using its bundled adapter.
func prepareRepo(t *testing.T, repo string) {
	t.Helper()
	prepareRepoOnly(t, repo)
}

func prepareRepoOnly(t *testing.T, repo string) {
	t.Helper()
	code, stdout, stderr := runCLI(t, []string{
		"prepare",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
	})
	if code != 0 {
		t.Fatalf("prepare for %s failed: code=%d\nstdout:\n%s\nstderr:\n%s", repo, code, stdout, stderr)
	}
}

func readCatalogRows(t *testing.T, configHome string) []portalloc.Allocation {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join(configHome, "devlane", "catalog.json"))
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	var stored struct {
		Allocations []portalloc.Allocation `json:"allocations"`
	}
	if err := json.Unmarshal(payload, &stored); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	return stored.Allocations
}

func portForService(t *testing.T, configHome, app, repoPath, service string) int {
	t.Helper()
	rows := readCatalogRows(t, configHome)
	wantCanonical := canonicalPath(t, repoPath)
	for _, row := range rows {
		if row.App != app || row.Service != service {
			continue
		}
		if canonicalPath(t, row.RepoPath) != wantCanonical {
			continue
		}
		return row.Port
	}
	t.Fatalf("no catalog row for app=%s service=%s repoPath=%s\nrows=%#v", app, service, repoPath, rows)
	return 0
}

func canonicalPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(resolved)
}

func loadLaneFromRepo(t *testing.T, repo string) (*config.AdapterConfig, portalloc.Lane) {
	t.Helper()
	adapter, err := config.LoadAdapter(filepath.Join(repo, "devlane.yaml"))
	if err != nil {
		t.Fatalf("load adapter: %v", err)
	}
	branch := currentBranch(t, repo)
	return adapter, portalloc.Lane{
		App:      adapter.App,
		RepoPath: repo,
		Name:     branch,
		Mode:     "dev",
		Branch:   branch,
		Stable:   false,
	}
}

func currentBranch(t *testing.T, repo string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}
