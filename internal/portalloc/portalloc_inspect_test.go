package portalloc_test

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/auro/devlane/internal/config"
	"github.com/auro/devlane/internal/portalloc"
	"github.com/auro/devlane/internal/testutil"
)

func TestInspectMatchesPrepareWhenDefaultCandidateIsBound(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	sharedConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", sharedConfig)

	listener, boundPort := bindTestPort(t)
	defer listener.Close()

	adapter := rewriteDefaultPort(t, repo, boundPort)
	lane := buildLaneFromAdapter(t, repo, adapter, "")

	inspectStates, inspectReady, err := portalloc.Inspect(adapter, lane)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if inspectReady {
		t.Fatalf("expected inspect to remain unready before prepare")
	}
	if inspectStates["web"].Allocated {
		t.Fatalf("expected provisional allocation, got %#v", inspectStates["web"])
	}
	if inspectStates["web"].Port == boundPort {
		t.Fatalf("inspect should skip bound provisional port %d", boundPort)
	}

	prepareStates, prepareReady, err := portalloc.Prepare(adapter, lane)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !prepareReady {
		t.Fatalf("expected prepare to commit allocation, got ready=%t", prepareReady)
	}
	if !prepareStates["web"].Allocated {
		t.Fatalf("expected committed allocation, got %#v", prepareStates["web"])
	}
	if prepareStates["web"].Port != inspectStates["web"].Port {
		t.Fatalf("inspect/prepare mismatch: inspect=%d prepare=%d", inspectStates["web"].Port, prepareStates["web"].Port)
	}
}

func TestInspectFailsWhenStableFixtureIsBound(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	sharedConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", sharedConfig)

	listener, boundPort := bindTestPort(t)
	defer listener.Close()

	adapter := rewriteDefaultPort(t, repo, boundPort)
	lane := buildLaneFromAdapter(t, repo, adapter, "stable")

	if _, _, err := portalloc.Inspect(adapter, lane); err == nil {
		t.Fatal("expected inspect to fail for a bound stable fixture")
	} else if !strings.Contains(err.Error(), "stable port") {
		t.Fatalf("unexpected inspect error: %v", err)
	}

	if _, _, err := portalloc.Prepare(adapter, lane); err == nil {
		t.Fatal("expected prepare to fail for a bound stable fixture")
	} else if !strings.Contains(err.Error(), "stable port") {
		t.Fatalf("unexpected prepare error: %v", err)
	}
}

func TestStableInspectDoesNotReuseSameCheckoutDevAllocation(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	sharedConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", sharedConfig)

	adapter := rewriteStableFixture(t, repo, 3001, 3000)

	devLane := buildLaneFromAdapter(t, repo, adapter, "")
	devStates, devReady, err := portalloc.Prepare(adapter, devLane)
	if err != nil {
		t.Fatalf("prepare dev: %v", err)
	}
	if !devReady || devStates["web"].Port != 3001 {
		t.Fatalf("expected dev allocation on 3001, got ready=%t state=%#v", devReady, devStates["web"])
	}

	stableLane := buildLaneFromAdapter(t, repo, adapter, "stable")
	stableStates, stableReady, err := portalloc.Inspect(adapter, stableLane)
	if err != nil {
		t.Fatalf("inspect stable: %v", err)
	}
	if stableReady {
		t.Fatalf("expected stable inspect to remain provisional before prepare")
	}
	if stableStates["web"].Allocated {
		t.Fatalf("expected stable inspect to report provisional fixture, got %#v", stableStates["web"])
	}
	if stableStates["web"].Port != 3000 {
		t.Fatalf("expected stable inspect fixture port 3000, got %#v", stableStates["web"])
	}

	stableStates, stableReady, err = portalloc.Prepare(adapter, stableLane)
	if err != nil {
		t.Fatalf("prepare stable: %v", err)
	}
	if !stableReady || !stableStates["web"].Allocated {
		t.Fatalf("expected committed stable allocation, got ready=%t state=%#v", stableReady, stableStates["web"])
	}
	if stableStates["web"].Port != 3000 {
		t.Fatalf("expected stable prepare to move allocation to fixture 3000, got %#v", stableStates["web"])
	}

	port := readCatalogPort(t, sharedConfig)
	if port != 3000 {
		t.Fatalf("expected single catalog row to move to fixture 3000, got %d", port)
	}
}

