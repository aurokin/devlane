package cli_test

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// writeGitDir makes dir look like a real Git worktree root for nested-checkout
// detection: a .git directory containing a HEAD file. This is hermetic (no git
// subprocess) and matches the structural signal isNestedGitRoot inspects.
func writeGitDir(t *testing.T, dir string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(dir, ".git", "HEAD"), "ref: refs/heads/main\n")
}

// writeDoctorAdapter writes a minimal valid devlane.yaml under repoPath
// declaring the given app and services, so the host doctor loader
// (<repoPath>/devlane.yaml) parses and validates it cleanly.
func writeDoctorAdapter(t *testing.T, repoPath, app string, services ...string) {
	t.Helper()

	var b strings.Builder
	b.WriteString("schema: 1\n")
	b.WriteString("app: " + app + "\n")
	b.WriteString("kind: cli\n")
	b.WriteString("lane:\n")
	b.WriteString("  stable_name: stable\n")
	b.WriteString("  stable_branches: [main]\n")
	b.WriteString("  project_pattern: \"{app}_{lane}\"\n")
	b.WriteString("  path_roots:\n")
	b.WriteString("    state: .devlane/state\n")
	b.WriteString("    cache: .devlane/cache\n")
	b.WriteString("    runtime: .devlane/runtime\n")
	b.WriteString("outputs:\n")
	b.WriteString("  manifest_path: .devlane/manifest.json\n")
	b.WriteString("  generated: []\n")
	if len(services) > 0 {
		b.WriteString("ports:\n")
		for _, s := range services {
			b.WriteString("  - name: " + s + "\n")
		}
	}
	mustWriteFile(t, filepath.Join(repoPath, "devlane.yaml"), b.String())
}

func TestHostDoctorCleanCatalogExitsZero(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	repoA := filepath.Join(t.TempDir(), "agentchat")
	repoB := filepath.Join(t.TempDir(), "billing")
	writeDoctorAdapter(t, repoA, "agentchat", "web", "api")
	writeDoctorAdapter(t, repoB, "billing", "web")

	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "agentchat", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: repoA},
		{App: "agentchat", Lane: "dev", Mode: "dev", Service: "api", Port: 3101, RepoPath: repoA},
		{App: "billing", Lane: "dev", Mode: "dev", Service: "web", Port: 3200, RepoPath: repoB},
	})

	code, stdout, stderr := runCLI(t, []string{"host", "doctor"})
	if code != 0 {
		t.Fatalf("expected exit 0 on a clean catalog, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if strings.TrimSpace(stdout) != "no drift detected" {
		t.Fatalf("expected \"no drift detected\", got:\n%s", stdout)
	}
}

func TestHostDoctorEmptyCatalogExitsZero(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	code, stdout, stderr := runCLI(t, []string{"host", "doctor"})
	if code != 0 {
		t.Fatalf("expected exit 0 on an empty catalog, got %d\nstderr:\n%s", code, stderr)
	}
	if strings.TrimSpace(stdout) != "no drift detected" {
		t.Fatalf("expected \"no drift detected\", got:\n%s", stdout)
	}
}

