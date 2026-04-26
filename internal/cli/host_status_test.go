package cli_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHostStatusEmptyCatalog(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	code, stdout, stderr := runCLI(t, []string{"host", "status"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstderr:\n%s", code, stderr)
	}
	if strings.TrimSpace(stdout) != "no allocations" {
		t.Fatalf("expected \"no allocations\", got:\n%s", stdout)
	}
}

func TestHostStatusPopulatedCatalogPrintsHeaderAndRows(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "agentchat", Lane: "feature-x", Mode: "dev", Service: "web", Port: 3100, RepoPath: "/home/auro/code/agentchat-feature-x"},
		{App: "legacyapp", Lane: "stable", Mode: "stable", Service: "api", Port: 4000, RepoPath: "/home/auro/code/legacyapp"},
	})

	code, stdout, stderr := runCLI(t, []string{"host", "status"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstderr:\n%s", code, stderr)
	}

	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected header + 2 rows = 3 lines, got %d:\n%s", len(lines), stdout)
	}

	header := lines[0]
	for _, col := range []string{"APP", "MODE", "LANE", "SERVICE", "PORT", "REPO PATH"} {
		if !strings.Contains(header, col) {
			t.Fatalf("header missing column %q:\n%s", col, header)
		}
	}

	if appIdx, modeIdx := strings.Index(header, "APP"), strings.Index(header, "MODE"); !(appIdx < modeIdx) {
		t.Fatalf("expected APP before MODE in header: %q", header)
	}
	if modeIdx, laneIdx := strings.Index(header, "MODE"), strings.Index(header, "LANE"); !(modeIdx < laneIdx) {
		t.Fatalf("expected MODE before LANE in header: %q", header)
	}
	if laneIdx, svcIdx := strings.Index(header, "LANE"), strings.Index(header, "SERVICE"); !(laneIdx < svcIdx) {
		t.Fatalf("expected LANE before SERVICE in header: %q", header)
	}
	if svcIdx, portIdx := strings.Index(header, "SERVICE"), strings.Index(header, "PORT"); !(svcIdx < portIdx) {
		t.Fatalf("expected SERVICE before PORT in header: %q", header)
	}
	if portIdx, repoIdx := strings.Index(header, "PORT"), strings.Index(header, "REPO PATH"); !(portIdx < repoIdx) {
		t.Fatalf("expected PORT before REPO PATH in header: %q", header)
	}

	for _, want := range []string{
		"agentchat", "feature-x", "dev", "web", "3100", "/home/auro/code/agentchat-feature-x",
		"legacyapp", "stable", "api", "4000", "/home/auro/code/legacyapp",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected output to contain %q:\n%s", want, stdout)
		}
	}
}