func TestStablePrepareFailsWhenFixtureIsUnavailableAfterSameCheckoutDevPrepare(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	sharedConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", sharedConfig)

	adapter := rewriteStableFixture(t, repo, 3001, 3000)

	devLane := buildLaneFromAdapter(t, repo, adapter, "")
	if _, _, err := portalloc.Prepare(adapter, devLane); err != nil {
		t.Fatalf("prepare dev: %v", err)
	}

	listener, err := net.Listen("tcp4", "127.0.0.1:3000")
	if err != nil {
		t.Fatalf("listen on stable fixture: %v", err)
	}
	defer listener.Close()

	stableLane := buildLaneFromAdapter(t, repo, adapter, "stable")
	if _, _, err := portalloc.Inspect(adapter, stableLane); err == nil {
		t.Fatal("expected inspect to fail when the stable fixture is unavailable")
	} else if !strings.Contains(err.Error(), "stable port 3000") {
		t.Fatalf("unexpected inspect error: %v", err)
	}

	if _, _, err := portalloc.Prepare(adapter, stableLane); err == nil {
		t.Fatal("expected prepare to fail when the stable fixture is unavailable")
	} else if !strings.Contains(err.Error(), "stable port 3000") {
		t.Fatalf("unexpected prepare error: %v", err)
	}

	port := readCatalogPort(t, sharedConfig)
	if port != 3001 {
		t.Fatalf("expected failed stable prepare to leave existing dev allocation on 3001, got %d", port)
	}
}

func bindTestPort(t *testing.T) (net.Listener, int) {
	t.Helper()

	listener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	return listener, listener.Addr().(*net.TCPAddr).Port
}

func rewriteDefaultPort(t *testing.T, repo string, port int) *config.AdapterConfig {
	t.Helper()

	configPath := filepath.Join(repo, "devlane.yaml")
	payload, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read adapter: %v", err)
	}

	updated := strings.Replace(string(payload), "default: 3000", fmt.Sprintf("default: %d", port), 1)
	if updated == string(payload) {
		t.Fatal("expected to rewrite default port")
	}
	if err := os.WriteFile(configPath, []byte(updated), 0o644); err != nil {
		t.Fatalf("write adapter: %v", err)
	}

	adapter, err := config.LoadAdapter(configPath)
	if err != nil {
		t.Fatalf("load adapter: %v", err)
	}
	return adapter
}

func rewriteStableFixture(t *testing.T, repo string, defaultPort, stablePort int) *config.AdapterConfig {
	t.Helper()

	configPath := filepath.Join(repo, "devlane.yaml")
	payload, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read adapter: %v", err)
	}

	replacement := fmt.Sprintf("default: %d\n    stable_port: %d", defaultPort, stablePort)
	updated := strings.Replace(string(payload), "default: 3000", replacement, 1)
	if updated == string(payload) {
		t.Fatal("expected to rewrite web port fixture")
	}
	if err := os.WriteFile(configPath, []byte(updated), 0o644); err != nil {
		t.Fatalf("write adapter: %v", err)
	}

	adapter, err := config.LoadAdapter(configPath)
	if err != nil {
		t.Fatalf("load adapter: %v", err)
	}
	return adapter
}

func readCatalogPort(t *testing.T, configHome string) int {
	t.Helper()

	payload, err := os.ReadFile(filepath.Join(configHome, "devlane", "catalog.json"))
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}

	var stored struct {
		Allocations []struct {
			Port int `json:"port"`
		} `json:"allocations"`
	}
	if err := json.Unmarshal(payload, &stored); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	if len(stored.Allocations) != 1 {
		t.Fatalf("expected exactly one allocation, got %d", len(stored.Allocations))
	}
	return stored.Allocations[0].Port
}

func buildLaneFromAdapter(t *testing.T, repo string, adapter *config.AdapterConfig, mode string) portalloc.Lane {
	t.Helper()

	stable := mode == "stable"
	laneName := "feature/test-lane"
	laneMode := "dev"
	if stable {
		laneName = adapter.Lane.StableName
		laneMode = "stable"
	}
	return portalloc.Lane{
		App:      adapter.App,
		RepoPath: repo,
		Name:     laneName,
		Mode:     laneMode,
		Branch:   "feature/test-lane",
		Stable:   stable,
	}
}