func TestHostDoctorEachCategoryTriggersExitOne(t *testing.T) {
	t.Run("missing-repoPath", func(t *testing.T) {
		configHome := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", configHome)

		gone := filepath.Join(t.TempDir(), "gone") // no devlane.yaml here
		seedCatalog(t, configHome, []hostStatusSeedRow{
			{App: "agentchat", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: gone},
		})

		code, stdout, _ := runCLI(t, []string{"host", "doctor"})
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\n%s", code, stdout)
		}
		assertContainsAll(t, stdout, "missing-repoPath", "agentchat", "web", "3100", gone)
	})

	t.Run("missing-service", func(t *testing.T) {
		configHome := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", configHome)

		repo := filepath.Join(t.TempDir(), "agentchat")
		writeDoctorAdapter(t, repo, "agentchat", "web") // "worker" no longer declared
		seedCatalog(t, configHome, []hostStatusSeedRow{
			{App: "agentchat", Lane: "dev", Mode: "dev", Service: "worker", Port: 3100, RepoPath: repo},
		})

		code, stdout, _ := runCLI(t, []string{"host", "doctor"})
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\n%s", code, stdout)
		}
		assertContainsAll(t, stdout, "missing-service", "worker")
	})

	t.Run("app-mismatch", func(t *testing.T) {
		configHome := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", configHome)

		repo := filepath.Join(t.TempDir(), "renamed")
		writeDoctorAdapter(t, repo, "newname", "web") // adapter now claims a different app
		seedCatalog(t, configHome, []hostStatusSeedRow{
			{App: "oldname", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: repo},
		})

		code, stdout, _ := runCLI(t, []string{"host", "doctor"})
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\n%s", code, stdout)
		}
		assertContainsAll(t, stdout, "app-mismatch", "oldname")
	})

	t.Run("duplicate-claim", func(t *testing.T) {
		configHome := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", configHome)

		repoA := filepath.Join(t.TempDir(), "agentchat")
		repoB := filepath.Join(t.TempDir(), "billing")
		writeDoctorAdapter(t, repoA, "agentchat", "web")
		writeDoctorAdapter(t, repoB, "billing", "web")
		seedCatalog(t, configHome, []hostStatusSeedRow{
			{App: "agentchat", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: repoA},
			{App: "billing", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: repoB},
		})

		code, stdout, _ := runCLI(t, []string{"host", "doctor"})
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\n%s", code, stdout)
		}
		assertContainsAll(t, stdout, "duplicate-claim", "agentchat", "billing")
	})

	t.Run("bad-adapter", func(t *testing.T) {
		configHome := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", configHome)

		repo := filepath.Join(t.TempDir(), "payments")
		// Present but invalid: missing required lane fields => validation error,
		// which is not os.ErrNotExist, so it classifies as bad-adapter.
		mustWriteFile(t, filepath.Join(repo, "devlane.yaml"), "schema: 1\napp: payments\nkind: web\n")
		seedCatalog(t, configHome, []hostStatusSeedRow{
			{App: "payments", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: repo},
		})

		code, stdout, _ := runCLI(t, []string{"host", "doctor"})
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\n%s", code, stdout)
		}
		assertContainsAll(t, stdout, "bad-adapter", "payments")
	})
}

func TestHostDoctorMultipleCategoriesInSingleRun(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	clean := filepath.Join(t.TempDir(), "clean")
	gone := filepath.Join(t.TempDir(), "gone")
	stale := filepath.Join(t.TempDir(), "stale")
	dupB := filepath.Join(t.TempDir(), "dup-b")
	writeDoctorAdapter(t, clean, "clean", "web")
	writeDoctorAdapter(t, stale, "stale", "web") // "worker" undeclared => missing-service
	writeDoctorAdapter(t, dupB, "dupb", "web")

	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "clean", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: clean},
		{App: "missing", Lane: "dev", Mode: "dev", Service: "web", Port: 3200, RepoPath: gone},
		{App: "stale", Lane: "dev", Mode: "dev", Service: "worker", Port: 3300, RepoPath: stale},
		{App: "clean", Lane: "dev", Mode: "dev", Service: "web", Port: 3500, RepoPath: clean + "-other"},
		{App: "dupb", Lane: "dev", Mode: "dev", Service: "web", Port: 3500, RepoPath: dupB},
	})

	code, stdout, stderr := runCLI(t, []string{"host", "doctor"})
	if code != 1 {
		t.Fatalf("expected exit 1 with multiple findings, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout,
		"missing-repoPath",
		"missing-service",
		"duplicate-claim",
	)
	// The clean, fully-declared row must not appear as a finding.
	if strings.Contains(stdout, "3100") {
		t.Fatalf("clean row (port 3100) should not be reported as a finding:\n%s", stdout)
	}
}

func TestHostDoctorRejectsPositionalArgs(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	code, _, stderr := runCLI(t, []string{"host", "doctor", "extra-arg"})
	if code != 2 {
		t.Fatalf("expected exit 2 for positional args, got %d\nstderr:\n%s", code, stderr)
	}
}

