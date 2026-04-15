package write

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/auro/devlane/internal/config"
	"github.com/auro/devlane/internal/manifest"
	"github.com/auro/devlane/internal/render"
	"github.com/auro/devlane/internal/util"
)

type RenderedRunCommand struct {
	Name        string
	Description string
	Command     string
}

func Manifest(manifest manifest.Manifest) error {
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	if err := util.EnsureParent(manifest.Paths.Manifest); err != nil {
		return fmt.Errorf("create manifest parent: %w", err)
	}

	if err := os.WriteFile(manifest.Paths.Manifest, append(payload, '\n'), 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	return nil
}

func ComposeEnv(manifest manifest.Manifest, adapter *config.AdapterConfig) error {
	if manifest.Paths.ComposeEnv == nil {
		return nil
	}

	env, err := ComputeEnv(manifest, adapter)
	if err != nil {
		return err
	}

	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := make([]byte, 0, len(keys)*24)
	for _, key := range keys {
		lines = append(lines, []byte(fmt.Sprintf("%s=%s\n", key, env[key]))...)
	}

	if err := util.EnsureParent(*manifest.Paths.ComposeEnv); err != nil {
		return fmt.Errorf("create compose env parent: %w", err)
	}

	if err := os.WriteFile(*manifest.Paths.ComposeEnv, lines, 0o644); err != nil {
		return fmt.Errorf("write compose env: %w", err)
	}

	return nil
}

func Outputs(manifest manifest.Manifest, adapter *config.AdapterConfig) error {
	context, err := TemplateContext(manifest, adapter)
	if err != nil {
		return err
	}

	for _, output := range manifest.Outputs.Generated {
		if !util.IsWithin(manifest.Lane.RepoRoot, output.Destination) {
			return fmt.Errorf("generated destination must stay within repo root: %s", output.Destination)
		}

		if err := render.File(output.Template, output.Destination, context); err != nil {
			return fmt.Errorf("render %s: %w", output.Destination, err)
		}
	}

	return nil
}

func RunCommands(manifest manifest.Manifest, adapter *config.AdapterConfig) ([]RenderedRunCommand, error) {
	context, err := TemplateContext(manifest, adapter)
	if err != nil {
		return nil, err
	}

	rendered := make([]RenderedRunCommand, 0, len(adapter.Runtime.Run.Commands))
	for _, command := range adapter.Runtime.Run.Commands {
		result, renderErr := render.Text(command.Command, context)
		if renderErr != nil {
			return nil, fmt.Errorf("render runtime.run command %q: %w", command.Name, renderErr)
		}

		rendered = append(rendered, RenderedRunCommand{
			Name:        command.Name,
			Description: command.Description,
			Command:     result,
		})
	}

	return rendered, nil
}

func ComputeEnv(manifest manifest.Manifest, adapter *config.AdapterConfig) (map[string]string, error) {
	env := map[string]string{
		"DEVLANE_APP":             adapter.App,
		"DEVLANE_APP_SLUG":        util.Slugify(adapter.App, false),
		"DEVLANE_KIND":            adapter.Kind,
		"DEVLANE_BRANCH":          manifest.Lane.Branch,
		"DEVLANE_MODE":            manifest.Lane.Mode,
		"DEVLANE_LANE":            manifest.Lane.Name,
		"DEVLANE_LANE_SLUG":       manifest.Lane.Slug,
		"DEVLANE_STABLE":          fmt.Sprintf("%t", manifest.Lane.Stable),
		"DEVLANE_REPO_ROOT":       manifest.Lane.RepoRoot,
		"DEVLANE_CONFIG":          manifest.Lane.ConfigPath,
		"DEVLANE_MANIFEST":        manifest.Paths.Manifest,
		"DEVLANE_STATE_ROOT":      manifest.Paths.StateRoot,
		"DEVLANE_CACHE_ROOT":      manifest.Paths.CacheRoot,
		"DEVLANE_RUNTIME_ROOT":    manifest.Paths.RuntimeRoot,
		"DEVLANE_COMPOSE_PROJECT": manifest.Network.ProjectName,
		"DEVLANE_PUBLIC_HOST":     derefOrEmpty(manifest.Network.PublicHost),
		"DEVLANE_PUBLIC_URL":      derefOrEmpty(manifest.Network.PublicURL),
	}

	if manifest.Paths.ComposeEnv != nil {
		env["DEVLANE_COMPOSE_ENV"] = *manifest.Paths.ComposeEnv
	} else {
		env["DEVLANE_COMPOSE_ENV"] = ""
	}

	for name, port := range manifest.Ports {
		env["DEVLANE_PORT_"+stringsToUpperSnake(name)] = fmt.Sprintf("%d", port.Port)
	}

	renderValues := map[string]string{
		"app":          adapter.App,
		"app_slug":     util.Slugify(adapter.App, false),
		"branch":       manifest.Lane.Branch,
		"branch_slug":  util.Slugify(manifest.Lane.Branch, false),
		"lane_name":    manifest.Lane.Name,
		"lane_slug":    manifest.Lane.Slug,
		"mode":         manifest.Lane.Mode,
		"public_host":  derefOrEmpty(manifest.Network.PublicHost),
		"public_url":   derefOrEmpty(manifest.Network.PublicURL),
		"project_name": manifest.Network.ProjectName,
		"state_root":   manifest.Paths.StateRoot,
		"cache_root":   manifest.Paths.CacheRoot,
		"runtime_root": manifest.Paths.RuntimeRoot,
	}

	for key, raw := range adapter.Runtime.Env {
		rendered, err := renderEnvValue(raw, renderValues)
		if err != nil {
			return nil, fmt.Errorf("render runtime.env.%s: %w", key, err)
		}
		env[key] = rendered
	}

	return env, nil
}

func TemplateContext(manifest manifest.Manifest, adapter *config.AdapterConfig) (map[string]any, error) {
	env, err := ComputeEnv(manifest, adapter)
	if err != nil {
		return nil, err
	}

	ports := make(map[string]any, len(manifest.Ports))
	for name, port := range manifest.Ports {
		ports[name] = port.Port
	}

	generated := make([]map[string]any, 0, len(manifest.Outputs.Generated))
	for _, output := range manifest.Outputs.Generated {
		generated = append(generated, map[string]any{
			"template":    output.Template,
			"destination": output.Destination,
		})
	}

	return map[string]any{
		"app":   manifest.App,
		"kind":  manifest.Kind,
		"ready": manifest.Ready,
		"lane": map[string]any{
			"name":       manifest.Lane.Name,
			"slug":       manifest.Lane.Slug,
			"mode":       manifest.Lane.Mode,
			"stable":     manifest.Lane.Stable,
			"branch":     manifest.Lane.Branch,
			"repoRoot":   manifest.Lane.RepoRoot,
			"configPath": manifest.Lane.ConfigPath,
		},
		"paths": map[string]any{
			"manifest":    manifest.Paths.Manifest,
			"composeEnv":  derefOrNil(manifest.Paths.ComposeEnv),
			"stateRoot":   manifest.Paths.StateRoot,
			"cacheRoot":   manifest.Paths.CacheRoot,
			"runtimeRoot": manifest.Paths.RuntimeRoot,
		},
		"network": map[string]any{
			"projectName": manifest.Network.ProjectName,
			"publicHost":  derefOrNil(manifest.Network.PublicHost),
			"publicUrl":   derefOrNil(manifest.Network.PublicURL),
		},
		"compose": map[string]any{
			"files":    append([]string{}, manifest.Compose.Files...),
			"profiles": append([]string{}, manifest.Compose.Profiles...),
		},
		"outputs": map[string]any{
			"generated": generated,
		},
		"ports": ports,
		"env":   env,
	}, nil
}

func renderEnvValue(raw any, values map[string]string) (string, error) {
	switch value := raw.(type) {
	case nil:
		return "", nil
	case bool:
		return fmt.Sprintf("%t", value), nil
	case int, int32, int64, float32, float64:
		return fmt.Sprint(value), nil
	case string:
		return util.RenderBracedPattern(value, values)
	default:
		return fmt.Sprint(value), nil
	}
}

func stringsToUpperSnake(value string) string {
	value = filepath.Base(value)
	value = strings.ReplaceAll(value, "-", "_")
	return strings.ToUpper(value)
}

func derefOrEmpty(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func derefOrNil(value *string) any {
	if value == nil {
		return nil
	}

	return *value
}
