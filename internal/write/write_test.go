package write_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/auro/devlane/internal/config"
	"github.com/auro/devlane/internal/manifest"
	"github.com/auro/devlane/internal/testutil"
	"github.com/auro/devlane/internal/util"
	"github.com/auro/devlane/internal/write"
)

func TestPrepareWritesManifestRenderedFilesAndSidecars(t *testing.T) {
	repo, adapter, laneManifest := buildDemoManifest(t)

	result, err := write.Prepare(laneManifest, adapter)
	if err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}
	if len(result.Messages) != 0 {
		t.Fatalf("expected no prepare messages, got %v", result.Messages)
	}

	assertManifestFileIncludesPortState(t, laneManifest.Paths.Manifest)
	assertComposeEnvPayload(t, *laneManifest.Paths.ComposeEnv)
	assertRenderedOutput(t, repo)
	assertGeneratedSidecars(t, repo, 1)
}

func TestPrepareWarnsWhenGeneratedFileWasEdited(t *testing.T) {
	repo, adapter, laneManifest := buildDemoManifest(t)

	if _, err := write.Prepare(laneManifest, adapter); err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}

	renderedPath := filepath.Join(repo, ".devlane", "generated", "app.env")
	if err := os.WriteFile(renderedPath, []byte("manually edited\n"), 0o644); err != nil {
		t.Fatalf("overwrite rendered file: %v", err)
	}

	result, err := write.Prepare(laneManifest, adapter)
	if err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 warning, got %v", result.Messages)
	}
	if !strings.Contains(result.Messages[0], "generated file was modified; overwriting") {
		t.Fatalf("unexpected warning: %v", result.Messages)
	}
}

func TestPrepareNoticesExistingGeneratedFileWithoutSidecar(t *testing.T) {
	repo, adapter, laneManifest := buildDemoManifest(t)

	renderedPath := filepath.Join(repo, ".devlane", "generated", "app.env")
	if err := os.MkdirAll(filepath.Dir(renderedPath), 0o755); err != nil {
		t.Fatalf("mkdir rendered dir: %v", err)
	}
	if err := os.WriteFile(renderedPath, []byte("stale file\n"), 0o644); err != nil {
		t.Fatalf("write stale generated file: %v", err)
	}

	result, err := write.Prepare(laneManifest, adapter)
	if err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 notice, got %v", result.Messages)
	}
	if !strings.Contains(result.Messages[0], "without sidecar hash") {
		t.Fatalf("unexpected notice: %v", result.Messages)
	}
}

func TestPreparePreservesExistingFileModes(t *testing.T) {
	repo, adapter, laneManifest := buildDemoManifest(t)

	if laneManifest.Paths.ComposeEnv == nil {
		t.Fatal("expected compose env path")
	}
	if err := os.MkdirAll(filepath.Dir(*laneManifest.Paths.ComposeEnv), 0o755); err != nil {
		t.Fatalf("mkdir compose env dir: %v", err)
	}
	if err := os.WriteFile(*laneManifest.Paths.ComposeEnv, []byte("OLD=1\n"), 0o644); err != nil {
		t.Fatalf("write compose env: %v", err)
	}
	if err := os.Chmod(*laneManifest.Paths.ComposeEnv, 0o600); err != nil {
		t.Fatalf("chmod compose env: %v", err)
	}

	renderedPath := filepath.Join(repo, ".devlane", "generated", "app.env")
	if err := os.MkdirAll(filepath.Dir(renderedPath), 0o755); err != nil {
		t.Fatalf("mkdir rendered dir: %v", err)
	}
	if err := os.WriteFile(renderedPath, []byte("OLD=1\n"), 0o644); err != nil {
		t.Fatalf("write rendered output: %v", err)
	}
	if err := os.Chmod(renderedPath, 0o755); err != nil {
		t.Fatalf("chmod rendered output: %v", err)
	}

	result, err := write.Prepare(laneManifest, adapter)
	if err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}
	if len(result.Messages) != 1 || !strings.Contains(result.Messages[0], "without sidecar hash") {
		t.Fatalf("expected generated-file notice, got %v", result.Messages)
	}

	assertFileMode(t, *laneManifest.Paths.ComposeEnv, 0o600)
	assertFileMode(t, renderedPath, 0o755)
	assertComposeEnvPayload(t, *laneManifest.Paths.ComposeEnv)
	assertRenderedOutputPath(t, renderedPath)
}

