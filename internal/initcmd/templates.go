package initcmd

import (
	"fmt"
	"strings"

	"github.com/auro/devlane/internal/util"
)

func ScaffoldTemplate(name TemplateName, targetBase string, composeFiles []string) (string, error) {
	appName := util.Slugify(targetBase, false)
	if appName == "" {
		appName = "app"
	}
	composeFilesYAML := composeFilesBlock(composeFiles)

	switch name {
	case TemplateContainerizedWeb:
		return fmt.Sprintf(`schema: 1
app: %s
kind: web

lane:
  stable_name: stable
  stable_branches: [main, master]
  project_pattern: "{app}_{lane}"
  path_roots:
    state: ".devlane/state"
    cache: ".devlane/cache"
    runtime: ".devlane/runtime"

  # host_patterns is optional. Uncomment if you have a proxy or DNS
  # resolving these hostnames (Caddy, Traefik, /etc/hosts, etc.).
  # host_patterns:
  #   stable: "{app}.localhost"
  #   dev: "{lane}.{app}.localhost"

runtime:
  compose_files:
%s
  default_profiles: [web]
  optional_profiles: []
  env: {}

# reserved:
#   - 5555

# worktree:
#   seed:
#     - .env.secrets
#     - config/master.key

outputs:
  manifest_path: ".devlane/manifest.json"
  compose_env_path: ".devlane/compose.env"
  generated: []
`, appName, composeFilesYAML), nil
	case TemplateBaremetalWeb:
		return fmt.Sprintf(`schema: 1
app: %s
kind: web

lane:
  stable_name: stable
  stable_branches: [main, master]
  project_pattern: "{app}_{lane}"
  path_roots:
    state: ".devlane/state"
    cache: ".devlane/cache"
    runtime: ".devlane/runtime"

  # host_patterns is optional. Uncomment if you have a proxy or DNS
  # resolving these hostnames (Caddy, Traefik, /etc/hosts, etc.).
  # host_patterns:
  #   stable: "{app}.localhost"
  #   dev: "{lane}.{app}.localhost"

runtime:
  env: {}
  # run:
  #   commands:
  #     - name: web
  #       description: "Start the app"
  #       command: "bin/server"

# reserved:
#   - 5555

# worktree:
#   seed:
#     - .env.secrets
#     - config/master.key

outputs:
  manifest_path: ".devlane/manifest.json"
  generated: []
`, appName), nil
	case TemplateHybridWeb:
		return fmt.Sprintf(`schema: 1
app: %s
kind: hybrid

lane:
  stable_name: stable
  stable_branches: [main, master]
  project_pattern: "{app}_{lane}"
  path_roots:
    state: ".devlane/state"
    cache: ".devlane/cache"
    runtime: ".devlane/runtime"

  # host_patterns is optional. Uncomment if you have a proxy or DNS
  # resolving these hostnames (Caddy, Traefik, /etc/hosts, etc.).
  # host_patterns:
  #   stable: "{app}.localhost"
  #   dev: "{lane}.{app}.localhost"

runtime:
  compose_files:
%s
  default_profiles: [cache]
  optional_profiles: []
  env: {}
  run:
    commands:
      - name: web
        description: "Start the app (bare-metal)"
        command: "bin/server"

# reserved:
#   - 5555

# worktree:
#   seed:
#     - .env.secrets
#     - config/master.key

outputs:
  manifest_path: ".devlane/manifest.json"
  compose_env_path: ".devlane/compose.env"
  generated: []
`, appName, composeFilesYAML), nil
	case TemplateCLI:
		return fmt.Sprintf(`schema: 1
app: %s
kind: cli

lane:
  stable_name: stable
  stable_branches: [main, master]
  project_pattern: "{app}_{lane}"
  path_roots:
    state: ".devlane/state"
    cache: ".devlane/cache"
    runtime: ".devlane/runtime"

  # host_patterns is optional. Uncomment if you have a proxy or DNS
  # resolving these hostnames.
  # host_patterns:
  #   stable: "{app}.localhost"
  #   dev: "{lane}.{app}.localhost"

runtime:
  env: {}

# worktree:
#   seed:
#     - .env.secrets
#     - config/master.key

outputs:
  manifest_path: ".devlane/manifest.json"
  generated: []
`, appName), nil
	default:
		return "", fmt.Errorf("unknown template %q", name)
	}
}

func composeFilesBlock(composeFiles []string) string {
	if len(composeFiles) == 0 {
		composeFiles = []string{"compose.yaml"}
	}

	lines := make([]string, 0, len(composeFiles))
	for _, composeFile := range composeFiles {
		lines = append(lines, fmt.Sprintf("    - %s", composeFile))
	}
	return strings.Join(lines, "\n")
}
