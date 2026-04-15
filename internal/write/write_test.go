package write_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/auro/devlane/internal/config"
	"github.com/auro/devlane/internal/manifest"
	"github.com/auro/devlane/internal/testutil"
	"github.com/auro/devlane/internal/write"
)

func TestPrepareWritesManifestAndRenderedFiles(t *testing.T) {
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

	if err := write.Manifest(laneManifest); err != nil {
		t.Fatalf("Manifest returned error: %v", err)
	}
	if err := write.ComposeEnv(laneManifest, adapter); err != nil {
		t.Fatalf("ComposeEnv returned error: %v", err)
	}
	if err := write.Outputs(laneManifest, adapter); err != nil {
		t.Fatalf("Outputs returned error: %v", err)
	}

	manifestPayload, err := os.ReadFile(laneManifest.Paths.Manifest)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	var stored manifest.Manifest
	if err := json.Unmarshal(manifestPayload, &stored); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}

	if stored.Network.PublicHost == nil || *stored.Network.PublicHost != "feature-test-lane.demoapp.localhost" {
		t.Fatalf("unexpected stored public host: %#v", stored.Network.PublicHost)
	}

	composeEnvPayload, err := os.ReadFile(*laneManifest.Paths.ComposeEnv)
	if err != nil {
		t.Fatalf("read compose env: %v", err)
	}
	if !strings.Contains(string(composeEnvPayload), "DEVLANE_PUBLIC_HOST=feature-test-lane.demoapp.localhost") {
		t.Fatalf("compose env missing expected host:\n%s", composeEnvPayload)
	}

	renderedPath := filepath.Join(repo, ".devlane", "generated", "app.env")
	renderedPayload, err := os.ReadFile(renderedPath)
	if err != nil {
		t.Fatalf("read rendered output: %v", err)
	}
	if !strings.Contains(string(renderedPayload), "DEVLANE_LANE=feature/test-lane") {
		t.Fatalf("rendered output missing lane name:\n%s", renderedPayload)
	}
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
		Compose: manifest.Compose{
			Files:    []string{},
			Profiles: []string{},
		},
		Outputs: manifest.Outputs{
			Generated: []manifest.GeneratedOutput{},
		},
		Ports: map[string]manifest.Port{},
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

	if network["publicHost"] != "demoapp.localhost" {
		t.Fatalf("unexpected publicHost: %#v", network["publicHost"])
	}
	if lane["slug"] != "stable" {
		t.Fatalf("unexpected lane slug: %#v", lane["slug"])
	}
	if env["DEVLANE_APP"] != "demoapp" {
		t.Fatalf("unexpected DEVLANE_APP: %q", env["DEVLANE_APP"])
	}
}
