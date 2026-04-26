package portalloc_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/auro/devlane/internal/portalloc"
)

func TestListReturnsEmptyWhenCatalogFileMissing(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	rows, err := portalloc.List()
	if err != nil {
		t.Fatalf("List on missing catalog: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected empty result for missing catalog, got %d rows", len(rows))
	}
}

func TestListReturnsHydratedRowsAndBackfillsLegacyMetadata(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	catalogPath := filepath.Join(configHome, "devlane", "catalog.json")
	if err := os.MkdirAll(filepath.Dir(catalogPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(catalogPath, []byte(`{
  "schema": 1,
  "allocations": [
    {
      "app": "agentchat",
      "lane": "feature-x",
      "mode": "dev",
      "branch": "feature-x",
      "service": "web",
      "port": 3100,
      "repoPath": "/home/auro/code/agentchat-feature-x",
      "lastPrepared": "2026-04-11T14:30:00Z"
    },
    {
      "app": "legacyapp",
      "lane": "stable",
      "service": "api",
      "port": 4000,
      "repoPath": "/home/auro/code/legacyapp",
      "lastPrepared": "2026-04-10T10:00:00Z"
    }
  ]
}
`), 0o644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}

	rows, err := portalloc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d (%#v)", len(rows), rows)
	}

	byApp := make(map[string]portalloc.Allocation, len(rows))
	for _, row := range rows {
		byApp[row.App] = row
	}

	got, ok := byApp["agentchat"]
	if !ok {
		t.Fatalf("expected agentchat row, got %#v", rows)
	}
	want := portalloc.Allocation{
		App:          "agentchat",
		Lane:         "feature-x",
		Mode:         "dev",
		Branch:       "feature-x",
		Service:      "web",
		Port:         3100,
		RepoPath:     "/home/auro/code/agentchat-feature-x",
		LastPrepared: "2026-04-11T14:30:00Z",
	}
	if got != want {
		t.Fatalf("agentchat row mismatch:\n got  %#v\n want %#v", got, want)
	}

	legacy, ok := byApp["legacyapp"]
	if !ok {
		t.Fatalf("expected legacyapp row, got %#v", rows)
	}
	if legacy.Mode != "stable" {
		t.Fatalf("expected legacy mode backfilled to stable from lane name, got %q", legacy.Mode)
	}
	if legacy.Branch != "stable" {
		t.Fatalf("expected legacy branch backfilled from lane name, got %q", legacy.Branch)
	}
}

func TestListReturnsCopyCallersCannotMutateCatalog(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	catalogPath := filepath.Join(configHome, "devlane", "catalog.json")
	if err := os.MkdirAll(filepath.Dir(catalogPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(catalogPath, []byte(`{
  "schema": 1,
  "allocations": [
    {"app": "a", "lane": "l", "mode": "dev", "branch": "l", "service": "web", "port": 3000, "repoPath": "/r", "lastPrepared": "t"}
  ]
}
`), 0o644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}

	first, err := portalloc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	first[0].Port = 9999

	second, err := portalloc.List()
	if err != nil {
		t.Fatalf("second List: %v", err)
	}
	if second[0].Port != 3000 {
		t.Fatalf("expected second List to be unaffected by caller mutation, got port %d", second[0].Port)
	}
}

func TestListErrorOnMalformedCatalogIncludesPath(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	catalogPath := filepath.Join(configHome, "devlane", "catalog.json")
	if err := os.MkdirAll(filepath.Dir(catalogPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(catalogPath, []byte("not json {{"), 0o644); err != nil {
		t.Fatalf("write malformed catalog: %v", err)
	}

	_, err := portalloc.List()
	if err == nil {
		t.Fatal("expected error for malformed catalog")
	}
	if !strings.Contains(err.Error(), catalogPath) {
		t.Fatalf("expected error to name catalog path %q, got %v", catalogPath, err)
	}
}

func TestListErrorOnUnsupportedSchemaIncludesPath(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	catalogPath := filepath.Join(configHome, "devlane", "catalog.json")
	if err := os.MkdirAll(filepath.Dir(catalogPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(catalogPath, []byte(`{"schema": 99, "allocations": []}`), 0o644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}

	_, err := portalloc.List()
	if err == nil {
		t.Fatal("expected error for unsupported schema")
	}
	if !strings.Contains(err.Error(), catalogPath) {
		t.Fatalf("expected error to name catalog path %q, got %v", catalogPath, err)
	}
}

func TestListNeverObservesPartialWriteDuringConcurrentRename(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	catalogDir := filepath.Join(configHome, "devlane")
	if err := os.MkdirAll(catalogDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	catalogPath := filepath.Join(catalogDir, "catalog.json")

	if err := publishCatalogAtomically(catalogDir, catalogPath, 1); err != nil {
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
			if err := publishCatalogAtomically(catalogDir, catalogPath, 1+(i%5)); err != nil {
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
		rows, err := portalloc.List()
		if err != nil {
			t.Fatalf("List during concurrent rename: %v", err)
		}
		if len(rows) < 1 || len(rows) > 5 {
			t.Fatalf("List returned out-of-range row count %d during concurrent rename", len(rows))
		}
		reads++
	}
	if reads == 0 {
		t.Fatal("expected at least one List call within the read window")
	}
}

func publishCatalogAtomically(dir, finalPath string, rowCount int) error {
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
		return fmt.Errorf("rename temp catalog: %w", err)
	}
	return nil
}
