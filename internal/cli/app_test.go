package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/auro/devlane/internal/cli"
	"github.com/auro/devlane/internal/testutil"
)

func TestPrepareMissingAdapterPointsAtInit(t *testing.T) {
	cwd := t.TempDir()

	code, _, stderr := runCLI(t, []string{"prepare", "--cwd", cwd})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stderr, "run `devlane init`") {
		t.Fatalf("expected init guidance, got stderr:\n%s", stderr)
	}
}

func TestInspectFindsNearestAdapterFromCWD(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	cwd := filepath.Join(repo, "nested", "child")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("mkdir nested cwd: %v", err)
	}

	code, stdout, stderr := runCLI(t, []string{"inspect", "--cwd", cwd, "--json"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stdout, `"app": "demoapp"`) {
		t.Fatalf("expected inspect JSON output, got:\n%s", stdout)
	}
}

func TestInspectAndPrepareUseSamePathAnchors(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	removePortsBlock(t, filepath.Join(repo, "devlane.yaml"))
	cwd := filepath.Join(repo, "nested", "child")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("mkdir nested cwd: %v", err)
	}

	code, stdout, stderr := runCLI(t, []string{"inspect", "--cwd", cwd, "--json"})
	if code != 0 {
		t.Fatalf("expected inspect exit code 0, got %d\nstderr:\n%s", code, stderr)
	}
	var inspected map[string]any
	if err := json.Unmarshal([]byte(stdout), &inspected); err != nil {
		t.Fatalf("decode inspect JSON: %v\n%s", err, stdout)
	}

	code, _, stderr = runCLI(t, []string{"prepare", "--cwd", cwd})
	if code != 0 {
		t.Fatalf("expected prepare exit code 0, got %d\nstderr:\n%s", code, stderr)
	}
	payload, err := os.ReadFile(filepath.Join(repo, ".devlane", "manifest.json"))
	if err != nil {
		t.Fatalf("read prepared manifest: %v", err)
	}
	var prepared map[string]any
	if err := json.Unmarshal(payload, &prepared); err != nil {
		t.Fatalf("decode prepared manifest: %v\n%s", err, payload)
	}

	if inspected["lane"].(map[string]any)["repoRoot"] != prepared["lane"].(map[string]any)["repoRoot"] {
		t.Fatalf("inspect and prepare disagreed on repoRoot:\ninspect=%v\nprepare=%v", inspected["lane"], prepared["lane"])
	}
	inspectedGenerated := inspected["outputs"].(map[string]any)["generated"].([]any)[0].(map[string]any)["destination"]
	preparedGenerated := prepared["outputs"].(map[string]any)["generated"].([]any)[0].(map[string]any)["destination"]
	if inspectedGenerated != preparedGenerated {
		t.Fatalf("inspect and prepare disagreed on generated destination: %v vs %v", inspectedGenerated, preparedGenerated)
	}
}

func TestUpFailsWhenPortsAreUnallocatedAndDoesNotImplicitlyRunPrepare(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	manifestPath := filepath.Join(repo, ".devlane", "manifest.json")
	composeEnvPath := filepath.Join(repo, ".devlane", "compose.env")

	code, stdout, stderr := runCLI(t, []string{
		"up",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
		"--dry-run",
	})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d\nstderr:\n%s", code, stderr)
	}
	if strings.Contains(stdout, "docker compose") {
		t.Fatalf("did not expect compose dry-run output before prepare, got:\n%s", stdout)
	}
	if !strings.Contains(stderr, "run `devlane prepare` first") {
		t.Fatalf("expected prepare guidance, got stderr:\n%s", stderr)
	}
	if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
		t.Fatalf("expected no implicit manifest write, stat err=%v", err)
	}
	if _, err := os.Stat(composeEnvPath); !os.IsNotExist(err) {
		t.Fatalf("expected no implicit compose env write, stat err=%v", err)
	}
}

func TestUpFailsWhenComposeLaneIsUnprepared(t *testing.T) {
	repo := initComposeOnlyRepo(t)
	manifestPath := filepath.Join(repo, ".devlane", "manifest.json")
	composeEnvPath := filepath.Join(repo, ".devlane", "compose.env")

	code, stdout, stderr := runCLI(t, []string{
		"up",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
		"--dry-run",
	})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d\nstderr:\n%s", code, stderr)
	}
	if strings.Contains(stdout, "docker compose") {
		t.Fatalf("did not expect compose dry-run output before prepare, got:\n%s", stdout)
	}
	if !strings.Contains(stderr, "run `devlane prepare` first") {
		t.Fatalf("expected prepare guidance, got stderr:\n%s", stderr)
	}
	if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
		t.Fatalf("expected no implicit manifest write, stat err=%v", err)
	}
	if _, err := os.Stat(composeEnvPath); !os.IsNotExist(err) {
		t.Fatalf("expected no implicit compose env write, stat err=%v", err)
	}
}

