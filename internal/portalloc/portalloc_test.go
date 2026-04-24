package portalloc_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/auro/devlane/internal/config"
	"github.com/auro/devlane/internal/manifest"
	"github.com/auro/devlane/internal/portalloc"
	"github.com/auro/devlane/internal/testutil"
)

func TestBeginPrepareSerializesCatalogWriters(t *testing.T) {
	repoA := testutil.InitDemoRepo(t)
	repoB := testutil.InitDemoRepo(t)
	sharedConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", sharedConfig)

	cmd := exec.Command("git", "checkout", "-B", "feature/other-lane")
	cmd.Dir = repoB
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout failed: %v\n%s", err, output)
	}

	adapterA, laneA := buildLane(t, repoA, "")
	adapterB, laneB := buildLane(t, repoB, "")

	sessionA, err := portalloc.BeginPrepare(adapterA, laneA)
	if err != nil {
		t.Fatalf("begin prepare A: %v", err)
	}
	t.Cleanup(func() {
		_ = sessionA.Close()
	})

	type prepareResult struct {
		session *portalloc.PrepareSession
		err     error
	}

	done := make(chan prepareResult, 1)
	go func() {
		session, beginErr := portalloc.BeginPrepare(adapterB, laneB)
		done <- prepareResult{session: session, err: beginErr}
	}()

	select {
	case result := <-done:
		if result.session != nil {
			_ = result.session.Close()
		}
		t.Fatalf("expected second prepare to wait for the catalog lock, got err=%v", result.err)
	case <-time.After(200 * time.Millisecond):
	}

	if err := sessionA.Commit(); err != nil {
		t.Fatalf("commit prepare A: %v", err)
	}

	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("begin prepare B: %v", result.err)
		}
		t.Cleanup(func() {
			_ = result.session.Close()
		})
		if got := result.session.States()["web"].Port; got != 3001 {
			t.Fatalf("expected second lane to allocate port 3001, got %d", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second prepare")
	}
}

func TestRelativeXDGConfigHomeFallsBackToUserConfigDir(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("XDG_CONFIG_HOME", "relative-config")
	adapter, lane := buildLane(t, repo, "")

	if _, _, err := portalloc.Prepare(adapter, lane); err != nil {
		t.Fatalf("prepare: %v", err)
	}

	if _, err := os.Stat(filepath.Join("relative-config", "devlane", "catalog.json")); !os.IsNotExist(err) {
		t.Fatalf("relative XDG_CONFIG_HOME should not be used, stat err=%v", err)
	}

	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("resolve user config dir: %v", err)
	}
	if !filepath.IsAbs(userConfigDir) {
		userConfigDir = filepath.Join(userHome, ".config")
	}
	if _, err := os.Stat(filepath.Join(userConfigDir, "devlane", "catalog.json")); err != nil {
		t.Fatalf("expected catalog under user config dir: %v", err)
	}
}

func TestPreparePersistsModeAndBranchMetadata(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	sharedConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", sharedConfig)

	adapter, lane := buildLane(t, repo, "stable")
	states, ready, err := portalloc.Prepare(adapter, lane)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !ready || !states["web"].Allocated {
		t.Fatalf("expected committed port state, got ready=%t state=%#v", ready, states["web"])
	}

	payload, err := os.ReadFile(filepath.Join(sharedConfig, "devlane", "catalog.json"))
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}

	var stored struct {
		Allocations []struct {
			Mode   string `json:"mode"`
			Branch string `json:"branch"`
		} `json:"allocations"`
	}
	if err := json.Unmarshal(payload, &stored); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	if len(stored.Allocations) != 1 {
		t.Fatalf("expected 1 allocation, got %d", len(stored.Allocations))
	}
	if stored.Allocations[0].Mode != "stable" {
		t.Fatalf("expected stable mode metadata, got %q", stored.Allocations[0].Mode)
	}
	if stored.Allocations[0].Branch != "feature/test-lane" {
		t.Fatalf("expected branch metadata, got %q", stored.Allocations[0].Branch)
	}
}

