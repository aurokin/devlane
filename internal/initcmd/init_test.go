package initcmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/auro/devlane/internal/config"
	"github.com/auro/devlane/internal/initcmd"
)

func TestScanWalksLexicallyAndSkipsTreesAndSymlinks(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "b-app", "package.json"), "{}\n")
	mustWriteFile(t, filepath.Join(root, "a-app", "compose.yaml"), "services: {}\n")
	mustWriteFile(t, filepath.Join(root, "node_modules", "ignored", "package.json"), "{}\n")

	linkTarget := t.TempDir()
	mustWriteFile(t, filepath.Join(linkTarget, "package.json"), "{}\n")
	if err := os.Symlink(linkTarget, filepath.Join(root, "symlink-app")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	candidates, err := initcmd.Scan(root)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if filepath.Base(candidates[0].Path) != "a-app" {
		t.Fatalf("expected lexical ordering, got first candidate %s", candidates[0].Path)
	}
	if filepath.Base(candidates[1].Path) != "b-app" {
		t.Fatalf("expected lexical ordering, got second candidate %s", candidates[1].Path)
	}
}

func TestScanSkipsNestedGitRepos(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "apps", "api", "package.json"), "{}\n")
	mustWriteFile(t, filepath.Join(root, "vendor", "ignored", "package.json"), "{}\n")

	nested := filepath.Join(root, "tools", "nested-repo")
	mustWriteFile(t, filepath.Join(nested, "package.json"), "{}\n")
	mustWriteFile(t, filepath.Join(nested, "apps", "compose.yaml"), "services: {}\n")
	runGit(t, nested, "init", "-b", "main")

	candidates, err := initcmd.Scan(root)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}

	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d: %#v", len(candidates), candidates)
	}
	if got := filepath.Base(candidates[0].Path); got != "api" {
		t.Fatalf("expected non-nested app candidate, got %s", candidates[0].Path)
	}
}

func TestClassifyPathFallsBackToCLIWhenNoSignalsExist(t *testing.T) {
	dir := t.TempDir()

	candidate, err := initcmd.ClassifyPath(dir)
	if err != nil {
		t.Fatalf("ClassifyPath returned error: %v", err)
	}
	if candidate.Template != initcmd.TemplateCLI {
		t.Fatalf("expected cli fallback, got %s", candidate.Template)
	}
	if !candidate.FallbackToCLI {
		t.Fatal("expected cli fallback marker")
	}
}

func TestClassifyPathDetectsDockerComposeYMLAsContainerized(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "docker-compose.yml"), "services: {}\n")

	candidate, err := initcmd.ClassifyPath(dir)
	if err != nil {
		t.Fatalf("ClassifyPath returned error: %v", err)
	}
	if candidate.Kind != initcmd.DetectionContainerized {
		t.Fatalf("expected containerized detection, got %s", candidate.Kind)
	}
	if candidate.Template != initcmd.TemplateContainerizedWeb {
		t.Fatalf("expected containerized template, got %s", candidate.Template)
	}
	if !slices.Equal(candidate.ComposeFiles, []string{"docker-compose.yml"}) {
		t.Fatalf("expected docker-compose.yml to be preserved, got %v", candidate.ComposeFiles)
	}
}

func TestClassifyPathIgnoresOverrideOnlyComposeFiles(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "docker-compose.override.yml"), "services: {}\n")

	candidate, err := initcmd.ClassifyPath(dir)
	if err != nil {
		t.Fatalf("ClassifyPath returned error: %v", err)
	}
	if candidate.Template != initcmd.TemplateCLI {
		t.Fatalf("expected cli fallback for override-only compose file, got %s", candidate.Template)
	}
	if len(candidate.ComposeFiles) != 0 {
		t.Fatalf("expected override-only compose file to be excluded, got %v", candidate.ComposeFiles)
	}
}

