package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func InitDemoRepo(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(repo, ".xdg"))
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
  - name: web
    default: 3000
    health_path: /health

outputs:
  manifest_path: ".devlane/manifest.json"
  compose_env_path: ".devlane/compose.env"
  generated:
    - template: "templates/app.env.tmpl"
      destination: ".devlane/generated/app.env"
`)
	mustWriteFile(t, filepath.Join(repo, "templates", "app.env.tmpl"), "APP_MODE={{env.APP_MODE}}\nDEVLANE_LANE={{lane.name}}\nDEVLANE_PUBLIC_URL={{network.publicUrl}}\n")
	mustWriteFile(t, filepath.Join(repo, "compose.yaml"), "services: {}\n")
	mustWriteFile(t, filepath.Join(repo, "compose.devlane.yaml"), "services: {}\n")

	run(t, repo, "git", "init", "-b", "main")
	run(t, repo, "git", "config", "user.name", "Devlane Tests")
	run(t, repo, "git", "config", "user.email", "tests@example.com")
	run(t, repo, "git", "add", ".")
	run(t, repo, "git", "commit", "-m", "initial")
	run(t, repo, "git", "checkout", "-b", "feature/test-lane")

	return repo
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

func run(t *testing.T, cwd string, name string, args ...string) {
	t.Helper()

	cmd := exec.Command(name, args...)
	cmd.Dir = cwd
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, output)
	}
}
