package cli_test

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostGcDryRunRemovesNothing(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	gone := filepath.Join(t.TempDir(), "gone") // missing repoPath => removable
	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "agentchat", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: gone},
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

	code, stdout, stderr := runCLI(t, []string{"host", "gc", "--dry-run"})
	if code != 0 {
		t.Fatalf("expected exit 0 for dry-run, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "missing-repoPath", "agentchat", "dry-run")

	after, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatalf("read catalog after: %v", err)
	}
	afterInfo, err := os.Stat(catalogPath)
	if err != nil {
		t.Fatalf("stat catalog after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("dry-run mutated catalog contents:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if !beforeInfo.ModTime().Equal(afterInfo.ModTime()) {
		t.Fatalf("dry-run changed catalog mtime: %v -> %v", beforeInfo.ModTime(), afterInfo.ModTime())
	}
}

func TestHostGcYesRemovesRemovableKeepsHealthy(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	healthy := filepath.Join(t.TempDir(), "agentchat")
	writeDoctorAdapter(t, healthy, "agentchat", "web")
	gone := filepath.Join(t.TempDir(), "gone")

	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "agentchat", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: healthy},
		{App: "stale", Lane: "dev", Mode: "dev", Service: "web", Port: 3200, RepoPath: gone},
	})

	code, stdout, stderr := runCLI(t, []string{"host", "gc", "--yes"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "removed 1")

	rows := readCatalogRows(t, configHome)
	if len(rows) != 1 {
		t.Fatalf("expected 1 surviving row, got %d: %#v", len(rows), rows)
	}
	if rows[0].App != "agentchat" || rows[0].Port != 3100 {
		t.Fatalf("expected healthy agentchat/3100 row to survive, got %#v", rows[0])
	}
}

func TestHostGcNoTtyNoYesRefuses(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	gone := filepath.Join(t.TempDir(), "gone")
	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "agentchat", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: gone},
	})

	code, stdout, stderr := runCLI(t, []string{"host", "gc"})
	if code != 1 {
		t.Fatalf("expected exit 1 when refusing without --yes and no TTY, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "refusing to remove without confirmation") {
		t.Fatalf("expected refusal message on stderr, got:\n%s", stderr)
	}

	rows := readCatalogRows(t, configHome)
	if len(rows) != 1 {
		t.Fatalf("expected the removable row to remain untouched, got %d rows: %#v", len(rows), rows)
	}
}

func TestHostGcAppFilterScopesRemoval(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	goneA := filepath.Join(t.TempDir(), "gone-a")
	goneB := filepath.Join(t.TempDir(), "gone-b")
	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "appa", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: goneA},
		{App: "appb", Lane: "dev", Mode: "dev", Service: "web", Port: 3200, RepoPath: goneB},
	})

	code, stdout, stderr := runCLI(t, []string{"host", "gc", "--app", "appa", "--yes"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "removed 1")

	rows := readCatalogRows(t, configHome)
	if len(rows) != 1 {
		t.Fatalf("expected only appb to remain, got %d rows: %#v", len(rows), rows)
	}
	if rows[0].App != "appb" {
		t.Fatalf("expected appb to survive --app appa scope, got %#v", rows[0])
	}
}

func TestHostGcSurfacedFindingsAreNotRemoved(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	// Two healthy rows that collide on one port => duplicate-claim, which is
	// surfaced-only. There is nothing removable, so gc must exit 0 and keep both.
	repoA := filepath.Join(t.TempDir(), "appa")
	repoB := filepath.Join(t.TempDir(), "appb")
	writeDoctorAdapter(t, repoA, "appa", "web")
	writeDoctorAdapter(t, repoB, "appb", "web")
	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "appa", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: repoA},
		{App: "appb", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: repoB},
	})

	code, stdout, stderr := runCLI(t, []string{"host", "gc", "--yes"})
	if code != 0 {
		t.Fatalf("expected exit 0 when only surfaced findings exist, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "duplicate-claim", "manual decision", "no removable drift")

	rows := readCatalogRows(t, configHome)
	if len(rows) != 2 {
		t.Fatalf("expected both surfaced rows to remain, got %d: %#v", len(rows), rows)
	}
}

func TestHostGcBoundButSinglyClaimedRowPreserved(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	// A bound port that is claimed by exactly one healthy row is not drift, so gc
	// must never remove it. Hold the port so it probes as bound.
	listener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	boundPort := listener.Addr().(*net.TCPAddr).Port

	repo := filepath.Join(t.TempDir(), "agentchat")
	writeDoctorAdapter(t, repo, "agentchat", "web")
	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "agentchat", Lane: "dev", Mode: "dev", Service: "web", Port: boundPort, RepoPath: repo},
	})

	code, stdout, stderr := runCLI(t, []string{"host", "gc", "--yes"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "no removable drift") {
		t.Fatalf("expected \"no removable drift\", got:\n%s", stdout)
	}

	rows := readCatalogRows(t, configHome)
	if len(rows) != 1 {
		t.Fatalf("expected the singly-claimed bound row to remain, got %d: %#v", len(rows), rows)
	}
}

func TestHostGcRemovesDeadRowThatAlsoDuplicateClaims(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	// Two rows collide on port 3100. Row A's worktree is gone (missing-repoPath,
	// removable) AND it duplicate-claims the port; row B is healthy and only
	// duplicate-claims. The duplicate-claim must not shield the independently-dead
	// row A from cleanup: gc removes A on its removable finding, prints the
	// duplicate-claim under the manual-decision section, and leaves healthy B as
	// the sole owner of 3100. Partitioning is per finding, not per row.
	gone := filepath.Join(t.TempDir(), "gone")
	healthy := filepath.Join(t.TempDir(), "agentchat")
	writeDoctorAdapter(t, healthy, "agentchat", "web")
	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "stale", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: gone},
		{App: "agentchat", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: healthy},
	})

	code, stdout, stderr := runCLI(t, []string{"host", "gc", "--yes"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "removed 1", "duplicate-claim", "manual decision")

	rows := readCatalogRows(t, configHome)
	if len(rows) != 1 {
		t.Fatalf("expected only the healthy row to survive, got %d: %#v", len(rows), rows)
	}
	if rows[0].App != "agentchat" || rows[0].Port != 3100 {
		t.Fatalf("expected healthy agentchat/3100 row to survive as sole claimant, got %#v", rows[0])
	}
}

func TestHostGcDoesNotRemoveUnreachableRepoPath(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	// The worktree root is absent AND its parent is also absent (e.g. an unmounted
	// volume), so the absence cannot be trusted as a deletion. The row surfaces as
	// bad-adapter, not the removable missing-repoPath, so gc must keep it — never
	// delete a live allocation whose volume was merely offline.
	repoPath := filepath.Join(t.TempDir(), "missing-parent", "web")
	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "agentchat", Lane: "dev", Mode: "dev", Service: "web", Port: 3100, RepoPath: repoPath},
	})

	code, stdout, stderr := runCLI(t, []string{"host", "gc", "--yes"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "no removable drift") {
		t.Fatalf("expected \"no removable drift\" (unreachable row is not removable), got:\n%s", stdout)
	}

	rows := readCatalogRows(t, configHome)
	if len(rows) != 1 {
		t.Fatalf("expected the unreachable row to remain, got %d: %#v", len(rows), rows)
	}
}

func TestHostGcRejectsPositionalArgs(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	code, _, stderr := runCLI(t, []string{"host", "gc", "extra-arg"})
	if code != 2 {
		t.Fatalf("expected exit 2 for positional args, got %d\nstderr:\n%s", code, stderr)
	}
}