func TestPrepareWritesThroughSymlinkedTargetsWithoutReplacingLinks(t *testing.T) {
	repo, adapter, laneManifest := buildDemoManifest(t)

	if laneManifest.Paths.ComposeEnv == nil {
		t.Fatal("expected compose env path")
	}

	composeTarget := filepath.Join(repo, ".devlane", "shared", "compose.env")
	if err := os.MkdirAll(filepath.Dir(composeTarget), 0o755); err != nil {
		t.Fatalf("mkdir compose target dir: %v", err)
	}
	if err := os.WriteFile(composeTarget, []byte("OLD=1\n"), 0o644); err != nil {
		t.Fatalf("write compose target: %v", err)
	}
	if err := os.RemoveAll(*laneManifest.Paths.ComposeEnv); err != nil {
		t.Fatalf("remove compose env path: %v", err)
	}
	if err := os.Symlink(composeTarget, *laneManifest.Paths.ComposeEnv); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	renderedPath := filepath.Join(repo, ".devlane", "generated", "app.env")
	renderedTarget := filepath.Join(repo, ".devlane", "shared", "app.env")
	if err := os.WriteFile(renderedTarget, []byte("OLD=1\n"), 0o644); err != nil {
		t.Fatalf("write rendered target: %v", err)
	}
	if err := os.RemoveAll(renderedPath); err != nil {
		t.Fatalf("remove rendered path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(renderedPath), 0o755); err != nil {
		t.Fatalf("mkdir rendered dir: %v", err)
	}
	if err := os.Symlink(renderedTarget, renderedPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	composeLink, err := os.Readlink(*laneManifest.Paths.ComposeEnv)
	if err != nil {
		t.Fatalf("read compose symlink: %v", err)
	}
	renderedLink, err := os.Readlink(renderedPath)
	if err != nil {
		t.Fatalf("read rendered symlink: %v", err)
	}

	result, err := write.Prepare(laneManifest, adapter)
	if err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}
	if len(result.Messages) != 1 || !strings.Contains(result.Messages[0], "without sidecar hash") {
		t.Fatalf("expected generated-file notice, got %v", result.Messages)
	}

	assertSymlink(t, *laneManifest.Paths.ComposeEnv, composeLink)
	assertSymlink(t, renderedPath, renderedLink)
	assertComposeEnvPayload(t, composeTarget)
	assertRenderedOutputPath(t, renderedTarget)
}

func TestPrepareRejectsMissingSymlinkTargetsBeforePromotingWrites(t *testing.T) {
	repo, adapter, laneManifest := buildDemoManifest(t)

	if laneManifest.Paths.ComposeEnv == nil {
		t.Fatal("expected compose env path")
	}

	composeTarget := filepath.Join(t.TempDir(), "shared", "compose.env")
	if err := os.RemoveAll(*laneManifest.Paths.ComposeEnv); err != nil {
		t.Fatalf("remove compose env path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(*laneManifest.Paths.ComposeEnv), 0o755); err != nil {
		t.Fatalf("mkdir compose env dir: %v", err)
	}
	if err := os.Symlink(composeTarget, *laneManifest.Paths.ComposeEnv); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	renderedPath := filepath.Join(repo, ".devlane", "generated", "app.env")
	renderedTarget := filepath.Join(t.TempDir(), "shared", "app.env")
	if err := os.RemoveAll(renderedPath); err != nil {
		t.Fatalf("remove rendered path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(renderedPath), 0o755); err != nil {
		t.Fatalf("mkdir rendered dir: %v", err)
	}
	if err := os.Symlink(renderedTarget, renderedPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	_, err := write.Prepare(laneManifest, adapter)
	if err == nil {
		t.Fatal("expected Prepare to fail")
	}
	if !strings.Contains(err.Error(), "symlink target must already exist") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(err.Error(), "partial local state") {
		t.Fatalf("expected preflight failure before promotion, got %v", err)
	}

	for _, path := range []string{
		laneManifest.Paths.Manifest,
		composeTarget,
		renderedTarget,
	} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("expected %s to remain absent, stat err=%v", path, statErr)
		}
	}
}

func TestPrepareRejectsComposeEnvDirectoryBeforePromotingWrites(t *testing.T) {
	repo, adapter, laneManifest := buildDemoManifest(t)

	if laneManifest.Paths.ComposeEnv == nil {
		t.Fatal("expected compose env path")
	}
	if err := os.MkdirAll(*laneManifest.Paths.ComposeEnv, 0o755); err != nil {
		t.Fatalf("mkdir compose env path: %v", err)
	}

	_, err := write.Prepare(laneManifest, adapter)
	if err == nil {
		t.Fatal("expected Prepare to fail")
	}
	if !strings.Contains(err.Error(), "compose env target must be a file") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(err.Error(), "partial local state") {
		t.Fatalf("expected preflight failure before promotion, got %v", err)
	}
	assertPathAbsent(t, laneManifest.Paths.Manifest)
	assertPathAbsent(t, filepath.Join(repo, ".devlane", "generated", "app.env"))
}

func TestPrepareRejectsGeneratedSidecarDirectoryBeforePromotingWrites(t *testing.T) {
	repo, adapter, laneManifest := buildDemoManifest(t)

	sidecarPath := generatedSidecarPathForTest(t, laneManifest.Lane.RepoRoot, laneManifest.Outputs.Generated[0].Destination)
	if err := os.MkdirAll(sidecarPath, 0o755); err != nil {
		t.Fatalf("mkdir sidecar path: %v", err)
	}

	_, err := write.Prepare(laneManifest, adapter)
	if err == nil {
		t.Fatal("expected Prepare to fail")
	}
	if !strings.Contains(err.Error(), "generated sidecar target must be a file") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(err.Error(), "partial local state") {
		t.Fatalf("expected preflight failure before promotion, got %v", err)
	}
	assertPathAbsent(t, laneManifest.Paths.Manifest)
	assertPathAbsent(t, filepath.Join(repo, ".devlane", "generated", "app.env"))
}

func TestTemplateContextExposesManifestSections(t *testing.T) {
	composeEnv := "/tmp/repo/.devlane/compose.env"
	publicHost := "demoapp.localhost"
	publicURL := "http://demoapp.localhost"
	laneManifest := manifest.Manifest{
		App:   "demoapp",
		Kind:  "web",
		Ready: true,
		Lane: manifest.Lane{
			Name:       "stable",
			Slug:       "stable",
			Mode:       "stable",
			Stable:     true,
			Branch:     "main",
			RepoRoot:   "/tmp/repo",
			ConfigPath: "/tmp/repo/devlane.yaml",
		},
		Paths: manifest.Paths{
			Manifest:    "/tmp/repo/.devlane/manifest.json",
			ComposeEnv:  &composeEnv,
			StateRoot:   "/tmp/repo/.devlane/state/stable",
			CacheRoot:   "/tmp/repo/.devlane/cache/stable",
			RuntimeRoot: "/tmp/repo/.devlane/runtime/stable",
		},
		Network: manifest.Network{
			ProjectName: "demoapp_stable",
			PublicHost:  &publicHost,
			PublicURL:   &publicURL,
		},
		Ports: manifest.Ports{
			"web": {
				Port:      3100,
				Allocated: true,
			},
		},
		Compose: manifest.Compose{
			Files:    []string{},
			Profiles: []string{},
		},
		Outputs: manifest.Outputs{
			Generated: []manifest.GeneratedOutput{},
		},
	}

	adapter := &config.AdapterConfig{
		Schema: 1,
		App:    "demoapp",
		Kind:   "web",
		Runtime: config.RuntimeConfig{
			Env: map[string]any{},
		},
	}

	context, err := write.TemplateContext(laneManifest, adapter)
	if err != nil {
		t.Fatalf("TemplateContext returned error: %v", err)
	}

	network := context["network"].(map[string]any)
	lane := context["lane"].(map[string]any)
	env := context["env"].(map[string]string)
	ports := context["ports"].(map[string]any)

	if context["ready"] != true {
		t.Fatalf("unexpected ready value: %#v", context["ready"])
	}
	if network["publicHost"] != "demoapp.localhost" {
		t.Fatalf("unexpected publicHost: %#v", network["publicHost"])
	}
	if lane["slug"] != "stable" {
		t.Fatalf("unexpected lane slug: %#v", lane["slug"])
	}
	if env["DEVLANE_APP"] != "demoapp" {
		t.Fatalf("unexpected DEVLANE_APP: %q", env["DEVLANE_APP"])
	}
	if env["DEVLANE_PORT_WEB"] != "3100" {
		t.Fatalf("unexpected DEVLANE_PORT_WEB: %q", env["DEVLANE_PORT_WEB"])
	}
	if ports["web"] != 3100 {
		t.Fatalf("unexpected ports.web: %#v", ports["web"])
	}
}

func TestTemplateContextSanitizesSlashDelimitedPortEnvKeys(t *testing.T) {
	_, adapter, laneManifest := buildDemoManifest(t)
	laneManifest.Ready = true
	laneManifest.Ports = manifest.Ports{
		"web/api": {
			Port:      3100,
			Allocated: true,
		},
	}

	context, err := write.TemplateContext(laneManifest, adapter)
	if err != nil {
		t.Fatalf("TemplateContext returned error: %v", err)
	}

	env := context["env"].(map[string]string)
	ports := context["ports"].(map[string]any)

	if env["DEVLANE_PORT_API"] != "3100" {
		t.Fatalf("unexpected DEVLANE_PORT_API: %q", env["DEVLANE_PORT_API"])
	}
	if _, ok := env["DEVLANE_PORT_WEB/API"]; ok {
		t.Fatalf("unexpected unsanitized env key: %#v", env)
	}
	if ports["web/api"] != 3100 {
		t.Fatalf("unexpected ports[web/api]: %#v", ports["web/api"])
	}
}

func buildDemoManifest(t *testing.T) (string, *config.AdapterConfig, manifest.Manifest) {
	t.Helper()

	repo := testutil.InitDemoRepo(t)
	configPath := filepath.Join(repo, "devlane.yaml")

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

	return repo, adapter, laneManifest
}

func assertManifestFileIncludesPortState(t *testing.T, manifestPath string) {
	t.Helper()

	manifestPayload, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	var stored map[string]any
	if err := json.Unmarshal(manifestPayload, &stored); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}

	if stored["ready"] != false {
		t.Fatalf("expected pre-prepare ready=false, got %#v", stored["ready"])
	}
	ports, ok := stored["ports"].(map[string]any)
	if !ok {
		t.Fatalf("expected ports object in manifest, got %#v", stored["ports"])
	}
	if _, ok := ports["web"]; !ok {
		t.Fatalf("expected web port entry in manifest, got %#v", ports)
	}
}

func assertComposeEnvPayload(t *testing.T, composeEnvPath string) {
	t.Helper()

	composeEnvPayload, err := os.ReadFile(composeEnvPath)
	if err != nil {
		t.Fatalf("read compose env: %v", err)
	}
	if !strings.Contains(string(composeEnvPayload), "DEVLANE_PUBLIC_HOST=feature-test-lane.demoapp.localhost") {
		t.Fatalf("compose env missing expected host:\n%s", composeEnvPayload)
	}
	if strings.Contains(string(composeEnvPayload), "DEVLANE_PORT_WEB=") {
		t.Fatalf("compose env should not include Phase 2 port projection:\n%s", composeEnvPayload)
	}
}

func assertRenderedOutput(t *testing.T, repo string) {
	t.Helper()

	renderedPath := filepath.Join(repo, ".devlane", "generated", "app.env")
	assertRenderedOutputPath(t, renderedPath)
}

func assertRenderedOutputPath(t *testing.T, renderedPath string) {
	t.Helper()

	renderedPayload, err := os.ReadFile(renderedPath)
	if err != nil {
		t.Fatalf("read rendered output: %v", err)
	}
	if !strings.Contains(string(renderedPayload), "DEVLANE_LANE=feature/test-lane") {
		t.Fatalf("rendered output missing lane name:\n%s", renderedPayload)
	}
	if !strings.Contains(string(renderedPayload), "APP_MODE=development") {
		t.Fatalf("rendered output missing APP_MODE:\n%s", renderedPayload)
	}
}

func assertGeneratedSidecars(t *testing.T, repo string, want int) {
	t.Helper()

	sidecarEntries, err := os.ReadDir(filepath.Join(repo, ".devlane", "generated-hashes"))
	if err != nil {
		t.Fatalf("read sidecar dir: %v", err)
	}
	if len(sidecarEntries) != want {
		t.Fatalf("expected %d generated sidecar(s), got %d", want, len(sidecarEntries))
	}
}

func assertPathAbsent(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be absent, stat err=%v", path, err)
	}
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("expected mode %o for %s, got %o", want, path, got)
	}
}

func assertSymlink(t *testing.T, path, wantTarget string) {
	t.Helper()

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected %s to remain a symlink", path)
	}
	gotTarget, err := os.Readlink(path)
	if err != nil {
		t.Fatalf("readlink %s: %v", path, err)
	}
	if gotTarget != wantTarget {
		t.Fatalf("expected symlink %s -> %s, got %s", path, wantTarget, gotTarget)
	}
}

func generatedSidecarPathForTest(t *testing.T, repoRoot, destinationPath string) string {
	t.Helper()

	canonicalRepoRoot, err := util.CanonicalPath(repoRoot)
	if err != nil {
		t.Fatalf("canonicalize repo root: %v", err)
	}
	canonicalDestinationPath, err := util.CanonicalPath(destinationPath)
	if err != nil {
		t.Fatalf("canonicalize destination path: %v", err)
	}

	relative, err := filepath.Rel(canonicalRepoRoot, canonicalDestinationPath)
	if err != nil {
		t.Fatalf("derive relative destination: %v", err)
	}

	hash := sha256.Sum256([]byte(filepath.ToSlash(relative)))
	return filepath.Join(repoRoot, ".devlane", "generated-hashes", hex.EncodeToString(hash[:])+".sha256")
}