func TestPrepareBackfillsLegacyCatalogMetadataBeforeSave(t *testing.T) {
	repoA := testutil.InitDemoRepo(t)
	repoB := testutil.InitDemoRepo(t)
	sharedConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", sharedConfig)

	catalogPath := filepath.Join(sharedConfig, "devlane", "catalog.json")
	if err := os.MkdirAll(filepath.Dir(catalogPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(catalogPath, []byte(fmt.Sprintf(`{
  "schema": 1,
  "allocations": [
    {
      "app": "demoapp",
      "lane": "feature/test-lane",
      "service": "web",
      "port": 3000,
      "repoPath": %q,
      "lastPrepared": "2026-04-11T14:30:00Z"
    }
  ]
}
`, repoA)), 0o644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}

	adapter, lane := buildLane(t, repoB, "")
	if _, _, err := portalloc.Prepare(adapter, lane); err != nil {
		t.Fatalf("prepare: %v", err)
	}

	payload, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}

	var stored struct {
		Allocations []struct {
			RepoPath string `json:"repoPath"`
			Mode     string `json:"mode"`
			Branch   string `json:"branch"`
		} `json:"allocations"`
	}
	if err := json.Unmarshal(payload, &stored); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}

	var legacyRowFound bool
	for _, row := range stored.Allocations {
		if row.RepoPath != repoA {
			continue
		}
		legacyRowFound = true
		if row.Mode != "dev" {
			t.Fatalf("expected legacy row mode backfill to be dev, got %q", row.Mode)
		}
		if row.Branch != "feature/test-lane" {
			t.Fatalf("expected legacy row branch backfill, got %q", row.Branch)
		}
	}
	if !legacyRowFound {
		t.Fatalf("expected catalog to keep legacy row for %s", repoA)
	}
}

func TestPreparePrunesRemovedServiceAllocations(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	sharedConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", sharedConfig)

	adapter := rewritePortsBlock(t, repo, `ports:
  - name: web
    default: 3000
    health_path: /health
  - name: worker
    default: 3001`)
	lane := buildLaneFromAdapter(t, repo, adapter, "")

	states, ready, err := portalloc.Prepare(adapter, lane)
	if err != nil {
		t.Fatalf("prepare initial adapter: %v", err)
	}
	if !ready {
		t.Fatalf("expected initial prepare to allocate ports, got ready=%t", ready)
	}
	if states["worker"].Port != 3001 {
		t.Fatalf("expected worker to allocate default port 3001, got %#v", states["worker"])
	}

	adapter = rewritePortsBlock(t, repo, `ports:
  - name: worker
    default: 3001`)
	lane = buildLaneFromAdapter(t, repo, adapter, "")

	states, ready, err = portalloc.Prepare(adapter, lane)
	if err != nil {
		t.Fatalf("prepare pruned adapter: %v", err)
	}
	if !ready {
		t.Fatalf("expected prepare after service removal to allocate ports, got ready=%t", ready)
	}
	if states["worker"].Port != 3001 {
		t.Fatalf("expected surviving worker service to keep port 3001, got %#v", states["worker"])
	}

	allocations := readCatalogAllocations(t, sharedConfig)
	if len(allocations) != 1 {
		t.Fatalf("expected 1 catalog allocation after pruning removed service, got %d", len(allocations))
	}
	if port, ok := allocations["worker"]; !ok || port != 3001 {
		t.Fatalf("expected worker allocation on 3001 after pruning, got %#v", allocations)
	}
	if _, ok := allocations["web"]; ok {
		t.Fatalf("expected removed web service to be pruned from catalog, got %#v", allocations)
	}
}

func TestPreparePrunesAllocationsWhenPortsBlockIsRemoved(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	sharedConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", sharedConfig)

	adapter, lane := buildLane(t, repo, "")

	states, ready, err := portalloc.Prepare(adapter, lane)
	if err != nil {
		t.Fatalf("prepare initial adapter: %v", err)
	}
	if !ready {
		t.Fatalf("expected initial prepare to allocate ports, got ready=%t", ready)
	}
	if states["web"].Port != 3000 {
		t.Fatalf("expected web to allocate default port 3000, got %#v", states["web"])
	}

	adapter = rewritePortsBlock(t, repo, "")
	lane = buildLaneFromAdapter(t, repo, adapter, "")

	states, ready, err = portalloc.Prepare(adapter, lane)
	if err != nil {
		t.Fatalf("prepare adapter without ports: %v", err)
	}
	if !ready {
		t.Fatalf("expected prepare without ports to stay ready, got ready=%t", ready)
	}
	if len(states) != 0 {
		t.Fatalf("expected no port states after removing ports block, got %#v", states)
	}

	allocations := readCatalogAllocations(t, sharedConfig)
	if len(allocations) != 0 {
		t.Fatalf("expected all catalog allocations to be pruned after removing ports block, got %#v", allocations)
	}
}

