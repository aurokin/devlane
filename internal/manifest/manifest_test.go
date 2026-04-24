package manifest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/auro/devlane/internal/config"
	"github.com/auro/devlane/internal/manifest"
	"github.com/auro/devlane/internal/testutil"
	"github.com/auro/devlane/internal/util"
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
	if laneManifest.Ready {
		t.Fatal("expected inspect manifest to report unallocated ports before prepare")
	}
	if laneManifest.Ports["web"].Port != 3000 {
		t.Fatalf("unexpected provisional web port: %#v", laneManifest.Ports["web"])
	}
	if laneManifest.Ports["web"].Allocated {
		t.Fatalf("expected provisional web port to be unallocated: %#v", laneManifest.Ports["web"])
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
	if laneManifest.Ports["web"].Port != 3000 {
		t.Fatalf("unexpected stable fixture port: %#v", laneManifest.Ports["web"])
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
}

func TestManifestResolvesRelativePathsFromAdapterRoot(t *testing.T) {
	repo := t.TempDir()
	appDir := filepath.Join(repo, "apps", "web")
	configPath := filepath.Join(appDir, "devlane.yaml")

	if err := os.MkdirAll(filepath.Join(appDir, "templates"), 0o755); err != nil {
		t.Fatalf("mkdir app dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "templates", "app.env.tmpl"), []byte("APP={{app}}\n"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`
schema: 1
app: nested
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
  compose_files:
    - compose.yaml
outputs:
  manifest_path: .devlane/manifest.json
  compose_env_path: .devlane/compose.env
  generated:
    - template: templates/app.env.tmpl
      destination: .devlane/generated/app.env
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	adapter, err := config.LoadAdapter(configPath)
	if err != nil {
		t.Fatalf("LoadAdapter returned error: %v", err)
	}

	laneManifest, err := manifest.Build(adapter, manifest.Options{
		CWD:        appDir,
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if laneManifest.Paths.Manifest != filepath.Join(appDir, ".devlane", "manifest.json") {
		t.Fatalf("unexpected manifest path: %s", laneManifest.Paths.Manifest)
	}
	if laneManifest.Compose.Files[0] != filepath.Join(appDir, "compose.yaml") {
		t.Fatalf("unexpected compose path: %s", laneManifest.Compose.Files[0])
	}
	if laneManifest.Outputs.Generated[0].Template != filepath.Join(appDir, "templates", "app.env.tmpl") {
		t.Fatalf("unexpected template path: %s", laneManifest.Outputs.Generated[0].Template)
	}
}

func TestManifestRejectsPathsOutsideRepoRoot(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	configPath := filepath.Join(repo, "devlane.yaml")
	if err := os.WriteFile(configPath, []byte(`
schema: 1
app: demoapp
kind: web
lane:
  stable_name: stable
  stable_branches: [main, master]
  project_pattern: "{app}_{lane}"
  path_roots:
    state: .devlane/state
    cache: .devlane/cache
    runtime: .devlane/runtime
runtime:
  env: {}
outputs:
  manifest_path: ../outside.json
  generated: []
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	adapter, err := config.LoadAdapter(configPath)
	if err != nil {
		t.Fatalf("LoadAdapter returned error: %v", err)
	}

	_, err = manifest.Build(adapter, manifest.Options{
		CWD:        repo,
		ConfigPath: configPath,
	})
	if err == nil {
		t.Fatal("expected Build to reject paths outside repo root")
	}
	if !strings.Contains(err.Error(), "repo root") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestManifestRejectsSymlinkEscapingPaths(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	outside := filepath.Join(root, "outside")
	configPath := filepath.Join(repo, "devlane.yaml")

	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, "linked")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`
schema: 1
app: demoapp
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
  manifest_path: linked/manifest.json
  generated: []
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	adapter, err := config.LoadAdapter(configPath)
	if err != nil {
		t.Fatalf("LoadAdapter returned error: %v", err)
	}

	_, err = manifest.Build(adapter, manifest.Options{
		CWD:        repo,
		ConfigPath: configPath,
	})
	if err == nil {
		t.Fatal("expected Build to reject symlink escape")
	}
	if !strings.Contains(err.Error(), "repo root") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestManifestUsesConfigDirectoryAsRepoRootOutsideGit(t *testing.T) {
	repo := t.TempDir()
	appDir := filepath.Join(repo, "apps", "web")
	nestedCWD := filepath.Join(appDir, "subdir", "child")
	configPath := filepath.Join(appDir, "devlane.yaml")

	if err := os.MkdirAll(filepath.Join(appDir, "templates"), 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	if err := os.MkdirAll(nestedCWD, 0o755); err != nil {
		t.Fatalf("mkdir nested cwd: %v", err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "templates", "app.env.tmpl"), []byte("APP={{app}}\n"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`
schema: 1
app: nested
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
  compose_files:
    - compose.yaml
outputs:
  manifest_path: .devlane/manifest.json
  compose_env_path: .devlane/compose.env
  generated:
    - template: templates/app.env.tmpl
      destination: .devlane/generated/app.env
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	adapter, err := config.LoadAdapter(configPath)
	if err != nil {
		t.Fatalf("LoadAdapter returned error: %v", err)
	}

	laneManifest, err := manifest.Build(adapter, manifest.Options{
		CWD:        nestedCWD,
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if laneManifest.Lane.RepoRoot != appDir {
		t.Fatalf("expected config directory as repo root, got %s", laneManifest.Lane.RepoRoot)
	}
	if laneManifest.Paths.Manifest != filepath.Join(appDir, ".devlane", "manifest.json") {
		t.Fatalf("unexpected manifest path: %s", laneManifest.Paths.Manifest)
	}
	if laneManifest.Compose.Files[0] != filepath.Join(appDir, "compose.yaml") {
		t.Fatalf("unexpected compose path: %s", laneManifest.Compose.Files[0])
	}
	if laneManifest.Outputs.Generated[0].Destination != filepath.Join(appDir, ".devlane", "generated", "app.env") {
		t.Fatalf("unexpected generated destination: %s", laneManifest.Outputs.Generated[0].Destination)
	}
}

func TestManifestDoesNotDeriveRepoRootFromSymlinkedSubdirectoryCWD(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	cwdTarget := filepath.Join(repo, "apps", "web")
	if err := os.MkdirAll(cwdTarget, 0o755); err != nil {
		t.Fatalf("mkdir cwd target: %v", err)
	}
	symlinkCWD := filepath.Join(t.TempDir(), "web-link")
	if err := os.Symlink(cwdTarget, symlinkCWD); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	configPath := filepath.Join(repo, "devlane.yaml")
	adapter, err := config.LoadAdapter(configPath)
	if err != nil {
		t.Fatalf("LoadAdapter returned error: %v", err)
	}

	laneManifest, err := manifest.Build(adapter, manifest.Options{
		CWD:        symlinkCWD,
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	canonicalRepo, err := util.CanonicalPath(repo)
	if err != nil {
		t.Fatalf("canonicalize repo: %v", err)
	}
	if laneManifest.Lane.RepoRoot != canonicalRepo {
		t.Fatalf("expected git repo root %s, got %s", canonicalRepo, laneManifest.Lane.RepoRoot)
	}
	if laneManifest.Paths.Manifest != filepath.Join(canonicalRepo, ".devlane", "manifest.json") {
		t.Fatalf("expected manifest path anchored at repo root, got %s", laneManifest.Paths.Manifest)
	}
}

func TestManifestPreservesCanonicalRepoRootFromSymlinkedCheckoutCWD(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir nested cwd target: %v", err)
	}
	symlinkRoot := filepath.Join(t.TempDir(), "repo-link")
	if err := os.Symlink(repo, symlinkRoot); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	configPath := filepath.Join(symlinkRoot, "devlane.yaml")
	adapter, err := config.LoadAdapter(configPath)
	if err != nil {
		t.Fatalf("LoadAdapter returned error: %v", err)
	}

	laneManifest, err := manifest.Build(adapter, manifest.Options{
		CWD:        filepath.Join(symlinkRoot, "nested"),
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	canonicalRepo, err := util.CanonicalPath(repo)
	if err != nil {
		t.Fatalf("canonicalize repo: %v", err)
	}
	if laneManifest.Lane.RepoRoot != canonicalRepo {
		t.Fatalf("expected canonical repo root %s, got %s", canonicalRepo, laneManifest.Lane.RepoRoot)
	}
	if laneManifest.Lane.ConfigPath != filepath.Join(canonicalRepo, "devlane.yaml") {
		t.Fatalf("expected config path anchored at canonical repo root, got %s", laneManifest.Lane.ConfigPath)
	}
	if laneManifest.Paths.Manifest != filepath.Join(canonicalRepo, ".devlane", "manifest.json") {
		t.Fatalf("expected manifest path anchored at canonical repo root, got %s", laneManifest.Paths.Manifest)
	}
}

func TestManifestUsesAdapterRootForDetachedLaneIdentityOutsideGit(t *testing.T) {
	repo := t.TempDir()
	appDir := filepath.Join(repo, "apps", "web")
	firstCWD := filepath.Join(appDir, "subdir", "child")
	secondCWD := filepath.Join(appDir, "other", "leaf")
	configPath := filepath.Join(appDir, "devlane.yaml")

	if err := os.MkdirAll(filepath.Join(appDir, "templates"), 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	for _, dir := range []string{firstCWD, secondCWD} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir nested cwd %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(appDir, "templates", "app.env.tmpl"), []byte("APP={{app}}\n"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`
schema: 1
app: nested
kind: web
lane:
  stable_name: stable
  stable_branches: [main]
  project_pattern: "{app}_{lane}"
  path_roots:
    state: .devlane/state
    cache: .devlane/cache
    runtime: .devlane/runtime
outputs:
  manifest_path: .devlane/manifest.json
  generated:
    - template: templates/app.env.tmpl
      destination: .devlane/generated/app.env
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	adapter, err := config.LoadAdapter(configPath)
	if err != nil {
		t.Fatalf("LoadAdapter returned error: %v", err)
	}

	firstManifest, err := manifest.Build(adapter, manifest.Options{
		CWD:        firstCWD,
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("Build first manifest: %v", err)
	}

	secondManifest, err := manifest.Build(adapter, manifest.Options{
		CWD:        secondCWD,
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("Build second manifest: %v", err)
	}

	if firstManifest.Lane.Name != "web" {
		t.Fatalf("expected adapter-root lane name, got %q", firstManifest.Lane.Name)
	}
	if secondManifest.Lane.Name != firstManifest.Lane.Name {
		t.Fatalf("expected stable detached lane name, got %q and %q", firstManifest.Lane.Name, secondManifest.Lane.Name)
	}
	if secondManifest.Lane.Slug != firstManifest.Lane.Slug {
		t.Fatalf("expected stable detached lane slug, got %q and %q", firstManifest.Lane.Slug, secondManifest.Lane.Slug)
	}
	if secondManifest.Network.ProjectName != firstManifest.Network.ProjectName {
		t.Fatalf("expected stable project name, got %q and %q", firstManifest.Network.ProjectName, secondManifest.Network.ProjectName)
	}
	if secondManifest.Paths.Manifest != firstManifest.Paths.Manifest {
		t.Fatalf("expected stable manifest path, got %q and %q", firstManifest.Paths.Manifest, secondManifest.Paths.Manifest)
	}
	if secondManifest.Paths.StateRoot != firstManifest.Paths.StateRoot {
		t.Fatalf("expected stable state root, got %q and %q", firstManifest.Paths.StateRoot, secondManifest.Paths.StateRoot)
	}
	if secondManifest.Paths.CacheRoot != firstManifest.Paths.CacheRoot {
		t.Fatalf("expected stable cache root, got %q and %q", firstManifest.Paths.CacheRoot, secondManifest.Paths.CacheRoot)
	}
	if secondManifest.Paths.RuntimeRoot != firstManifest.Paths.RuntimeRoot {
		t.Fatalf("expected stable runtime root, got %q and %q", firstManifest.Paths.RuntimeRoot, secondManifest.Paths.RuntimeRoot)
	}
}

func TestManifestRejectsPathsOutsideConfigDirectoryOutsideGit(t *testing.T) {
	repo := t.TempDir()
	appDir := filepath.Join(repo, "apps", "web")
	nestedCWD := filepath.Join(appDir, "subdir")
	configPath := filepath.Join(appDir, "devlane.yaml")

	if err := os.MkdirAll(nestedCWD, 0o755); err != nil {
		t.Fatalf("mkdir nested cwd: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`
schema: 1
app: nested
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
  manifest_path: ../outside.json
  generated: []
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	adapter, err := config.LoadAdapter(configPath)
	if err != nil {
		t.Fatalf("LoadAdapter returned error: %v", err)
	}

	_, err = manifest.Build(adapter, manifest.Options{
		CWD:        nestedCWD,
		ConfigPath: configPath,
	})
	if err == nil {
		t.Fatal("expected Build to reject paths outside config directory")
	}
	if !strings.Contains(err.Error(), "repo root") {
		t.Fatalf("unexpected error: %v", err)
	}
}