func TestClassifyPathMarksMixedSignalsAsAmbiguous(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "docker-compose.yml"), "services: {}\n")
	mustWriteFile(t, filepath.Join(dir, "package.json"), "{}\n")

	candidate, err := initcmd.ClassifyPath(dir)
	if err != nil {
		t.Fatalf("ClassifyPath returned error: %v", err)
	}
	if candidate.Kind != initcmd.DetectionAmbiguous {
		t.Fatalf("expected ambiguous detection, got %s", candidate.Kind)
	}
	if !candidate.HybridHint {
		t.Fatal("expected hybrid hint")
	}
	if candidate.Template != initcmd.TemplateCLI {
		t.Fatalf("expected cli fallback for ambiguous detection, got %s", candidate.Template)
	}
	if !slices.Equal(candidate.ComposeFiles, []string{"docker-compose.yml"}) {
		t.Fatalf("expected docker-compose.yml compose signal to be preserved, got %v", candidate.ComposeFiles)
	}
}

func TestScaffoldTemplatePreservesDetectedComposeFiles(t *testing.T) {
	t.Run("single detected compose file", func(t *testing.T) {
		payload, err := initcmd.ScaffoldTemplate(initcmd.TemplateContainerizedWeb, "Demo App", []string{"compose.dev.yaml"})
		if err != nil {
			t.Fatalf("ScaffoldTemplate returned error: %v", err)
		}
		if !strings.Contains(payload, "compose_files:\n    - compose.dev.yaml\n") {
			t.Fatalf("expected scaffold to preserve detected compose filename:\n%s", payload)
		}
		if strings.Contains(payload, "compose_files:\n    - compose.yaml\n") {
			t.Fatalf("expected scaffold to avoid default compose.yaml when a filename was detected:\n%s", payload)
		}
	})

	t.Run("multiple detected compose files", func(t *testing.T) {
		payload, err := initcmd.ScaffoldTemplate(initcmd.TemplateHybridWeb, "Demo App", []string{"compose.yml", "docker-compose.override.yml"})
		if err != nil {
			t.Fatalf("ScaffoldTemplate returned error: %v", err)
		}
		expected := "compose_files:\n    - compose.yml\n    - docker-compose.override.yml\n"
		if !strings.Contains(payload, expected) {
			t.Fatalf("expected scaffold to preserve detected compose filenames:\n%s", payload)
		}
	})
}

func TestScaffoldTemplateProducesSchemaValidAdapters(t *testing.T) {
	templates := []initcmd.TemplateName{
		initcmd.TemplateContainerizedWeb,
		initcmd.TemplateBaremetalWeb,
		initcmd.TemplateHybridWeb,
		initcmd.TemplateCLI,
	}

	for _, templateName := range templates {
		t.Run(string(templateName), func(t *testing.T) {
			repo := t.TempDir()
			payload, err := initcmd.ScaffoldTemplate(templateName, "Demo App", nil)
			if err != nil {
				t.Fatalf("ScaffoldTemplate returned error: %v", err)
			}

			configPath := filepath.Join(repo, "devlane.yaml")
			if err := os.WriteFile(configPath, []byte(payload), 0o644); err != nil {
				t.Fatalf("write scaffolded adapter: %v", err)
			}

			adapter, err := config.LoadAdapter(configPath)
			if err != nil {
				t.Fatalf("LoadAdapter returned error: %v", err)
			}
			if !strings.Contains(payload, "# host_patterns") {
				t.Fatalf("expected host_patterns scaffold comment in template:\n%s", payload)
			}
			if !strings.Contains(payload, "# worktree:") {
				t.Fatalf("expected worktree scaffold comment in template:\n%s", payload)
			}
			if adapter.Outputs.ManifestPath == "" {
				t.Fatal("expected manifest path in scaffolded adapter")
			}
		})
	}
}

