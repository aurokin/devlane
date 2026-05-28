package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/auro/devlane/internal/portalloc"
)

type gcSeedRow struct {
	App      string
	Service  string
	Port     int
	RepoPath string
}

func seedGcCatalog(t *testing.T, configHome string, rows []gcSeedRow) {
	t.Helper()

	catalogDir := filepath.Join(configHome, "devlane")
	if err := os.MkdirAll(catalogDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	allocations := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		allocations = append(allocations, map[string]any{
			"app":          r.App,
			"lane":         "dev",
			"mode":         "dev",
			"branch":       "main",
			"service":      r.Service,
			"port":         r.Port,
			"repoPath":     r.RepoPath,
			"lastPrepared": "2026-04-25T00:00:00Z",
		})
	}
	payload, err := json.Marshal(map[string]any{
		"schema":      1,
		"allocations": allocations,
	})
	if err != nil {
		t.Fatalf("marshal catalog: %v", err)
	}
	if err := os.WriteFile(filepath.Join(catalogDir, "catalog.json"), payload, 0o644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
}

func TestHostGcAtomicOnMutateFailure(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	gone := filepath.Join(t.TempDir(), "gone")
	seedGcCatalog(t, configHome, []gcSeedRow{
		{App: "stale", Service: "web", Port: 3200, RepoPath: gone},
	})

	original := mutateCatalog
	mutateCatalog = func(_ func(*portalloc.Snapshot) error) error {
		return errors.New("mutate catalog: forced failure")
	}
	defer func() { mutateCatalog = original }()

	code, _, stderr := runCLIInternal(t, []string{"host", "gc", "--yes"})
	if code != 1 {
		t.Fatalf("expected exit 1 when Mutate fails, got %d\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "forced failure") {
		t.Fatalf("expected forced mutate failure in stderr, got:\n%s", stderr)
	}

	rows := readCatalogAllocationsInternal(t, filepath.Join(configHome, "devlane", "catalog.json"))
	if len(rows) != 1 {
		t.Fatalf("expected the removable row to remain after a failed mutate, got %d: %#v", len(rows), rows)
	}
}

func TestHostGcInteractiveConfirmationAccepts(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	gone := filepath.Join(t.TempDir(), "gone")
	seedGcCatalog(t, configHome, []gcSeedRow{
		{App: "stale", Service: "web", Port: 3200, RepoPath: gone},
	})

	originalTTY := stdinIsInteractive
	stdinIsInteractive = func(*os.File) bool { return true }
	defer func() { stdinIsInteractive = originalTTY }()

	code, stdout, stderr := runCLIInternalWithInput(t, []string{"host", "gc"}, "y\n")
	if code != 0 {
		t.Fatalf("expected exit 0 on confirmed removal, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "removed 1") {
		t.Fatalf("expected \"removed 1\" in stdout, got:\n%s", stdout)
	}

	rows := readCatalogAllocationsInternal(t, filepath.Join(configHome, "devlane", "catalog.json"))
	if len(rows) != 0 {
		t.Fatalf("expected the removable row to be deleted after confirmation, got %d: %#v", len(rows), rows)
	}
}

func TestHostGcInteractiveConfirmationDeclines(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	gone := filepath.Join(t.TempDir(), "gone")
	seedGcCatalog(t, configHome, []gcSeedRow{
		{App: "stale", Service: "web", Port: 3200, RepoPath: gone},
	})

	originalTTY := stdinIsInteractive
	stdinIsInteractive = func(*os.File) bool { return true }
	defer func() { stdinIsInteractive = originalTTY }()

	code, _, stderr := runCLIInternalWithInput(t, []string{"host", "gc"}, "n\n")
	if code != 1 {
		t.Fatalf("expected exit 1 on declined removal, got %d\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "gc cancelled") {
		t.Fatalf("expected \"gc cancelled\" on stderr, got:\n%s", stderr)
	}

	rows := readCatalogAllocationsInternal(t, filepath.Join(configHome, "devlane", "catalog.json"))
	if len(rows) != 1 {
		t.Fatalf("expected the removable row to remain after a declined prompt, got %d: %#v", len(rows), rows)
	}
}

// writeGcAdapter writes a minimal valid devlane.yaml so the gc loader parses and
// validates it. It lives in the internal package (vs. the cli_test helper) so
// tests that drive the mutateCatalog seam can repair an adapter on disk.
func writeGcAdapter(t *testing.T, repoPath, app string, services ...string) {
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
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", repoPath, err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "devlane.yaml"), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write adapter: %v", err)
	}
}

func TestHostGcDoesNotRemoveRowRepairedBeforeLockedWrite(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	// At scan time the adapter declares the app but not the "worker" service, so
	// the row is missing-service (removable). The catalog row value never changes
	// — only the adapter on disk does — so a full-value match alone would still
	// delete it; the under-lock re-detect is what must preserve it.
	repo := filepath.Join(t.TempDir(), "agentchat")
	writeGcAdapter(t, repo, "agentchat", "web") // "worker" not declared yet
	seedGcCatalog(t, configHome, []gcSeedRow{
		{App: "agentchat", Service: "worker", Port: 3300, RepoPath: repo},
	})

	// Simulate the adapter being repaired (worker added back) after the unlocked
	// scan/confirmation but before the locked deletion — e.g. an interactive
	// prompt waiting while someone fixes the adapter. The re-detect under the lock
	// must reclassify the row as healthy and NOT delete it.
	original := mutateCatalog
	repaired := false
	mutateCatalog = func(fn func(*portalloc.Snapshot) error) error {
		if !repaired {
			repaired = true
			writeGcAdapter(t, repo, "agentchat", "web", "worker")
		}
		return original(fn)
	}
	defer func() { mutateCatalog = original }()

	code, stdout, stderr := runCLIInternal(t, []string{"host", "gc", "--yes"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "removed 0") {
		t.Fatalf("expected the repaired row to be preserved (removed 0), got:\n%s", stdout)
	}

	rows := readCatalogAllocationsInternal(t, filepath.Join(configHome, "devlane", "catalog.json"))
	if len(rows) != 1 {
		t.Fatalf("expected the repaired row to survive, got %d rows: %#v", len(rows), rows)
	}
	if rows[0].Service != "worker" || rows[0].Port != 3300 {
		t.Fatalf("expected the repaired worker/3300 row to remain unchanged, got %#v", rows[0])
	}
}

func TestHostGcDoesNotRemoveRowRefreshedAfterDetection(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	// At detection time the row points at a missing repoPath, so gc classifies it
	// removable and (under --yes) proceeds to delete it.
	gone := filepath.Join(t.TempDir(), "gone")
	seedGcCatalog(t, configHome, []gcSeedRow{
		{App: "stale", Service: "web", Port: 3200, RepoPath: gone},
	})

	// Simulate a concurrent prepare landing in the window between gc's unlocked
	// read and its locked write: rewrite the catalog row to a different state
	// (new port, new repoPath) before the real Mutate loads the snapshot.
	refreshed := filepath.Join(t.TempDir(), "refreshed")
	original := mutateCatalog
	rewritten := false
	mutateCatalog = func(fn func(*portalloc.Snapshot) error) error {
		if !rewritten {
			rewritten = true
			seedGcCatalog(t, configHome, []gcSeedRow{
				{App: "stale", Service: "web", Port: 4100, RepoPath: refreshed},
			})
		}
		return original(fn)
	}
	defer func() { mutateCatalog = original }()

	code, stdout, stderr := runCLIInternal(t, []string{"host", "gc", "--yes"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "removed 0") {
		t.Fatalf("expected refreshed row to be preserved (removed 0), got:\n%s", stdout)
	}

	rows := readCatalogAllocationsInternal(t, filepath.Join(configHome, "devlane", "catalog.json"))
	if len(rows) != 1 {
		t.Fatalf("expected the refreshed row to survive, got %d rows: %#v", len(rows), rows)
	}
	if rows[0].Port != 4100 || rows[0].RepoPath != refreshed {
		t.Fatalf("expected the refreshed row (port 4100, %s) to remain unchanged, got %#v", refreshed, rows[0])
	}
}

func TestStdinIsInteractiveRejectsNonTerminalCharDevices(t *testing.T) {
	// /dev/null is a character device, so an os.ModeCharDevice-based check would
	// wrongly classify it interactive and route an unattended `host gc < /dev/null`
	// into the prompt path. term.IsTerminal must report it non-interactive so gc
	// takes the documented refusal path instead.
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer devnull.Close()

	if stdinIsInteractive(devnull) {
		t.Fatalf("%s must not be classified as an interactive terminal", os.DevNull)
	}
	if stdinIsInteractive(nil) {
		t.Fatalf("nil stdin must not be classified as an interactive terminal")
	}
}

// runCLIInternalWithInput mirrors runCLIInternal but feeds input on stdin so the
// interactive confirmation path (gated by stdinIsInteractive) can be exercised.
func runCLIInternalWithInput(t *testing.T, args []string, input string) (int, string, string) {
	t.Helper()

	originalStdout := os.Stdout
	originalStderr := os.Stderr
	originalStdin := os.Stdin

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	if _, err := stdinWriter.WriteString(input); err != nil {
		t.Fatalf("stdin write: %v", err)
	}
	_ = stdinWriter.Close()

	os.Stdout = stdoutWriter
	os.Stderr = stderrWriter
	os.Stdin = stdinReader

	code := Run(args)

	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	os.Stdout = originalStdout
	os.Stderr = originalStderr
	os.Stdin = originalStdin

	return code, readGcPipeInternal(t, stdoutReader), readGcPipeInternal(t, stderrReader)
}

func readGcPipeInternal(t *testing.T, reader *os.File) string {
	t.Helper()
	var buffer bytes.Buffer
	if _, err := buffer.ReadFrom(reader); err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return buffer.String()
}