func TestHostDoctorSubtreeAdapterIsHealthy(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	root := t.TempDir()
	// The adapter lives in a subtree; the catalog row's repoPath is the worktree
	// root, as prepare records it. Discovery must find the subtree adapter so a
	// healthy monorepo allocation is not misreported as drift.
	writeDoctorAdapter(t, filepath.Join(root, "apps", "web"), "webui", "web")

	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "webui", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: root},
	})

	code, stdout, stderr := runCLI(t, []string{"host", "doctor"})
	if code != 0 {
		t.Fatalf("expected exit 0 for a healthy subtree adapter, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if strings.TrimSpace(stdout) != "no drift detected" {
		t.Fatalf("expected \"no drift detected\", got:\n%s", stdout)
	}
}

func TestHostDoctorDeepSubtreeAdapterIsHealthy(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	root := t.TempDir()
	// A real worktree root has a .git entry; discovery must descend past it
	// (skipping .git itself) rather than treating the root as opaque.
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir root .git: %v", err)
	}
	// prepare records repoPath as the worktree root no matter how deep --cwd
	// points, so discovery must not bound its depth.
	writeDoctorAdapter(t, filepath.Join(root, "a", "b", "c", "d", "e"), "deepapp", "web")

	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "deepapp", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: root},
	})

	code, stdout, stderr := runCLI(t, []string{"host", "doctor"})
	if code != 0 {
		t.Fatalf("expected exit 0 for a deep subtree adapter, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if strings.TrimSpace(stdout) != "no drift detected" {
		t.Fatalf("expected \"no drift detected\", got:\n%s", stdout)
	}
}

func TestHostDoctorIgnoresNestedGitRepoAdapters(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	root := t.TempDir()
	// A nested checkout has its own worktree root and its own catalog rows; its
	// adapter must not be attributed to the parent worktree's row.
	nested := filepath.Join(root, "nested")
	writeGitDir(t, nested)
	writeDoctorAdapter(t, nested, "nestedapp", "web")

	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "nestedapp", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: root},
	})

	code, stdout, _ := runCLI(t, []string{"host", "doctor"})
	if code != 1 {
		t.Fatalf("expected exit 1 (nested git adapter ignored), got %d\n%s", code, stdout)
	}
	assertContainsAll(t, stdout, "app-mismatch", "nestedapp")
}

func TestHostDoctorStrayGitEntryDoesNotHideAdapter(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	root := t.TempDir()
	// A stray, invalid .git entry inside an adapter directory is NOT a real
	// checkout, so discovery must still descend and find the adapter rather than
	// skip the subtree and emit a removable app-mismatch.
	adapterDir := filepath.Join(root, "apps", "web")
	writeDoctorAdapter(t, adapterDir, "webui", "web")
	mustWriteFile(t, filepath.Join(adapterDir, ".git"), "not a valid gitfile\n")

	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "webui", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: root},
	})

	code, stdout, stderr := runCLI(t, []string{"host", "doctor"})
	if code != 0 {
		t.Fatalf("expected exit 0 (stray .git must not hide a present adapter), got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if strings.TrimSpace(stdout) != "no drift detected" {
		t.Fatalf("expected \"no drift detected\", got:\n%s", stdout)
	}
}

func TestHostDoctorUnreadableDirReportedAsBadAdapterNotRemovable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions are not enforced the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permission bits")
	}

	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	root := t.TempDir()
	// The only adapter lives under a directory we then make unreadable. The row
	// must NOT be reported as app-mismatch (a removable category); the scan
	// uncertainty must surface as the non-removable bad-adapter instead.
	apps := filepath.Join(root, "apps")
	writeDoctorAdapter(t, filepath.Join(apps, "web"), "webui", "web")
	if err := os.Chmod(apps, 0o000); err != nil {
		t.Fatalf("chmod apps: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(apps, 0o755) }) // restore so TempDir cleanup works

	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "webui", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: root},
	})

	code, stdout, _ := runCLI(t, []string{"host", "doctor"})
	if code != 1 {
		t.Fatalf("expected exit 1, got %d\n%s", code, stdout)
	}
	assertContainsAll(t, stdout, "bad-adapter", "webui")
	if strings.Contains(stdout, "app-mismatch") {
		t.Fatalf("unreadable dir must not yield removable app-mismatch:\n%s", stdout)
	}
}