func TestExecuteRejectsAppPathsOutsideCWD(t *testing.T) {
	root := t.TempDir()
	sibling := filepath.Join(filepath.Dir(root), "sibling")
	mustMkdir(t, sibling)

	t.Run("rejects parent traversal", func(t *testing.T) {
		_, err := initcmd.Execute(initcmd.Options{
			CWD:      root,
			AppPath:  "../sibling",
			Template: string(initcmd.TemplateCLI),
			Yes:      true,
		})
		if err == nil {
			t.Fatal("expected escape error")
		}
		if !strings.Contains(err.Error(), "app path escapes --cwd") {
			t.Fatalf("expected escape error, got %v", err)
		}
	})

	t.Run("rejects absolute paths", func(t *testing.T) {
		_, err := initcmd.Execute(initcmd.Options{
			CWD:      root,
			AppPath:  sibling,
			Template: string(initcmd.TemplateCLI),
			Yes:      true,
		})
		if err == nil {
			t.Fatal("expected absolute-path error")
		}
		if !strings.Contains(err.Error(), "app path must be relative to --cwd") {
			t.Fatalf("expected absolute-path error, got %v", err)
		}
	})

	t.Run("rejects symlink escapes", func(t *testing.T) {
		external := t.TempDir()
		mustWriteFile(t, filepath.Join(external, "package.json"), "{}\n")

		if err := os.Symlink(external, filepath.Join(root, "linked-app")); err != nil {
			t.Skipf("symlink unsupported: %v", err)
		}

		_, err := initcmd.Execute(initcmd.Options{
			CWD:      root,
			AppPath:  "linked-app",
			Template: string(initcmd.TemplateCLI),
			Yes:      true,
		})
		if err == nil {
			t.Fatal("expected symlink escape error")
		}
		if !strings.Contains(err.Error(), "app path escapes --cwd via symlink") {
			t.Fatalf("expected symlink escape error, got %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(external, "devlane.yaml")); !os.IsNotExist(statErr) {
			t.Fatalf("expected external devlane.yaml to remain absent, stat err=%v", statErr)
		}
	})
}

func TestExecuteTemplatePreservesDetectedComposeFiles(t *testing.T) {
	t.Run("hybrid override keeps detected compose files", func(t *testing.T) {
		root := t.TempDir()
		mustWriteFile(t, filepath.Join(root, "docker-compose.yml"), "services: {}\n")
		mustWriteFile(t, filepath.Join(root, "docker-compose.override.yml"), "services: {}\n")
		mustWriteFile(t, filepath.Join(root, "package.json"), "{}\n")

		if _, err := initcmd.Execute(initcmd.Options{
			CWD:      root,
			AppPath:  ".",
			Template: string(initcmd.TemplateHybridWeb),
			Yes:      true,
		}); err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}

		payload := mustReadFile(t, filepath.Join(root, "devlane.yaml"))
		expected := "compose_files:\n    - docker-compose.yml\n    - docker-compose.override.yml\n"
		if !strings.Contains(payload, expected) {
			t.Fatalf("expected written scaffold to preserve detected compose filename:\n%s", payload)
		}
		if strings.Contains(payload, "compose_files:\n    - compose.yaml\n") {
			t.Fatalf("expected written scaffold to avoid default compose.yaml:\n%s", payload)
		}
	})

	t.Run("containerized override keeps detected compose files", func(t *testing.T) {
		root := t.TempDir()
		mustWriteFile(t, filepath.Join(root, "compose.dev.yaml"), "services: {}\n")
		mustWriteFile(t, filepath.Join(root, "compose.override.yaml"), "services: {}\n")

		if _, err := initcmd.Execute(initcmd.Options{
			CWD:      root,
			AppPath:  ".",
			Template: string(initcmd.TemplateContainerizedWeb),
			Yes:      true,
		}); err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}

		payload := mustReadFile(t, filepath.Join(root, "devlane.yaml"))
		if !strings.Contains(payload, "compose_files:\n    - compose.dev.yaml\n    - compose.override.yaml\n") {
			t.Fatalf("expected written scaffold to preserve detected compose filename:\n%s", payload)
		}
	})
}

func TestExecuteTemplateIgnoresOverrideOnlyComposeFiles(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "compose.override.yaml"), "services: {}\n")

	if _, err := initcmd.Execute(initcmd.Options{
		CWD:      root,
		AppPath:  ".",
		Template: string(initcmd.TemplateContainerizedWeb),
		Yes:      true,
	}); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	payload := mustReadFile(t, filepath.Join(root, "devlane.yaml"))
	if strings.Contains(payload, "compose.override.yaml") {
		t.Fatalf("expected override-only compose file to be excluded from scaffold:\n%s", payload)
	}
	if !strings.Contains(payload, "compose_files:\n    - compose.yaml\n") {
		t.Fatalf("expected scaffold to fall back to compose.yaml when only override files exist:\n%s", payload)
	}
}

