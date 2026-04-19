package doctor_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/auro/devlane/internal/config"
	"github.com/auro/devlane/internal/doctor"
)

func TestRunFailsWhenDockerComposeSubcommandIsUnavailable(t *testing.T) {
	root := t.TempDir()
	writeExecutable(t, filepath.Join(root, "docker"), "#!/bin/sh\nif [ \"$1\" = \"compose\" ] && [ \"$2\" = \"version\" ]; then\n  echo 'docker: \"compose\" is not a docker command.' >&2\n  exit 1\nfi\nexit 0\n")
	t.Setenv("PATH", root)

	composeFile := filepath.Join(root, "compose.yaml")
	mustWriteFile(t, composeFile, "services: {}\n")

	result := doctor.Run(composeAdapter(), []string{composeFile}, filepath.Join(root, "devlane.yaml"))
	if !result.Failed {
		t.Fatal("expected doctor to fail")
	}

	joined := strings.Join(result.Messages, "\n")
	if !strings.Contains(joined, `fail: docker compose unavailable: docker: "compose" is not a docker command.`) {
		t.Fatalf("expected compose-subcommand failure, got:\n%s", joined)
	}
}

func TestRunPassesWhenDockerComposeIsAvailable(t *testing.T) {
	root := t.TempDir()
	writeExecutable(t, filepath.Join(root, "docker"), "#!/bin/sh\nif [ \"$1\" = \"compose\" ] && [ \"$2\" = \"version\" ]; then\n  echo 'Docker Compose version v2.0.0'\n  exit 0\nfi\nexit 0\n")
	t.Setenv("PATH", root)

	composeFile := filepath.Join(root, "compose.yaml")
	mustWriteFile(t, composeFile, "services: {}\n")

	result := doctor.Run(composeAdapter(), []string{composeFile}, filepath.Join(root, "devlane.yaml"))
	if result.Failed {
		t.Fatalf("expected doctor to pass, got messages:\n%s", strings.Join(result.Messages, "\n"))
	}

	joined := strings.Join(result.Messages, "\n")
	if !strings.Contains(joined, "ok: docker compose available") {
		t.Fatalf("expected compose success message, got:\n%s", joined)
	}
}

func composeAdapter() *config.AdapterConfig {
	return &config.AdapterConfig{
		Runtime: config.RuntimeConfig{
			ComposeFiles: []string{"compose.yaml"},
		},
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
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