func TestPreparePrunesRenamedServiceAllocations(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	sharedConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", sharedConfig)

	adapter := rewritePortsBlock(t, repo, `ports:
  - name: web
    default: 3000
  - name: worker
    default: 3001`)
	lane := buildLaneFromAdapter(t, repo, adapter, "")

	states, ready, err := portalloc.Prepare(adapter, lane)
	if err != nil {
		t.Fatalf("prepare initial adapter: %v", err)
	}
	if !ready {
		t.Fatalf("expected initial prepare to allocate ports, got ready=%t", ready)
	}
	if states["web"].Port != 3000 || states["worker"].Port != 3001 {
		t.Fatalf("expected initial sticky defaults, got %#v", states)
	}

	adapter = rewritePortsBlock(t, repo, `ports:
  - name: http
    default: 3000
  - name: worker
    default: 3001`)
	lane = buildLaneFromAdapter(t, repo, adapter, "")

	states, ready, err = portalloc.Prepare(adapter, lane)
	if err != nil {
		t.Fatalf("prepare renamed adapter: %v", err)
	}
	if !ready {
		t.Fatalf("expected prepare after rename to allocate ports, got ready=%t", ready)
	}
	if states["http"].Port != 3000 {
		t.Fatalf("expected renamed http service to reclaim port 3000, got %#v", states["http"])
	}
	if states["worker"].Port != 3001 {
		t.Fatalf("expected unaffected worker service to keep port 3001, got %#v", states["worker"])
	}

	allocations := readCatalogAllocations(t, sharedConfig)
	if len(allocations) != 2 {
		t.Fatalf("expected 2 catalog allocations after rename, got %d", len(allocations))
	}
	if port, ok := allocations["http"]; !ok || port != 3000 {
		t.Fatalf("expected http allocation on 3000 after rename, got %#v", allocations)
	}
	if port, ok := allocations["worker"]; !ok || port != 3001 {
		t.Fatalf("expected worker allocation on 3001 after rename, got %#v", allocations)
	}
	if _, ok := allocations["web"]; ok {
		t.Fatalf("expected orphaned web allocation to be pruned after rename, got %#v", allocations)
	}
}

func buildLane(t *testing.T, repo, mode string) (*config.AdapterConfig, portalloc.Lane) {
	t.Helper()

	configPath := filepath.Join(repo, "devlane.yaml")
	adapter, err := config.LoadAdapter(configPath)
	if err != nil {
		t.Fatalf("load adapter: %v", err)
	}

	laneManifest, err := manifest.Build(adapter, manifest.Options{
		CWD:        repo,
		ConfigPath: configPath,
		Mode:       mode,
	})
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}

	return adapter, portalloc.Lane{
		App:      adapter.App,
		RepoPath: laneManifest.Lane.RepoRoot,
		Name:     laneManifest.Lane.Name,
		Mode:     laneManifest.Lane.Mode,
		Branch:   laneManifest.Lane.Branch,
		Stable:   laneManifest.Lane.Stable,
	}
}

func rewritePortsBlock(t *testing.T, repo, portsBlock string) *config.AdapterConfig {
	t.Helper()

	configPath := filepath.Join(repo, "devlane.yaml")
	payload, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read adapter: %v", err)
	}

	content := string(payload)
	start := strings.Index(content, "\nports:\n")
	if start == -1 {
		t.Fatal("expected adapter to contain ports block")
	}
	start++
	end := strings.Index(content[start:], "\noutputs:\n")
	if end == -1 {
		t.Fatal("expected adapter to contain outputs block")
	}
	end += start

	updated := content[:start] + portsBlock + "\n" + content[end:]
	if err := os.WriteFile(configPath, []byte(updated), 0o644); err != nil {
		t.Fatalf("write adapter: %v", err)
	}

	adapter, err := config.LoadAdapter(configPath)
	if err != nil {
		t.Fatalf("load adapter: %v", err)
	}
	return adapter
}

func readCatalogAllocations(t *testing.T, configHome string) map[string]int {
	t.Helper()

	payload, err := os.ReadFile(filepath.Join(configHome, "devlane", "catalog.json"))
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}

	var stored struct {
		Allocations []struct {
			Service string `json:"service"`
			Port    int    `json:"port"`
		} `json:"allocations"`
	}
	if err := json.Unmarshal(payload, &stored); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}

	allocations := make(map[string]int, len(stored.Allocations))
	for _, row := range stored.Allocations {
		allocations[row.Service] = row.Port
	}
	return allocations
}