func TestExecuteTemplateWithoutAppSelectsDetectedDescendantRoot(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "apps", "web")
	mustWriteFile(t, filepath.Join(app, "package.json"), "{}\n")

	result, err := initcmd.Execute(initcmd.Options{
		CWD:      root,
		Template: string(initcmd.TemplateBaremetalWeb),
		Yes:      true,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(root, "devlane.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("expected root devlane.yaml to remain absent, stat err=%v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(app, "devlane.yaml")); statErr != nil {
		t.Fatalf("expected descendant devlane.yaml to be written: %v", statErr)
	}

	joined := strings.Join(result.Messages, "\n")
	if !strings.Contains(joined, "Selected app root: apps/web") {
		t.Fatalf("expected selected app root message, got:\n%s", joined)
	}
}

func TestExecuteFromWithoutAppSelectsDetectedDescendantRoot(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "apps", "web")
	mustWriteFile(t, filepath.Join(app, "package.json"), "{}\n")

	sourcePayload, err := initcmd.ScaffoldTemplate(initcmd.TemplateCLI, "Source App", nil)
	if err != nil {
		t.Fatalf("ScaffoldTemplate returned error: %v", err)
	}
	sourcePath := filepath.Join(root, "source.yaml")
	mustWriteFile(t, sourcePath, sourcePayload)

	result, err := initcmd.Execute(initcmd.Options{
		CWD:  root,
		From: "source.yaml",
		Yes:  true,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(root, "devlane.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("expected root devlane.yaml to remain absent, stat err=%v", statErr)
	}
	written := mustReadFile(t, filepath.Join(app, "devlane.yaml"))
	if written != sourcePayload {
		t.Fatalf("expected copied adapter to match source payload")
	}

	joined := strings.Join(result.Messages, "\n")
	if !strings.Contains(joined, filepath.Join(app, "devlane.yaml")) {
		t.Fatalf("expected descendant write message, got:\n%s", joined)
	}
}

func TestExecuteTemplateWithoutAppFailsInsteadOfGuessingOnMultipleCandidates(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "app-one", "package.json"), "{}\n")
	mustWriteFile(t, filepath.Join(root, "app-two", "go.mod"), "module example.com/two\n")

	_, err := initcmd.Execute(initcmd.Options{
		CWD:      root,
		Template: string(initcmd.TemplateBaremetalWeb),
		Yes:      true,
	})
	if err == nil {
		t.Fatal("expected multiple-candidates error")
	}
	if !strings.Contains(err.Error(), "multiple candidates found; rerun with --all or --app <path>") {
		t.Fatalf("expected multiple-candidates guidance, got %v", err)
	}
}

func TestExecuteFromWithoutAppFailsInsteadOfGuessingOnMultipleCandidates(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "app-one", "package.json"), "{}\n")
	mustWriteFile(t, filepath.Join(root, "app-two", "go.mod"), "module example.com/two\n")

	sourcePayload, err := initcmd.ScaffoldTemplate(initcmd.TemplateCLI, "Source App", nil)
	if err != nil {
		t.Fatalf("ScaffoldTemplate returned error: %v", err)
	}
	mustWriteFile(t, filepath.Join(root, "source.yaml"), sourcePayload)

	_, err = initcmd.Execute(initcmd.Options{
		CWD:  root,
		From: "source.yaml",
		Yes:  true,
	})
	if err == nil {
		t.Fatal("expected multiple-candidates error")
	}
	if !strings.Contains(err.Error(), "multiple candidates found; rerun with --all or --app <path>") {
		t.Fatalf("expected multiple-candidates guidance, got %v", err)
	}
}