func TestHostStatusSortsByAppRepoPathService(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	seedCatalog(t, configHome, []hostStatusSeedRow{
		{App: "zeta", Lane: "dev", Mode: "dev", Service: "web", Port: 3001, RepoPath: "/repo/zeta"},
		{App: "alpha", Lane: "dev", Mode: "dev", Service: "web", Port: 3002, RepoPath: "/repo/alpha-b"},
		{App: "alpha", Lane: "dev", Mode: "dev", Service: "api", Port: 3003, RepoPath: "/repo/alpha-a"},
		{App: "alpha", Lane: "dev", Mode: "dev", Service: "web", Port: 3004, RepoPath: "/repo/alpha-a"},
	})

	code, stdout, stderr := runCLI(t, []string{"host", "status"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstderr:\n%s", code, stderr)
	}

	rows := dataLines(stdout)
	if len(rows) != 4 {
		t.Fatalf("expected 4 data rows, got %d:\n%s", len(rows), stdout)
	}

	wantOrder := []struct{ repoPath, service string }{
		{"/repo/alpha-a", "api"},
		{"/repo/alpha-a", "web"},
		{"/repo/alpha-b", "web"},
		{"/repo/zeta", "web"},
	}
	for i, want := range wantOrder {
		if !strings.Contains(rows[i], want.repoPath) || !strings.Contains(rows[i], want.service) {
			t.Fatalf("row %d: expected repoPath=%q service=%q, got:\n%s", i, want.repoPath, want.service, rows[i])
		}
	}
}

func TestHostStatusNeverObservesPartialWriteDuringConcurrentRename(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	catalogDir := filepath.Join(configHome, "devlane")
	if err := os.MkdirAll(catalogDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	catalogPath := filepath.Join(catalogDir, "catalog.json")

	if err := publishHostStatusCatalog(catalogDir, catalogPath, 1); err != nil {
		t.Fatalf("seed catalog: %v", err)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	var stop atomic.Bool
	writerErr := make(chan error, 1)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		i := 1
		for !stop.Load() {
			if err := publishHostStatusCatalog(catalogDir, catalogPath, 1+(i%5)); err != nil {
				writerErr <- err
				return
			}
			i++
		}
	}()
	defer func() {
		stop.Store(true)
		<-writerDone
		select {
		case err := <-writerErr:
			t.Fatalf("writer goroutine: %v", err)
		default:
		}
	}()

	reads := 0
	for time.Now().Before(deadline) {
		code, stdout, stderr := runCLI(t, []string{"host", "status"})
		if code != 0 {
			t.Fatalf("host status exit %d during concurrent rename\nstderr:\n%s", code, stderr)
		}
		assertHostStatusSnapshotIsConsistent(t, stdout)
		reads++
	}
	if reads == 0 {
		t.Fatal("expected at least one host status invocation within the read window")
	}
}

type hostStatusSeedRow struct {
	App      string
	Lane     string
	Mode     string
	Branch   string
	Service  string
	Port     int
	RepoPath string
}

func seedCatalog(t *testing.T, configHome string, rows []hostStatusSeedRow) {
	t.Helper()

	catalogDir := filepath.Join(configHome, "devlane")
	if err := os.MkdirAll(catalogDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	allocations := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		allocations = append(allocations, map[string]any{
			"app":          r.App,
			"lane":         r.Lane,
			"mode":         r.Mode,
			"branch":       r.Branch,
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

func publishHostStatusCatalog(dir, finalPath string, rowCount int) error {
	rows := make([]map[string]any, 0, rowCount)
	for i := 0; i < rowCount; i++ {
		rows = append(rows, map[string]any{
			"app":          fmt.Sprintf("app-%d", i),
			"lane":         "dev",
			"mode":         "dev",
			"branch":       "main",
			"service":      "web",
			"port":         3000 + i,
			"repoPath":     fmt.Sprintf("/repo/%d", i),
			"lastPrepared": "2026-04-25T00:00:00Z",
		})
	}
	payload, err := json.Marshal(map[string]any{
		"schema":      1,
		"allocations": rows,
	})
	if err != nil {
		return fmt.Errorf("marshal catalog: %w", err)
	}
	temp, err := os.CreateTemp(dir, "catalog-*.json")
	if err != nil {
		return fmt.Errorf("create temp catalog: %w", err)
	}
	tempPath := temp.Name()
	if _, err := temp.Write(payload); err != nil {
		temp.Close()
		os.Remove(tempPath)
		return fmt.Errorf("write temp catalog: %w", err)
	}
	if err := temp.Close(); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("close temp catalog: %w", err)
	}
	if err := os.Rename(tempPath, finalPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("rename catalog: %w", err)
	}
	return nil
}

func assertHostStatusSnapshotIsConsistent(t *testing.T, stdout string) {
	t.Helper()

	rows := dataLines(stdout)
	if len(rows) < 1 || len(rows) > 5 {
		t.Fatalf("host status returned out-of-range row count %d during concurrent rename:\n%s", len(rows), stdout)
	}

	for i, line := range rows {
		wantApp := fmt.Sprintf("app-%d", i)
		wantPort := fmt.Sprintf("%d", 3000+i)
		wantRepo := fmt.Sprintf("/repo/%d", i)
		if !strings.Contains(line, wantApp) || !strings.Contains(line, wantPort) || !strings.Contains(line, wantRepo) {
			t.Fatalf("row %d does not match any published snapshot tuple (app=%s port=%s repoPath=%s):\n%s",
				i, wantApp, wantPort, wantRepo, line)
		}
	}
}

func dataLines(stdout string) []string {
	all := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(all) <= 1 {
		return nil
	}
	return all[1:]
}