func TestHostDoctorIgnoresAdaptersInSkippedTrees(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	root := t.TempDir()
	// An adapter buried in node_modules must NOT be discovered, so a row whose
	// app only lives there surfaces as drift instead of looking healthy.
	writeDoctorAdapter(t, filepath.Join(root, "node_modules", "pkg"), "vendored", "web")

	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "vendored", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: root},
	})

	code, stdout, _ := runCLI(t, []string{"host", "doctor"})
	if code != 1 {
		t.Fatalf("expected exit 1 (adapter in node_modules ignored), got %d\n%s", code, stdout)
	}
	assertContainsAll(t, stdout, "app-mismatch", "vendored")
}

func TestHostDoctorHandlesCyclicDirectorySymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory symlinks behave differently on Windows")
	}
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	root := t.TempDir()
	writeDoctorAdapter(t, filepath.Join(root, "apps", "web"), "webui", "web")
	// A cyclic directory symlink must not cause unbounded recursion: the walk
	// skips symlinked directories, so this completes and finds the real adapter.
	if err := os.Symlink(root, filepath.Join(root, "loop")); err != nil {
		t.Fatalf("symlink loop: %v", err)
	}

	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "webui", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: root},
	})

	code, stdout, stderr := runCLI(t, []string{"host", "doctor"})
	if code != 0 {
		t.Fatalf("expected exit 0 (cyclic symlink skipped, adapter healthy), got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if strings.TrimSpace(stdout) != "no drift detected" {
		t.Fatalf("expected \"no drift detected\", got:\n%s", stdout)
	}
}

func TestHostDoctorDoesNotFollowSymlinkedDirToAdapter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory symlinks behave differently on Windows")
	}
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	root := t.TempDir()
	// The adapter lives in an out-of-tree directory; a symlink under the worktree
	// aliases it. Discovery must NOT follow the symlink, so the row stays drift
	// rather than being falsely reported healthy via the alias.
	external := t.TempDir()
	writeDoctorAdapter(t, filepath.Join(external, "web"), "webui", "web")
	if err := os.Symlink(external, filepath.Join(root, "aliased")); err != nil {
		t.Fatalf("symlink aliased: %v", err)
	}

	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "webui", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: root},
	})

	code, stdout, _ := runCLI(t, []string{"host", "doctor"})
	if code != 1 {
		t.Fatalf("expected exit 1 (symlinked adapter not followed), got %d\n%s", code, stdout)
	}
	assertContainsAll(t, stdout, "app-mismatch", "webui")
}

func TestHostDoctorAdapterPathIsDirectoryReportedAsBadAdapter(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	root := t.TempDir()
	// devlane.yaml exists but is a directory, not a loadable adapter file. This
	// anomalous path must surface as bad-adapter, never the removable
	// app-mismatch.
	if err := os.MkdirAll(filepath.Join(root, "devlane.yaml"), 0o755); err != nil {
		t.Fatalf("mkdir devlane.yaml dir: %v", err)
	}

	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "webui", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: root},
	})

	code, stdout, _ := runCLI(t, []string{"host", "doctor"})
	if code != 1 {
		t.Fatalf("expected exit 1, got %d\n%s", code, stdout)
	}
	assertContainsAll(t, stdout, "bad-adapter", "webui")
	if strings.Contains(stdout, "app-mismatch") {
		t.Fatalf("a directory adapter path must not yield removable app-mismatch:\n%s", stdout)
	}
}

