package manifest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/auro/devlane/internal/config"
	"github.com/auro/devlane/internal/manifest"
	"github.com/auro/devlane/internal/testutil"
)

func TestManifestUsesBranchAsDevLane(t *testing.T) {
	repo := testutil.InitDemoRepo(t)

	adapter, err := config.LoadAdapter(filepath.Join(repo, "devlane.yaml"))
	if err != nil {
		t.Fatalf("LoadAdapter returned error: %v", err)
	}

	laneManifest, err := manifest.Build(adapter, manifest.Options{
		CWD:        repo,
		ConfigPath: filepath.Join(repo, "devlane.yaml"),
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if laneManifest.Ready {
		t.Fatal("expected manifest.Ready to be false before allocation support exists")
	}
	if laneManifest.Lane.Mode != "dev" {
		t.Fatalf("unexpected mode: %q", laneManifest.Lane.Mode)
	}
	if laneManifest.Lane.Slug != "feature-test-lane" {
		t.Fatalf("unexpected lane slug: %q", laneManifest.Lane.Slug)
	}
	if laneManifest.Network.ProjectName != "demoapp_feature-test-lane" {
		t.Fatalf("unexpected project name: %q", laneManifest.Network.ProjectName)
	}
	if laneManifest.Network.PublicHost == nil || *laneManifest.Network.PublicHost != "feature-test-lane.demoapp.localhost" {
		t.Fatalf("unexpected public host: %#v", laneManifest.Network.PublicHost)
	}
	if laneManifest.Ports["web"].Port != 3000 {
		t.Fatalf("unexpected port: %d", laneManifest.Ports["web"].Port)
	}
	if laneManifest.Ports["web"].Allocated {
		t.Fatal("expected web port to be unallocated")
	}
}

func TestManifestCanForceStableMode(t *testing.T) {
	repo := testutil.InitDemoRepo(t)

	adapter, err := config.LoadAdapter(filepath.Join(repo, "devlane.yaml"))
	if err != nil {
		t.Fatalf("LoadAdapter returned error: %v", err)
	}

	laneManifest, err := manifest.Build(adapter, manifest.Options{
		CWD:        repo,
		ConfigPath: filepath.Join(repo, "devlane.yaml"),
		Mode:       "stable",
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if laneManifest.Lane.Mode != "stable" {
		t.Fatalf("unexpected mode: %q", laneManifest.Lane.Mode)
	}
	if !laneManifest.Lane.Stable {
		t.Fatal("expected stable lane")
	}
	if laneManifest.Lane.Name != "stable" {
		t.Fatalf("unexpected lane name: %q", laneManifest.Lane.Name)
	}
	if laneManifest.Network.PublicHost == nil || *laneManifest.Network.PublicHost != "demoapp.localhost" {
		t.Fatalf("unexpected public host: %#v", laneManifest.Network.PublicHost)
	}
}

func TestManifestOmitsComposeEnvWithoutCompose(t *testing.T) {
	repo := t.TempDir()
	configPath := filepath.Join(repo, "devlane.yaml")
	if err := os.WriteFile(configPath, []byte(`
schema: 1
app: baremetal
kind: web
lane:
  stable_name: stable
  stable_branches: [main]
  project_pattern: "{app}_{lane}"
  path_roots:
    state: .devlane/state
    cache: .devlane/cache
    runtime: .devlane/runtime
runtime:
  env: {}
outputs:
  manifest_path: .devlane/manifest.json
  generated: []
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	adapter, err := config.LoadAdapter(configPath)
	if err != nil {
		t.Fatalf("LoadAdapter returned error: %v", err)
	}

	laneManifest, err := manifest.Build(adapter, manifest.Options{
		CWD:        repo,
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if laneManifest.Paths.ComposeEnv != nil {
		t.Fatalf("expected compose env to be omitted, got %q", *laneManifest.Paths.ComposeEnv)
	}
	if !laneManifest.Ready {
		t.Fatal("expected manifest to be ready when no ports are declared")
	}
}
