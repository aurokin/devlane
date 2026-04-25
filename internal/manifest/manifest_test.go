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
	if !samePath(t, laneManifest.Paths.Manifest, filepath.Join(canonicalRepo, ".devlane", "manifest.json")) {
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
	if laneManifest.Lane.ConfigPath != configPath {
		t.Fatalf("expected config path to preserve selected symlink spelling, got %s", laneManifest.Lane.ConfigPath)
	}
	if laneManifest.Paths.Manifest != filepath.Join(symlinkRoot, ".devlane", "manifest.json") {
		t.Fatalf("expected manifest path anchored at canonical repo root, got %s", laneManifest.Paths.Manifest)
	}
}

func TestManifestPreservesSymlinkedConfigPathAsAdapterRoot(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	sharedRoot := t.TempDir()
	sharedConfigPath := filepath.Join(sharedRoot, "shared-devlane.yaml")
	payload, err := os.ReadFile(filepath.Join(repo, "devlane.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if err := os.WriteFile(sharedConfigPath, payload, 0o644); err != nil {
		t.Fatalf("write shared config: %v", err)
	}
	configPath := filepath.Join(repo, "devlane.yaml")
	if err := os.Remove(configPath); err != nil {
		t.Fatalf("remove repo config: %v", err)
	}
	if err := os.Symlink(sharedConfigPath, configPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
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

	canonicalRepo, err := util.CanonicalPath(repo)
	if err != nil {
		t.Fatalf("canonicalize repo: %v", err)
	}
	if laneManifest.Lane.RepoRoot != canonicalRepo {
		t.Fatalf("expected canonical repo root %s, got %s", canonicalRepo, laneManifest.Lane.RepoRoot)
	}
	if laneManifest.Lane.ConfigPath != configPath {
		t.Fatalf("expected config path to preserve symlink location, got %s", laneManifest.Lane.ConfigPath)
	}
	if laneManifest.Paths.Manifest != filepath.Join(repo, ".devlane", "manifest.json") {
		t.Fatalf("expected manifest path anchored at symlink config directory, got %s", laneManifest.Paths.Manifest)
	}
	if laneManifest.Compose.Files[0] != filepath.Join(repo, "compose.yaml") {
		t.Fatalf("expected compose file anchored at symlink config directory, got %s", laneManifest.Compose.Files[0])
	}
}

func TestManifestUsesAdapterRootForDetachedLaneIdentityOutsideGit(t *testing.T) {
	configPath, firstCWD, secondCWD := setupDetachedAdapterRootFixture(t)
	firstManifest := buildTestManifest(t, configPath, firstCWD)
	secondManifest := buildTestManifest(t, configPath, secondCWD)

	if firstManifest.Lane.Name != "web" {
		t.Fatalf("expected adapter-root lane name, got %q", firstManifest.Lane.Name)
	}
	assertDetachedLaneIdentity(t, firstManifest, secondManifest)
}

func setupDetachedAdapterRootFixture(t *testing.T) (string, string, string) {
	t.Helper()

	repo := t.TempDir()
	appDir := filepath.Join(repo, "apps", "web")
	firstCWD := filepath.Join(appDir, "subdir", "child")
	secondCWD := filepath.Join(appDir, "other", "leaf")
	configPath := filepath.Join(appDir, "devlane.yaml")

	mustMkdirAll(t, filepath.Join(appDir, "templates"))
	for _, dir := range []string{firstCWD, secondCWD} {
		mustMkdirAll(t, dir)
	}
	mustWriteManifestTestFile(t, filepath.Join(appDir, "templates", "app.env.tmpl"), "APP={{app}}\n")
	mustWriteManifestTestFile(t, configPath, `
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
`)

	return configPath, firstCWD, secondCWD
}

func buildTestManifest(t *testing.T, configPath, cwd string) manifest.Manifest {
	t.Helper()

	adapter, err := config.LoadAdapter(configPath)
	if err != nil {
		t.Fatalf("LoadAdapter returned error: %v", err)
	}

	laneManifest, err := manifest.Build(adapter, manifest.Options{
		CWD:        cwd,
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("Build manifest: %v", err)
	}
	return laneManifest
}

func assertDetachedLaneIdentity(t *testing.T, firstManifest, secondManifest manifest.Manifest) {
	t.Helper()

	assertEqualString(t, secondManifest.Lane.Name, firstManifest.Lane.Name, "stable detached lane name")
	assertEqualString(t, secondManifest.Lane.Slug, firstManifest.Lane.Slug, "stable detached lane slug")
	assertEqualString(t, secondManifest.Network.ProjectName, firstManifest.Network.ProjectName, "stable project name")
	assertEqualString(t, secondManifest.Paths.Manifest, firstManifest.Paths.Manifest, "stable manifest path")
	assertEqualString(t, secondManifest.Paths.StateRoot, firstManifest.Paths.StateRoot, "stable state root")
	assertEqualString(t, secondManifest.Paths.CacheRoot, firstManifest.Paths.CacheRoot, "stable cache root")
	assertEqualString(t, secondManifest.Paths.RuntimeRoot, firstManifest.Paths.RuntimeRoot, "stable runtime root")
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteManifestTestFile(t *testing.T, path, payload string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertEqualString(t *testing.T, got, want, label string) {
	t.Helper()

	if got != want {
		t.Fatalf("expected %s, got %q and %q", label, want, got)
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

func samePath(t *testing.T, left, right string) bool {
	t.Helper()

	leftCanonical, err := util.CanonicalPath(left)
	if err != nil {
		t.Fatalf("canonicalize %s: %v", left, err)
	}
	rightCanonical, err := util.CanonicalPath(right)
	if err != nil {
		t.Fatalf("canonicalize %s: %v", right, err)
	}
	return leftCanonical == rightCanonical
}
