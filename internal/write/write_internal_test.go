package write

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/auro/devlane/internal/config"
	"github.com/auro/devlane/internal/manifest"
	"github.com/auro/devlane/internal/testutil"
)

func TestPrepareWithRollbackRestoresPromotedFilesWhenApplyFails(t *testing.T) {
	repo, adapter, laneManifest := buildDemoManifestInternal(t)

	if err := os.MkdirAll(filepath.Dir(laneManifest.Paths.Manifest), 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	if err := os.WriteFile(laneManifest.Paths.Manifest, []byte("old manifest\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if laneManifest.Paths.ComposeEnv == nil {
		t.Fatal("expected compose env path")
	}
	if err := os.WriteFile(*laneManifest.Paths.ComposeEnv, []byte("OLD=1\n"), 0o644); err != nil {
		t.Fatalf("write compose env: %v", err)
	}

	originalRename := renameFile
	renameCalls := 0
	renameFile = func(from, to string) error {
		renameCalls++
		if renameCalls == 2 {
			return errors.New("forced rename failure")
		}
		return originalRename(from, to)
	}
	defer func() {
		renameFile = originalRename
	}()

	_, rollback, err := PrepareWithRollback(laneManifest, adapter)
	if err == nil {
		t.Fatal("expected PrepareWithRollback to fail")
	}
	if rollback != nil {
		t.Fatal("expected rollback callback to be nil on apply failure")
	}
	if !strings.Contains(err.Error(), "forced rename failure") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "rolled back promoted local state") {
		t.Fatalf("expected rollback detail in error, got %v", err)
	}

	manifestPayload, readErr := os.ReadFile(laneManifest.Paths.Manifest)
	if readErr != nil {
		t.Fatalf("read restored manifest: %v", readErr)
	}
	if string(manifestPayload) != "old manifest\n" {
		t.Fatalf("expected manifest rollback, got %q", manifestPayload)
	}

	composeEnvPayload, readErr := os.ReadFile(*laneManifest.Paths.ComposeEnv)
	if readErr != nil {
		t.Fatalf("read restored compose env: %v", readErr)
	}
	if string(composeEnvPayload) != "OLD=1\n" {
		t.Fatalf("expected compose env rollback, got %q", composeEnvPayload)
	}

	renderedPath := filepath.Join(repo, ".devlane", "generated", "app.env")
	if _, statErr := os.Stat(renderedPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected generated output to remain absent, stat err=%v", statErr)
	}
}

func TestPrepareWithRollbackRestoresSymlinkTargetsWithoutReplacingLinksWhenApplyFails(t *testing.T) {
	repo, adapter, laneManifest := buildDemoManifestInternal(t)

	if err := os.MkdirAll(filepath.Dir(laneManifest.Paths.Manifest), 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	if err := os.WriteFile(laneManifest.Paths.Manifest, []byte("old manifest\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
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
	if err := os.WriteFile(renderedTarget, []byte("OLD_GENERATED=1\n"), 0o644); err != nil {
		t.Fatalf("write rendered target: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(renderedPath), 0o755); err != nil {
		t.Fatalf("mkdir rendered dir: %v", err)
	}
	if err := os.RemoveAll(renderedPath); err != nil {
		t.Fatalf("remove rendered path: %v", err)
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

	originalRename := renameFile
	renameCalls := 0
	renameFile = func(from, to string) error {
		renameCalls++
		if renameCalls == 4 {
			return errors.New("forced rename failure")
		}
		return originalRename(from, to)
	}
	defer func() {
		renameFile = originalRename
	}()

	_, rollback, err := PrepareWithRollback(laneManifest, adapter)
	if err == nil {
		t.Fatal("expected PrepareWithRollback to fail")
	}
	if rollback != nil {
		t.Fatal("expected rollback callback to be nil on apply failure")
	}
	if !strings.Contains(err.Error(), "forced rename failure") {
		t.Fatalf("unexpected error: %v", err)
	}

	manifestPayload, readErr := os.ReadFile(laneManifest.Paths.Manifest)
	if readErr != nil {
		t.Fatalf("read restored manifest: %v", readErr)
	}
	if string(manifestPayload) != "old manifest\n" {
		t.Fatalf("expected manifest rollback, got %q", manifestPayload)
	}

	composePayload, readErr := os.ReadFile(composeTarget)
	if readErr != nil {
		t.Fatalf("read restored compose target: %v", readErr)
	}
	if string(composePayload) != "OLD=1\n" {
		t.Fatalf("expected compose target rollback, got %q", composePayload)
	}

	renderedPayload, readErr := os.ReadFile(renderedTarget)
	if readErr != nil {
		t.Fatalf("read restored rendered target: %v", readErr)
	}
	if string(renderedPayload) != "OLD_GENERATED=1\n" {
		t.Fatalf("expected rendered target rollback, got %q", renderedPayload)
	}

	assertSymlinkInternal(t, *laneManifest.Paths.ComposeEnv, composeLink)
	assertSymlinkInternal(t, renderedPath, renderedLink)
	sidecarPath, err := generatedSidecarPath(laneManifest.Lane.RepoRoot, renderedPath)
	if err != nil {
		t.Fatalf("derive sidecar path: %v", err)
	}
	if _, statErr := os.Stat(sidecarPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected generated sidecar to remain absent, stat err=%v", statErr)
	}
}

func TestPrepareWithRollbackRejectsMissingSymlinkTargetsBeforePromotion(t *testing.T) {
	repo, adapter, laneManifest := buildDemoManifestInternal(t)

	if laneManifest.Paths.ComposeEnv == nil {
		t.Fatal("expected compose env path")
	}

	composeTarget := filepath.Join(t.TempDir(), "shared", "compose.env")
	if err := os.RemoveAll(*laneManifest.Paths.ComposeEnv); err != nil {
		t.Fatalf("remove compose env path: %v", err)
	}
	if err := os.Symlink(composeTarget, *laneManifest.Paths.ComposeEnv); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	renderedPath := filepath.Join(repo, ".devlane", "generated", "app.env")
	renderedTarget := filepath.Join(t.TempDir(), "shared", "app.env")
	if err := os.MkdirAll(filepath.Dir(renderedPath), 0o755); err != nil {
		t.Fatalf("mkdir rendered dir: %v", err)
	}
	if err := os.RemoveAll(renderedPath); err != nil {
		t.Fatalf("remove rendered path: %v", err)
	}
	if err := os.Symlink(renderedTarget, renderedPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	_, rollback, err := PrepareWithRollback(laneManifest, adapter)
	if err == nil {
		t.Fatal("expected PrepareWithRollback to fail")
	}
	if rollback != nil {
		t.Fatal("expected rollback callback to be nil on preflight failure")
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

func TestPrepareAcceptsGeneratedOutputsWithEquivalentRootSpellings(t *testing.T) {
	repo, adapter, laneManifest := buildDemoManifestInternal(t)

	linkRoot := filepath.Join(filepath.Dir(repo), "linked-repo")
	if err := os.Symlink(repo, linkRoot); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	composeEnvPath := filepath.Join(linkRoot, ".devlane", "compose.env")
	laneManifest.Lane.ConfigPath = filepath.Join(linkRoot, "devlane.yaml")
	laneManifest.Paths.Manifest = filepath.Join(linkRoot, ".devlane", "manifest.json")
	laneManifest.Paths.ComposeEnv = &composeEnvPath
	laneManifest.Compose.Files = []string{
		filepath.Join(linkRoot, "compose.yaml"),
		filepath.Join(linkRoot, "compose.devlane.yaml"),
	}
	laneManifest.Outputs.Generated = []manifest.GeneratedOutput{
		{
			Template:    filepath.Join(linkRoot, "templates", "app.env.tmpl"),
			Destination: filepath.Join(linkRoot, ".devlane", "generated", "app.env"),
		},
	}

	if _, err := Prepare(laneManifest, adapter); err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".devlane", "generated", "app.env")); err != nil {
		t.Fatalf("expected generated output through equivalent path spelling: %v", err)
	}
}

func TestPrepareRejectsGeneratedSymlinkTargetOutsideRepo(t *testing.T) {
	repo, adapter, laneManifest := buildDemoManifestInternal(t)

	renderedPath := filepath.Join(repo, ".devlane", "generated", "app.env")
	outsideTarget := filepath.Join(t.TempDir(), "app.env")
	if err := os.WriteFile(outsideTarget, []byte("OUTSIDE=1\n"), 0o644); err != nil {
		t.Fatalf("write outside target: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(renderedPath), 0o755); err != nil {
		t.Fatalf("mkdir rendered dir: %v", err)
	}
	if err := os.Symlink(outsideTarget, renderedPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	_, rollback, err := PrepareWithRollback(laneManifest, adapter)
	if err == nil {
		t.Fatal("expected PrepareWithRollback to fail")
	}
	if rollback != nil {
		t.Fatal("expected rollback callback to be nil on preflight failure")
	}
	if !strings.Contains(err.Error(), "repo root") {
		t.Fatalf("expected repo root containment error, got %v", err)
	}
}

func assertSymlinkInternal(t *testing.T, path, wantTarget string) {
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

func buildDemoManifestInternal(t *testing.T) (string, *config.AdapterConfig, manifest.Manifest) {
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