func TestTemplateOverrideClearsFallbackHintsInMessages(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docker-compose.yml"), "services: {}\n")
	mustWriteFile(t, filepath.Join(root, "package.json"), "{}\n")

	result, err := initcmd.Execute(initcmd.Options{
		CWD:      root,
		AppPath:  ".",
		Template: string(initcmd.TemplateHybridWeb),
		Yes:      true,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	joined := strings.Join(result.Messages, "\n")
	if strings.Contains(joined, "wrote the cli template") {
		t.Fatalf("expected cli fallback notice to be cleared, got:\n%s", joined)
	}
	if strings.Contains(joined, "overlapping compose + bare-metal signals") {
		t.Fatalf("expected hybrid fallback notice to be cleared, got:\n%s", joined)
	}
	if !strings.Contains(joined, "Using template override: hybrid-web") {
		t.Fatalf("expected override reason in messages, got:\n%s", joined)
	}
}

func TestExecuteFromResolvesRelativePathAgainstCWD(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	mustMkdir(t, target)

	sourcePayload, err := initcmd.ScaffoldTemplate(initcmd.TemplateCLI, "Source App", nil)
	if err != nil {
		t.Fatalf("ScaffoldTemplate returned error: %v", err)
	}
	sourcePath := filepath.Join(root, "source.yaml")
	mustWriteFile(t, sourcePath, sourcePayload)

	otherWD := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	if err := os.Chdir(otherWD); err != nil {
		t.Fatalf("Chdir returned error: %v", err)
	}
	defer func() {
		if err := os.Chdir(originalWD); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()

	if _, err := initcmd.Execute(initcmd.Options{
		CWD:     root,
		AppPath: "target",
		From:    "source.yaml",
		Yes:     true,
	}); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	written := mustReadFile(t, filepath.Join(target, "devlane.yaml"))
	if written != sourcePayload {
		t.Fatalf("expected copied adapter to match source payload")
	}
}

func TestExecuteTemplateAllWritesEveryDetectedCandidate(t *testing.T) {
	root := t.TempDir()
	appOne := filepath.Join(root, "app-one")
	appTwo := filepath.Join(root, "app-two")
	mustWriteFile(t, filepath.Join(appOne, "compose.dev.yaml"), "services: {}\n")
	mustWriteFile(t, filepath.Join(appTwo, "docker-compose.yml"), "services: {}\n")

	if _, err := initcmd.Execute(initcmd.Options{
		CWD:      root,
		All:      true,
		Template: string(initcmd.TemplateHybridWeb),
		Yes:      true,
	}); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	one := mustReadFile(t, filepath.Join(appOne, "devlane.yaml"))
	if !strings.Contains(one, "compose_files:\n    - compose.dev.yaml\n") {
		t.Fatalf("expected first scaffold to preserve detected compose file:\n%s", one)
	}

	two := mustReadFile(t, filepath.Join(appTwo, "devlane.yaml"))
	if !strings.Contains(two, "compose_files:\n    - docker-compose.yml\n") {
		t.Fatalf("expected second scaffold to preserve detected compose file:\n%s", two)
	}

	if _, err := os.Stat(filepath.Join(root, "devlane.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected root devlane.yaml to remain absent, stat err=%v", err)
	}
}

func TestExecuteFromAllWritesEveryDetectedCandidate(t *testing.T) {
	root := t.TempDir()
	appOne := filepath.Join(root, "app-one")
	appTwo := filepath.Join(root, "app-two")
	mustWriteFile(t, filepath.Join(appOne, "package.json"), "{}\n")
	mustWriteFile(t, filepath.Join(appTwo, "go.mod"), "module example.com/two\n")

	sourcePayload, err := initcmd.ScaffoldTemplate(initcmd.TemplateCLI, "Source App", nil)
	if err != nil {
		t.Fatalf("ScaffoldTemplate returned error: %v", err)
	}
	sourcePath := filepath.Join(root, "source.yaml")
	mustWriteFile(t, sourcePath, sourcePayload)

	if _, err := initcmd.Execute(initcmd.Options{
		CWD:  root,
		All:  true,
		From: "source.yaml",
		Yes:  true,
	}); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	for _, target := range []string{appOne, appTwo} {
		written := mustReadFile(t, filepath.Join(target, "devlane.yaml"))
		if written != sourcePayload {
			t.Fatalf("expected copied adapter to match source payload for %s", target)
		}
	}
}

func TestExecuteFromRejectsRepoEscapingCopiedPaths(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")

	target := filepath.Join(root, "target")
	mustMkdir(t, target)

	sourcePath := filepath.Join(root, "source.yaml")
	mustWriteFile(t, sourcePath, `schema: 1
app: copied
kind: cli
lane:
  stable_name: stable
  stable_branches: [main]
  project_pattern: "{app}_{lane}"
  path_roots:
    state: ".devlane/state"
    cache: ".devlane/cache"
    runtime: ".devlane/runtime"
outputs:
  manifest_path: ".devlane/manifest.json"
  generated:
    - template: "../../shared/app.env.tmpl"
      destination: ".devlane/generated/app.env"
`)

	_, err := initcmd.Execute(initcmd.Options{
		CWD:     root,
		AppPath: "target",
		From:    "source.yaml",
		Yes:     true,
	})
	if err == nil {
		t.Fatal("expected copied path validation error")
	}
	if !strings.Contains(err.Error(), `outputs.generated[].template "../../shared/app.env.tmpl"`) {
		t.Fatalf("expected template path validation error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(target, "devlane.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no devlane.yaml to be written, stat err=%v", statErr)
	}
}

func TestExecuteFromRejectsSymlinkEscapingCopiedPaths(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")

	target := filepath.Join(root, "target")
	external := t.TempDir()
	mustMkdir(t, target)
	mustWriteFile(t, filepath.Join(external, "app.env.tmpl"), "APP={{app}}\n")
	if err := os.Symlink(external, filepath.Join(target, "linked")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	sourcePath := filepath.Join(root, "source.yaml")
	mustWriteFile(t, sourcePath, `schema: 1
app: copied
kind: cli
lane:
  stable_name: stable
  stable_branches: [main]
  project_pattern: "{app}_{lane}"
  path_roots:
    state: ".devlane/state"
    cache: ".devlane/cache"
    runtime: ".devlane/runtime"
outputs:
  manifest_path: ".devlane/manifest.json"
  generated:
    - template: "linked/app.env.tmpl"
      destination: ".devlane/generated/app.env"
`)

	_, err := initcmd.Execute(initcmd.Options{
		CWD:     root,
		AppPath: "target",
		From:    "source.yaml",
		Yes:     true,
	})
	if err == nil {
		t.Fatal("expected copied path validation error")
	}
	if !strings.Contains(err.Error(), `outputs.generated[].template "linked/app.env.tmpl"`) {
		t.Fatalf("expected template path validation error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(target, "devlane.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no devlane.yaml to be written, stat err=%v", statErr)
	}
}

func TestExecuteFromWarnsOnMissingInRepoInputs(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")

	target := filepath.Join(root, "target")
	mustMkdir(t, target)

	sourcePath := filepath.Join(root, "source.yaml")
	mustWriteFile(t, sourcePath, `schema: 1
app: copied
kind: cli
lane:
  stable_name: stable
  stable_branches: [main]
  project_pattern: "{app}_{lane}"
  path_roots:
    state: ".devlane/state"
    cache: ".devlane/cache"
    runtime: ".devlane/runtime"
worktree:
  seed:
    - ".env.local"
outputs:
  manifest_path: ".devlane/manifest.json"
  generated:
    - template: "templates/app.env.tmpl"
      destination: ".devlane/generated/app.env"
`)

	result, err := initcmd.Execute(initcmd.Options{
		CWD:     root,
		AppPath: "target",
		From:    "source.yaml",
		Yes:     true,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(target, "devlane.yaml")); statErr != nil {
		t.Fatalf("expected devlane.yaml to be written: %v", statErr)
	}
	joined := strings.Join(result.Messages, "\n")
	if !strings.Contains(joined, "warning: copied adapter references missing template in target repo: templates/app.env.tmpl") {
		t.Fatalf("expected missing template warning, got:\n%s", joined)
	}
	if !strings.Contains(joined, "warning: copied adapter references missing worktree seed path in target repo: .env.local") {
		t.Fatalf("expected missing seed warning, got:\n%s", joined)
	}
}

func TestExecuteForceRejectsSymlinkedExistingConfig(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	mustMkdir(t, target)

	external := filepath.Join(root, "external.yaml")
	mustWriteFile(t, external, "original\n")

	configPath := filepath.Join(target, "devlane.yaml")
	if err := os.Symlink(external, configPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	_, err := initcmd.Execute(initcmd.Options{
		CWD:      root,
		AppPath:  "target",
		Template: string(initcmd.TemplateCLI),
		Yes:      true,
		Force:    true,
	})
	if err == nil {
		t.Fatal("expected non-regular overwrite error")
	}
	if !strings.Contains(err.Error(), "refusing to overwrite non-regular existing") {
		t.Fatalf("expected non-regular overwrite error, got %v", err)
	}

	if got := mustReadFile(t, external); got != "original\n" {
		t.Fatalf("expected symlink target to remain unchanged, got %q", got)
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

func mustMkdir(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()

	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(payload)
}

func runGit(t *testing.T, cwd string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}