func TestHostDoctorUnreachableRepoPathReportedAsBadAdapter(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	// The worktree root is absent AND its parent is also absent — the signature of
	// an unmounted/unreachable volume rather than a deletion. doctor must surface
	// it as the non-removable bad-adapter, never the removable missing-repoPath, so
	// a transiently-offline volume can never look safe for host gc to delete.
	repoPath := filepath.Join(t.TempDir(), "missing-parent", "web")
	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "agentchat", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: repoPath},
	})

	code, stdout, _ := runCLI(t, []string{"host", "doctor"})
	if code != 1 {
		t.Fatalf("expected exit 1, got %d\n%s", code, stdout)
	}
	assertContainsAll(t, stdout, "bad-adapter", "agentchat", "unreachable")
	if strings.Contains(stdout, "missing-repoPath") {
		t.Fatalf("an unreachable repoPath must not be reported as removable missing-repoPath:\n%s", stdout)
	}
}

func TestHostDoctorProbesAndReportsBoundFree(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	// Hold an IPv4 port so the doctor's probe observes it as bound.
	boundListener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen bound port: %v", err)
	}
	defer boundListener.Close()
	boundPort := boundListener.Addr().(*net.TCPAddr).Port

	// Acquire then release a port so it is free when the doctor probes it.
	freeListener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen free port: %v", err)
	}
	freePort := freeListener.Addr().(*net.TCPAddr).Port
	_ = freeListener.Close()

	// Both rows point at a missing repo so they surface as findings; the probe
	// column is the thing under test here.
	gone := filepath.Join(t.TempDir(), "gone")
	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "app", Lane: "dev", Mode: "dev", Service: "bound", Port: boundPort, RepoPath: gone},
		{App: "app", Lane: "dev", Mode: "dev", Service: "free", Port: freePort, RepoPath: gone},
	})

	code, stdout, stderr := runCLI(t, []string{"host", "doctor"})
	if code != 1 {
		t.Fatalf("expected exit 1, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	// Header carries the PROBE column.
	header := strings.SplitN(strings.TrimLeft(stdout, "\n"), "\n", 2)[0]
	if !strings.Contains(header, "PROBE") {
		t.Fatalf("expected PROBE column in header: %q", header)
	}
	if got := probeFieldForPort(t, stdout, boundPort); got != "bound" {
		t.Fatalf("expected port %d to probe as bound, got %q\n%s", boundPort, got, stdout)
	}
	if got := probeFieldForPort(t, stdout, freePort); got != "free" {
		t.Fatalf("expected port %d to probe as free, got %q\n%s", freePort, got, stdout)
	}
}

func TestHostDoctorNeverMutatesCatalog(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	// Seed a catalog containing drift so the finding-printing path runs.
	gone := filepath.Join(t.TempDir(), "gone")
	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "app", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: gone},
	})

	catalogPath := filepath.Join(configHome, "devlane", "catalog.json")
	before, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatalf("read catalog before: %v", err)
	}
	beforeInfo, err := os.Stat(catalogPath)
	if err != nil {
		t.Fatalf("stat catalog before: %v", err)
	}

	code, _, _ := runCLI(t, []string{"host", "doctor"})
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}

	after, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatalf("read catalog after: %v", err)
	}
	afterInfo, err := os.Stat(catalogPath)
	if err != nil {
		t.Fatalf("stat catalog after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("host doctor mutated catalog contents:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if !beforeInfo.ModTime().Equal(afterInfo.ModTime()) {
		t.Fatalf("host doctor changed catalog mtime: %v -> %v", beforeInfo.ModTime(), afterInfo.ModTime())
	}
}

// probeFieldForPort returns the PROBE column value for the finding row whose
// PORT column equals port. The table is tab-aligned with spaces, so the fixed
// leading columns (CATEGORY APP MODE LANE SERVICE PORT PROBE ...) are stable
// under strings.Fields; repoPath and detail trail the PROBE column.
func probeFieldForPort(t *testing.T, stdout string, port int) string {
	t.Helper()
	portStr := strconv.Itoa(port)
	for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}
		if fields[5] == portStr { // PORT column
			return fields[6] // PROBE column
		}
	}
	t.Fatalf("no finding row found for port %d in:\n%s", port, stdout)
	return ""
}

func assertContainsAll(t *testing.T, haystack string, needles ...string) {
	t.Helper()
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			t.Fatalf("expected output to contain %q:\n%s", needle, haystack)
		}
	}
}