func TestUpIsNoOpForPortOnlyBareMetalAdapter(t *testing.T) {
	repo := initPortOnlyRepo(t)
	manifestPath := filepath.Join(repo, ".devlane", "manifest.json")

	code, stdout, stderr := runCLI(t, []string{
		"up",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
		"--dry-run",
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "nothing to start") {
		t.Fatalf("expected no-op up hint, got stdout:\n%s", stdout)
	}
	if strings.Contains(stderr, "run `devlane prepare` first") {
		t.Fatalf("did not expect prepare gate for ports-only adapter, got stderr:\n%s", stderr)
	}
	if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
		t.Fatalf("expected no implicit manifest write, stat err=%v", err)
	}
}

func TestPrepareAllocatesPortsAndProjectsComposeEnv(t *testing.T) {
	repo := testutil.InitDemoRepo(t)

	code, stdout, stderr := runCLI(t, []string{
		"prepare",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	composeEnvPayload, err := os.ReadFile(filepath.Join(repo, ".devlane", "compose.env"))
	if err != nil {
		t.Fatalf("read compose env: %v", err)
	}
	if !strings.Contains(string(composeEnvPayload), "DEVLANE_PORT_WEB=3000") {
		t.Fatalf("expected allocated web port in compose env:\n%s", composeEnvPayload)
	}
}

func TestPrepareSanitizesSlashDelimitedPortEnvKeys(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	mustWriteFile(t, filepath.Join(repo, "devlane.yaml"), `
schema: 1
app: demoapp
kind: web

lane:
  stable_name: stable
  stable_branches: [main, master]
  project_pattern: "{app}_{lane}"
  path_roots:
    state: ".devlane/state"
    cache: ".devlane/cache"
    runtime: ".devlane/runtime"
  host_patterns:
    stable: "{app}.localhost"
    dev: "{lane}.{app}.localhost"

runtime:
  compose_files:
    - compose.yaml
    - compose.devlane.yaml
  default_profiles: [web]
  optional_profiles: [db]
  env:
    APP_MODE: development

ports:
  - name: web/api
    default: 3000
    health_path: /health

outputs:
  manifest_path: ".devlane/manifest.json"
  compose_env_path: ".devlane/compose.env"
  generated:
    - template: "templates/app.env.tmpl"
      destination: ".devlane/generated/app.env"
`)

	code, stdout, stderr := runCLI(t, []string{
		"prepare",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	composeEnvPayload, err := os.ReadFile(filepath.Join(repo, ".devlane", "compose.env"))
	if err != nil {
		t.Fatalf("read compose env: %v", err)
	}
	if !strings.Contains(string(composeEnvPayload), "DEVLANE_PORT_API=3000") {
		t.Fatalf("expected sanitized port env key in compose env:\n%s", composeEnvPayload)
	}
	if strings.Contains(string(composeEnvPayload), "DEVLANE_PORT_WEB/API=") {
		t.Fatalf("unexpected unsanitized port env key in compose env:\n%s", composeEnvPayload)
	}
}

func TestUpFailsWhenPreparedComposeEnvIsStale(t *testing.T) {
	repo := testutil.InitDemoRepo(t)

	code, stdout, stderr := runCLI(t, []string{
		"prepare",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
	})
	if code != 0 {
		t.Fatalf("expected prepare exit code 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	cmd := exec.Command("git", "checkout", "-B", "feature/other-lane")
	cmd.Dir = repo
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout failed: %v\n%s", err, output)
	}

	code, stdout, stderr = runCLI(t, []string{
		"up",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
		"--dry-run",
	})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if strings.Contains(stdout, "docker compose") {
		t.Fatalf("did not expect compose dry-run output for stale prepare state, got:\n%s", stdout)
	}
	if !strings.Contains(stderr, "prepared compose env is stale") {
		t.Fatalf("expected stale compose env guidance, got stderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "run `devlane prepare` first") {
		t.Fatalf("expected prepare guidance, got stderr:\n%s", stderr)
	}
}

func TestUpFailsWhenPreparedGeneratedOutputIsStale(t *testing.T) {
	repo := testutil.InitDemoRepo(t)

	code, stdout, stderr := runCLI(t, []string{
		"prepare",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
	})
	if code != 0 {
		t.Fatalf("expected prepare exit code 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	mustWriteFile(t, filepath.Join(repo, "templates", "app.env.tmpl"), "APP_MODE={{env.APP_MODE}}\nDEVLANE_LANE={{lane.name}}\nDEVLANE_PUBLIC_URL={{network.publicUrl}}\nFEATURE_FLAG=enabled\n")

	code, stdout, stderr = runCLI(t, []string{
		"up",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
		"--dry-run",
	})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if strings.Contains(stdout, "docker compose") {
		t.Fatalf("did not expect compose dry-run output for stale generated output, got:\n%s", stdout)
	}
	if !strings.Contains(stderr, "prepared generated output") {
		t.Fatalf("expected stale generated output guidance, got stderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "run `devlane prepare` first") {
		t.Fatalf("expected prepare guidance, got stderr:\n%s", stderr)
	}
}

func TestBareMetalUpPrintsCommandsWhenPreparedOutputsAreFresh(t *testing.T) {
	repo := initBareMetalGeneratedRepo(t)

	code, stdout, stderr := runCLI(t, []string{
		"prepare",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
	})
	if code != 0 {
		t.Fatalf("expected prepare exit code 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	code, stdout, stderr = runCLI(t, []string{
		"up",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
		"--dry-run",
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Bare-metal commands for lane") {
		t.Fatalf("expected printed bare-metal commands, got stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "npm run dev") {
		t.Fatalf("expected printed run command, got stdout:\n%s", stdout)
	}
	if strings.Contains(stdout, "docker compose") {
		t.Fatalf("did not expect compose output for pure bare-metal up, got stdout:\n%s", stdout)
	}
	if stderr != "" {
		t.Fatalf("expected no stderr, got:\n%s", stderr)
	}
}

func TestBareMetalUpFailsWhenPreparedGeneratedOutputIsStale(t *testing.T) {
	repo := initBareMetalGeneratedRepo(t)

	code, stdout, stderr := runCLI(t, []string{
		"prepare",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
	})
	if code != 0 {
		t.Fatalf("expected prepare exit code 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	mustWriteFile(t, filepath.Join(repo, "templates", "app.env.tmpl"), "DEVLANE_LANE={{lane.name}}\nPORT={{ports.web}}\nFEATURE_FLAG=enabled\n")

	code, stdout, stderr = runCLI(t, []string{
		"up",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
		"--dry-run",
	})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Bare-metal commands for lane") {
		t.Fatalf("expected bare-metal commands before stale-output failure, got stdout:\n%s", stdout)
	}
	if strings.Contains(stdout, "docker compose") {
		t.Fatalf("did not expect compose output for pure bare-metal up, got stdout:\n%s", stdout)
	}
	if !strings.Contains(stderr, "prepared generated output") {
		t.Fatalf("expected stale generated output guidance, got stderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "run `devlane prepare` first") {
		t.Fatalf("expected prepare guidance, got stderr:\n%s", stderr)
	}
}

func TestHybridUpPrintsRunCommandsBeforeFailingOnStaleComposeEnv(t *testing.T) {
	repo := initExampleRepo(t, "examples/hybrid-web")

	code, stdout, stderr := runCLI(t, []string{
		"prepare",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
	})
	if code != 0 {
		t.Fatalf("expected prepare exit code 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	cmd := exec.Command("git", "checkout", "-B", "feature/other-lane")
	cmd.Dir = repo
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout failed: %v\n%s", err, output)
	}

	code, stdout, stderr = runCLI(t, []string{
		"up",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
		"--dry-run",
	})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Bare-metal commands for lane") {
		t.Fatalf("expected hybrid bare-metal commands before compose verification, got stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "bin/rails server -p") {
		t.Fatalf("expected printed Rails command, got stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "bin/sidekiq") {
		t.Fatalf("expected printed worker command, got stdout:\n%s", stdout)
	}
	if strings.Contains(stdout, "docker compose") {
		t.Fatalf("did not expect compose dry-run output for stale prepare state, got:\n%s", stdout)
	}
	if !strings.Contains(stderr, "prepared compose env is stale") {
		t.Fatalf("expected stale compose env guidance, got stderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "run `devlane prepare` first") {
		t.Fatalf("expected prepare guidance, got stderr:\n%s", stderr)
	}
}

func TestHybridUpPrintsRunCommandsBeforeFailingOnStaleGeneratedOutput(t *testing.T) {
	repo := initExampleRepo(t, "examples/hybrid-web")

	code, stdout, stderr := runCLI(t, []string{
		"prepare",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
	})
	if code != 0 {
		t.Fatalf("expected prepare exit code 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	mustWriteFile(t, filepath.Join(repo, "templates", "web.env.tmpl"), "DEVLANE_APP={{app}}\nDEVLANE_LANE={{lane.name}}\nDEVLANE_MODE={{lane.mode}}\nPORT={{ports.web}}\nREDIS_URL=redis://localhost:{{ports.redis}}/0\nRAILS_ENV={{env.RAILS_ENV}}\nFEATURE_FLAG=enabled\n")

	code, stdout, stderr = runCLI(t, []string{
		"up",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
		"--dry-run",
	})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Bare-metal commands for lane") {
		t.Fatalf("expected hybrid bare-metal commands before compose verification, got stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "bin/rails server -p") {
		t.Fatalf("expected printed Rails command, got stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "bin/sidekiq") {
		t.Fatalf("expected printed worker command, got stdout:\n%s", stdout)
	}
	if strings.Contains(stdout, "docker compose") {
		t.Fatalf("did not expect compose dry-run output for stale generated output, got:\n%s", stdout)
	}
	if !strings.Contains(stderr, "prepared generated output") {
		t.Fatalf("expected stale generated output guidance, got stderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "run `devlane prepare` first") {
		t.Fatalf("expected prepare guidance, got stderr:\n%s", stderr)
	}
}

func TestPrepareAllocatesDistinctPortsAcrossLanes(t *testing.T) {
	repoA := testutil.InitDemoRepo(t)
	repoB := testutil.InitDemoRepo(t)
	sharedConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", sharedConfig)

	cmd := exec.Command("git", "checkout", "-B", "feature/other-lane")
	cmd.Dir = repoB
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout failed: %v\n%s", err, output)
	}

	for _, repo := range []string{repoA, repoB} {
		code, stdout, stderr := runCLI(t, []string{
			"prepare",
			"--cwd", repo,
			"--config", filepath.Join(repo, "devlane.yaml"),
		})
		if code != 0 {
			t.Fatalf("prepare failed for %s: code=%d\nstdout:\n%s\nstderr:\n%s", repo, code, stdout, stderr)
		}
	}

	envA, err := os.ReadFile(filepath.Join(repoA, ".devlane", "compose.env"))
	if err != nil {
		t.Fatalf("read compose env A: %v", err)
	}
	envB, err := os.ReadFile(filepath.Join(repoB, ".devlane", "compose.env"))
	if err != nil {
		t.Fatalf("read compose env B: %v", err)
	}

	if !strings.Contains(string(envA), "DEVLANE_PORT_WEB=3000") {
		t.Fatalf("expected first lane to claim default web port:\n%s", envA)
	}
	if !strings.Contains(string(envB), "DEVLANE_PORT_WEB=3001") {
		t.Fatalf("expected second lane to move off the claimed web port:\n%s", envB)
	}
}

func TestPrepareSkipsBoundDefaultPortForDevLane(t *testing.T) {
	repo := testutil.InitDemoRepo(t)

	listener, err := net.Listen("tcp4", "127.0.0.1:3000")
	if err != nil {
		t.Fatalf("listen on 3000: %v", err)
	}
	defer listener.Close()

	code, stdout, stderr := runCLI(t, []string{
		"prepare",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	composeEnvPayload, err := os.ReadFile(filepath.Join(repo, ".devlane", "compose.env"))
	if err != nil {
		t.Fatalf("read compose env: %v", err)
	}
	if !strings.Contains(string(composeEnvPayload), "DEVLANE_PORT_WEB=3001") {
		t.Fatalf("expected prepare to skip bound port 3000:\n%s", composeEnvPayload)
	}
}

func TestPrepareMinimalWebRendersAllocatedAppPort(t *testing.T) {
	repo := initExampleRepo(t, filepath.Join("examples", "minimal-web"))

	listener, err := net.Listen("tcp4", "127.0.0.1:3000")
	if err != nil {
		t.Fatalf("listen on 3000: %v", err)
	}
	defer listener.Close()

	code, stdout, stderr := runCLI(t, []string{
		"prepare",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	renderedPayload, err := os.ReadFile(filepath.Join(repo, ".devlane", "generated", "app.env"))
	if err != nil {
		t.Fatalf("read rendered output: %v", err)
	}
	if !strings.Contains(string(renderedPayload), "PORT=3001") {
		t.Fatalf("expected generated app env to use allocated port:\n%s", renderedPayload)
	}
}

func TestPreparePrunesCatalogRowsWhenPortsBlockIsRemoved(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	sharedConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", sharedConfig)

	code, _, stderr := runCLI(t, []string{
		"prepare",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
	})
	if code != 0 {
		t.Fatalf("expected initial prepare to succeed, got %d\nstderr:\n%s", code, stderr)
	}
	assertCatalogAllocations(t, filepath.Join(sharedConfig, "devlane", "catalog.json"), 1)

	removePortsBlock(t, filepath.Join(repo, "devlane.yaml"))

	code, _, stderr = runCLI(t, []string{
		"prepare",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
	})
	if code != 0 {
		t.Fatalf("expected prepare after removing ports block to succeed, got %d\nstderr:\n%s", code, stderr)
	}
	assertCatalogAllocations(t, filepath.Join(sharedConfig, "devlane", "catalog.json"), 0)
}

func TestPrepareRejectsBoundStableFixture(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	sharedConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", sharedConfig)

	listener, err := net.Listen("tcp4", "127.0.0.1:3000")
	if err != nil {
		t.Fatalf("listen on 3000: %v", err)
	}
	defer listener.Close()

	code, _, stderr := runCLI(t, []string{
		"prepare",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
		"--mode", "stable",
	})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "stable port 3000") {
		t.Fatalf("expected stable fixture collision error, got stderr:\n%s", stderr)
	}
	assertCatalogAllocations(t, filepath.Join(sharedConfig, "devlane", "catalog.json"), 0)
}

func TestDoctorDoesNotFailEarlyOnBoundStableFixture(t *testing.T) {
	repo := testutil.InitDemoRepo(t)

	listener, err := net.Listen("tcp4", "127.0.0.1:3000")
	if err != nil {
		t.Fatalf("listen on 3000: %v", err)
	}
	defer listener.Close()

	code, stdout, stderr := runCLI(t, []string{
		"doctor",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
		"--mode", "stable",
	})
	if code != 0 && code != 1 {
		t.Fatalf("expected doctor exit code 0 or 1, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "ok: config exists at") {
		t.Fatalf("expected doctor diagnostics, got stdout:\n%s", stdout)
	}
	if strings.Contains(stderr, "stable port 3000") {
		t.Fatalf("doctor should not fail on port allocation, got stderr:\n%s", stderr)
	}
}

func TestDoctorFailsForManifestPathOutsideRepoRoot(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	mustWriteFile(t, filepath.Join(repo, "devlane.yaml"), `
schema: 1
app: demoapp
kind: cli

lane:
  stable_name: stable
  stable_branches: [main, master]
  project_pattern: "{app}_{lane}"
  path_roots:
    state: ".devlane/state"
    cache: ".devlane/cache"
    runtime: ".devlane/runtime"

outputs:
  manifest_path: "../outside.json"
  generated: []
`)

	code, stdout, stderr := runCLI(t, []string{
		"doctor",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
	})
	if code != 1 {
		t.Fatalf("expected doctor to fail, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("expected no doctor stdout on manifest validation failure, got:\n%s", stdout)
	}
	if !strings.Contains(stderr, "resolve outputs.manifest_path") {
		t.Fatalf("expected manifest path validation error, got stderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "repo root") {
		t.Fatalf("expected repo root escape detail, got stderr:\n%s", stderr)
	}
}

func TestPrepareDoesNotPublishCatalogWhenLocalWritesFail(t *testing.T) {
	repo := testutil.InitDemoRepo(t)
	sharedConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", sharedConfig)

	composeEnvPath := filepath.Join(repo, ".devlane", "compose.env")
	if err := os.MkdirAll(composeEnvPath, 0o755); err != nil {
		t.Fatalf("mkdir compose env path: %v", err)
	}

	code, _, stderr := runCLI(t, []string{
		"prepare",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
	})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "compose env target must be a file") {
		t.Fatalf("expected compose env write failure, got stderr:\n%s", stderr)
	}
	assertCatalogAllocations(t, filepath.Join(sharedConfig, "devlane", "catalog.json"), 0)
}

func TestStatusPrintsUnallocatedPortRowsForBareMetal(t *testing.T) {
	repo := initBareMetalRepo(t)

	code, stdout, stderr := runCLI(t, []string{
		"status",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Services:\n") {
		t.Fatalf("expected services section, got:\n%s", stdout)
	}
	row := serviceStatusRow(t, stdout, "web")
	if row.Status != "unallocated" {
		t.Fatalf("expected unallocated port row, got %#v in:\n%s", row, stdout)
	}
	if row.Port <= 0 {
		t.Fatalf("expected provisional port in unallocated row, got %#v", row)
	}
	if strings.Contains(stdout, "docker compose") {
		t.Fatalf("did not expect compose status output for bare-metal adapter, got:\n%s", stdout)
	}
}

func TestStatusPrintsFreePortRowsForPreparedBareMetal(t *testing.T) {
	repo := initBareMetalRepo(t)

	code, stdout, stderr := runCLI(t, []string{
		"prepare",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
	})
	if code != 0 {
		t.Fatalf("expected prepare exit code 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	allocatedPort := preparedPort(t, repo, "web")
	code, stdout, stderr = runCLI(t, []string{
		"status",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
	})
	if code != 0 {
		t.Fatalf("expected status exit code 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	row := serviceStatusRow(t, stdout, "web")
	if row.Port != allocatedPort || row.Status != "free" {
		t.Fatalf("expected free row for allocated port %d, got %#v in:\n%s", allocatedPort, row, stdout)
	}
	if strings.Contains(stdout, "docker compose") {
		t.Fatalf("did not expect compose status output for bare-metal adapter, got:\n%s", stdout)
	}
}

func TestStatusPrintsBoundPortRowsBeforeComposeStatus(t *testing.T) {
	repo := testutil.InitDemoRepo(t)

	code, stdout, stderr := runCLI(t, []string{
		"prepare",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
	})
	if code != 0 {
		t.Fatalf("expected prepare exit code 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	allocatedPort := preparedPort(t, repo, "web")
	listener, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", allocatedPort))
	if err != nil {
		t.Fatalf("listen on allocated port %d: %v", allocatedPort, err)
	}
	defer listener.Close()

	code, stdout, stderr = runCLI(t, []string{
		"status",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
		"--dry-run",
	})
	if code != 0 {
		t.Fatalf("expected status exit code 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	row := serviceStatusRow(t, stdout, "web")
	if row.Port != allocatedPort || row.Status != "bound" {
		t.Fatalf("expected bound row for allocated port %d, got %#v in:\n%s", allocatedPort, row, stdout)
	}
	servicesIndex := strings.Index(stdout, "Services:\n")
	composeIndex := strings.Index(stdout, "docker compose")
	if servicesIndex == -1 || composeIndex == -1 || servicesIndex > composeIndex {
		t.Fatalf("expected service rows before compose status command, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "-p demoapp_feature-test-lane") || !strings.Contains(stdout, " ps") {
		t.Fatalf("expected compose status command, got:\n%s", stdout)
	}
}

func TestStatusPrintsComposeStatusForContainerizedAdapter(t *testing.T) {
	repo := initComposeOnlyRepo(t)

	code, stdout, stderr := runCLI(t, []string{
		"status",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
		"--dry-run",
	})
	if code != 0 {
		t.Fatalf("expected status exit code 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if strings.Contains(stdout, "Services:\n") {
		t.Fatalf("did not expect service rows for compose-only adapter without ports, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "docker compose") || !strings.Contains(stdout, " ps") {
		t.Fatalf("expected compose ps command, got:\n%s", stdout)
	}
}

func TestStatusPrintsHybridPortRowsBeforeComposeStatus(t *testing.T) {
	repo := initExampleRepo(t, filepath.Join("examples", "hybrid-web"))

	code, stdout, stderr := runCLI(t, []string{
		"prepare",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
	})
	if code != 0 {
		t.Fatalf("expected prepare exit code 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	webPort := preparedPort(t, repo, "web")
	redisPort := preparedPort(t, repo, "redis")
	code, stdout, stderr = runCLI(t, []string{
		"status",
		"--cwd", repo,
		"--config", filepath.Join(repo, "devlane.yaml"),
		"--dry-run",
	})
	if code != 0 {
		t.Fatalf("expected status exit code 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	webRow := serviceStatusRow(t, stdout, "web")
	redisRow := serviceStatusRow(t, stdout, "redis")
	if webRow.Port != webPort || webRow.Status != "free" {
		t.Fatalf("expected free web row for port %d, got %#v in:\n%s", webPort, webRow, stdout)
	}
	if redisRow.Port != redisPort || redisRow.Status != "free" {
		t.Fatalf("expected free redis row for port %d, got %#v in:\n%s", redisPort, redisRow, stdout)
	}

	webIndex := strings.Index(stdout, "web")
	redisIndex := strings.Index(stdout, "redis")
	composeIndex := strings.Index(stdout, "docker compose")
	if webIndex == -1 || redisIndex == -1 || composeIndex == -1 || webIndex > redisIndex || redisIndex > composeIndex {
		t.Fatalf("expected adapter-order service rows before compose status command, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "-p hybridweb_feature-example-lane") || !strings.Contains(stdout, " ps") {
		t.Fatalf("expected hybrid compose status command, got:\n%s", stdout)
	}
}

func TestInitTemplateWritesValidAdapter(t *testing.T) {
	repo := t.TempDir()

	code, _, stderr := runCLIWithInput(t, []string{
		"init",
		"--cwd", repo,
		"--template", "containerized-web",
	}, "")
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d\nstderr:\n%s", code, stderr)
	}

	if _, err := os.Stat(filepath.Join(repo, "devlane.yaml")); err != nil {
		t.Fatalf("expected devlane.yaml to be written: %v", err)
	}
}

func TestInitMonorepoRequiresAllOrAppInNonTTYMode(t *testing.T) {
	repo := t.TempDir()
	mustWriteFile(t, filepath.Join(repo, "apps", "api", "package.json"), "{}\n")
	mustWriteFile(t, filepath.Join(repo, "apps", "web", "compose.yaml"), "services: {}\n")

	code, stdout, stderr := runCLIWithInput(t, []string{
		"init",
		"--cwd", repo,
	}, "")
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stdout, "Detected candidates:") {
		t.Fatalf("expected candidate list, got stdout:\n%s", stdout)
	}
	if !strings.Contains(stderr, "--all or --app") {
		t.Fatalf("expected no-guess guidance, got stderr:\n%s", stderr)
	}
}

func TestInitAllWritesEveryCandidate(t *testing.T) {
	repo := t.TempDir()
	mustWriteFile(t, filepath.Join(repo, "apps", "api", "package.json"), "{}\n")
	mustWriteFile(t, filepath.Join(repo, "apps", "web", "compose.yaml"), "services: {}\n")

	code, stdout, stderr := runCLIWithInput(t, []string{
		"init",
		"--cwd", repo,
		"--all",
	}, "")
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(repo, "apps", "api", "devlane.yaml")); err != nil {
		t.Fatalf("expected api devlane.yaml: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "apps", "web", "devlane.yaml")); err != nil {
		t.Fatalf("expected web devlane.yaml: %v", err)
	}
}

func TestInitAllSkipsNestedGitRepos(t *testing.T) {
	repo := t.TempDir()
	mustWriteFile(t, filepath.Join(repo, "apps", "api", "package.json"), "{}\n")

	nested := filepath.Join(repo, "tools", "nested-repo")
	mustWriteFile(t, filepath.Join(nested, "package.json"), "{}\n")
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = nested
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, output)
	}

	code, stdout, stderr := runCLIWithInput(t, []string{
		"init",
		"--cwd", repo,
		"--all",
	}, "")
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(repo, "apps", "api", "devlane.yaml")); err != nil {
		t.Fatalf("expected api devlane.yaml: %v", err)
	}
	if _, err := os.Stat(filepath.Join(nested, "devlane.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected nested repo to be skipped, stat err=%v", err)
	}
}

func TestInitFromCopiesLiteralAdapter(t *testing.T) {
	repo := t.TempDir()
	source := filepath.Join(repo, "source.yaml")
	sourcePayload := "schema: 1\napp: copied\nkind: cli\nlane:\n  stable_name: stable\n  stable_branches: [main]\n  project_pattern: \"{app}_{lane}\"\n  path_roots:\n    state: .devlane/state\n    cache: .devlane/cache\n    runtime: .devlane/runtime\noutputs:\n  manifest_path: .devlane/manifest.json\n  generated: []\n"
	mustWriteFile(t, source, sourcePayload)

	target := filepath.Join(repo, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}

	code, stdout, stderr := runCLIWithInput(t, []string{
		"init",
		"--cwd", repo,
		"--app", "target",
		"--from", source,
	}, "")
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	payload, err := os.ReadFile(filepath.Join(target, "devlane.yaml"))
	if err != nil {
		t.Fatalf("read copied adapter: %v", err)
	}
	if string(payload) != sourcePayload {
		t.Fatalf("expected literal copy, got:\n%s", payload)
	}
}

func runCLI(t *testing.T, args []string) (int, string, string) {
	return runCLIWithInput(t, args, "")
}

func runCLIWithInput(t *testing.T, args []string, input string) (int, string, string) {
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

	code := cli.Run(args)

	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	os.Stdout = originalStdout
	os.Stderr = originalStderr
	os.Stdin = originalStdin

	stdout := readPipe(t, stdoutReader)
	stderr := readPipe(t, stderrReader)
	return code, stdout, stderr
}

func readPipe(t *testing.T, reader *os.File) string {
	t.Helper()

	var buffer bytes.Buffer
	if _, err := buffer.ReadFrom(reader); err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return buffer.String()
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func initBareMetalRepo(t *testing.T) string {
	t.Helper()

	repo := testutil.InitDemoRepo(t)
	mustWriteFile(t, filepath.Join(repo, "devlane.yaml"), `
schema: 1
app: demoapp
kind: cli

lane:
  stable_name: stable
  stable_branches: [main, master]
  project_pattern: "{app}_{lane}"
  path_roots:
    state: ".devlane/state"
    cache: ".devlane/cache"
    runtime: ".devlane/runtime"
  host_patterns:
    stable: "{app}.localhost"
    dev: "{lane}.{app}.localhost"

runtime:
  run:
    commands:
      - name: web
        command: npm run dev

ports:
  - name: web
    default: 3000
    health_path: /health

outputs:
  manifest_path: ".devlane/manifest.json"
  generated: []
`)
	return repo
}

func initBareMetalGeneratedRepo(t *testing.T) string {
	t.Helper()

	repo := testutil.InitDemoRepo(t)
	mustWriteFile(t, filepath.Join(repo, "templates", "app.env.tmpl"), "DEVLANE_LANE={{lane.name}}\nPORT={{ports.web}}\n")
	mustWriteFile(t, filepath.Join(repo, "devlane.yaml"), `
schema: 1
app: demoapp
kind: cli

lane:
  stable_name: stable
  stable_branches: [main, master]
  project_pattern: "{app}_{lane}"
  path_roots:
    state: ".devlane/state"
    cache: ".devlane/cache"
    runtime: ".devlane/runtime"
  host_patterns:
    stable: "{app}.localhost"
    dev: "{lane}.{app}.localhost"

runtime:
  run:
    commands:
      - name: web
        command: npm run dev

ports:
  - name: web
    default: 3000
    health_path: /health

outputs:
  manifest_path: ".devlane/manifest.json"
  generated:
    - template: "templates/app.env.tmpl"
      destination: ".devlane/generated/app.env"
`)
	return repo
}

func initComposeOnlyRepo(t *testing.T) string {
	t.Helper()

	repo := testutil.InitDemoRepo(t)
	mustWriteFile(t, filepath.Join(repo, "devlane.yaml"), `
schema: 1
app: demoapp
kind: web

lane:
  stable_name: stable
  stable_branches: [main, master]
  project_pattern: "{app}_{lane}"
  path_roots:
    state: ".devlane/state"
    cache: ".devlane/cache"
    runtime: ".devlane/runtime"
  host_patterns:
    stable: "{app}.localhost"
    dev: "{lane}.{app}.localhost"

runtime:
  compose_files:
    - compose.yaml
    - compose.devlane.yaml
  default_profiles: [web]

outputs:
  manifest_path: ".devlane/manifest.json"
  compose_env_path: ".devlane/compose.env"
  generated: []
`)
	return repo
}

func initPortOnlyRepo(t *testing.T) string {
	t.Helper()

	repo := testutil.InitDemoRepo(t)
	mustWriteFile(t, filepath.Join(repo, "devlane.yaml"), `
schema: 1
app: demoapp
kind: cli

lane:
  stable_name: stable
  stable_branches: [main, master]
  project_pattern: "{app}_{lane}"
  path_roots:
    state: ".devlane/state"
    cache: ".devlane/cache"
    runtime: ".devlane/runtime"
  host_patterns:
    stable: "{app}.localhost"
    dev: "{lane}.{app}.localhost"

ports:
  - name: web
    default: 3000
    health_path: /health

outputs:
  manifest_path: ".devlane/manifest.json"
  generated: []
`)
	return repo
}

func initExampleRepo(t *testing.T, relativePath string) string {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	source := filepath.Clean(filepath.Join(cwd, "..", "..", relativePath))
	repo := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(repo, ".xdg"))

	if err := copyDir(source, repo); err != nil {
		t.Fatalf("copy example repo: %v", err)
	}

	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = repo
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, output)
	}

	for _, args := range [][]string{
		{"config", "user.name", "Devlane Tests"},
		{"config", "user.email", "tests@example.com"},
		{"add", "."},
		{"commit", "-m", "initial"},
		{"checkout", "-b", "feature/example-lane"},
	} {
		cmd = exec.Command("git", args...)
		cmd.Dir = repo
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, output)
		}
	}

	return repo
}

func copyDir(source, destination string) error {
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, payload, 0o644)
	})
}

type statusRow struct {
	Service string
	Port    int
	Status  string
}

func serviceStatusRow(t *testing.T, stdout, service string) statusRow {
	t.Helper()

	for _, line := range strings.Split(stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 4 || fields[0] != service || fields[1] != "port" {
			continue
		}

		port, err := strconv.Atoi(fields[2])
		if err != nil {
			t.Fatalf("parse status port from line %q: %v", line, err)
		}

		return statusRow{
			Service: fields[0],
			Port:    port,
			Status:  fields[3],
		}
	}

	t.Fatalf("missing status row for service %q in:\n%s", service, stdout)
	return statusRow{}
}

func preparedPort(t *testing.T, repo, service string) int {
	t.Helper()

	payload, err := os.ReadFile(filepath.Join(repo, ".devlane", "manifest.json"))
	if err != nil {
		t.Fatalf("read prepared manifest: %v", err)
	}

	var prepared struct {
		Ports map[string]struct {
			Port      int  `json:"port"`
			Allocated bool `json:"allocated"`
		} `json:"ports"`
	}
	if err := json.Unmarshal(payload, &prepared); err != nil {
		t.Fatalf("decode prepared manifest: %v\n%s", err, payload)
	}

	portState, ok := prepared.Ports[service]
	if !ok {
		t.Fatalf("prepared manifest is missing service %q: %s", service, payload)
	}
	if !portState.Allocated {
		t.Fatalf("prepared manifest service %q is not allocated: %#v", service, portState)
	}
	if portState.Port <= 0 {
		t.Fatalf("prepared manifest service %q has invalid port: %#v", service, portState)
	}

	return portState.Port
}

func removePortsBlock(t *testing.T, configPath string) {
	t.Helper()

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

	updated := content[:start] + content[end:]
	if err := os.WriteFile(configPath, []byte(updated), 0o644); err != nil {
		t.Fatalf("write adapter: %v", err)
	}
}

func assertCatalogAllocations(t *testing.T, path string, want int) {
	t.Helper()

	payload, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if want == 0 {
			return
		}
		t.Fatalf("expected catalog at %s", path)
	}
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}

	var stored struct {
		Allocations []any `json:"allocations"`
	}
	if err := json.Unmarshal(payload, &stored); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	if len(stored.Allocations) != want {
		t.Fatalf("expected %d catalog allocations, got %d", want, len(stored.Allocations))
	}
}
