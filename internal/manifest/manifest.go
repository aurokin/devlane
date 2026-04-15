package manifest

import (
	"fmt"
	"path/filepath"

	"github.com/auro/devlane/internal/config"
	"github.com/auro/devlane/internal/gitutil"
	"github.com/auro/devlane/internal/util"
)

type Options struct {
	CWD        string
	ConfigPath string
	LaneName   string
	Mode       string
	Profiles   []string
}

type Manifest struct {
	Schema  int             `json:"schema"`
	App     string          `json:"app"`
	Kind    string          `json:"kind"`
	Ready   bool            `json:"ready"`
	Lane    Lane            `json:"lane"`
	Paths   Paths           `json:"paths"`
	Network Network         `json:"network"`
	Ports   map[string]Port `json:"ports"`
	Compose Compose         `json:"compose"`
	Outputs Outputs         `json:"outputs"`
}

type Lane struct {
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	Mode       string `json:"mode"`
	Stable     bool   `json:"stable"`
	Branch     string `json:"branch"`
	RepoRoot   string `json:"repoRoot"`
	ConfigPath string `json:"configPath"`
}

type Paths struct {
	Manifest    string  `json:"manifest"`
	ComposeEnv  *string `json:"composeEnv,omitempty"`
	StateRoot   string  `json:"stateRoot"`
	CacheRoot   string  `json:"cacheRoot"`
	RuntimeRoot string  `json:"runtimeRoot"`
}

type Network struct {
	ProjectName string  `json:"projectName"`
	PublicHost  *string `json:"publicHost"`
	PublicURL   *string `json:"publicUrl"`
}

type Port struct {
	Port      int     `json:"port"`
	Allocated bool    `json:"allocated"`
	HealthURL *string `json:"healthUrl"`
}

type Compose struct {
	Files    []string `json:"files"`
	Profiles []string `json:"profiles"`
}

type Outputs struct {
	Generated []GeneratedOutput `json:"generated"`
}

type GeneratedOutput struct {
	Template    string `json:"template"`
	Destination string `json:"destination"`
}

func Build(adapter *config.AdapterConfig, options Options) (Manifest, error) {
	cwd, err := filepath.Abs(options.CWD)
	if err != nil {
		return Manifest{}, fmt.Errorf("resolve cwd: %w", err)
	}

	repoRoot := gitutil.FindRepoRoot(cwd)
	branch := gitutil.CurrentBranch(cwd)

	isStable, mode, err := deriveMode(adapter, options.Mode, branch)
	if err != nil {
		return Manifest{}, err
	}

	laneName := deriveLaneName(adapter, options.LaneName, isStable, branch, cwd)
	patternValues, projectName, err := buildPatternValues(adapter, laneName, mode, branch)
	if err != nil {
		return Manifest{}, err
	}

	network, err := buildNetwork(adapter, patternValues, projectName, isStable)
	if err != nil {
		return Manifest{}, err
	}

	laneSlug := patternValues["lane"]
	paths := buildPaths(adapter, repoRoot, laneSlug)
	ports := buildPorts(adapter, isStable)
	composeFiles := buildComposeFiles(adapter, repoRoot)
	profiles := deriveProfiles(adapter, options.Profiles)
	generated := buildGeneratedOutputs(adapter, repoRoot)

	ready := len(ports) == 0

	return Manifest{
		Schema: 1,
		App:    adapter.App,
		Kind:   adapter.Kind,
		Ready:  ready,
		Lane: Lane{
			Name:       laneName,
			Slug:       laneSlug,
			Mode:       mode,
			Stable:     isStable,
			Branch:     branch,
			RepoRoot:   repoRoot,
			ConfigPath: filepath.Clean(options.ConfigPath),
		},
		Paths:   paths,
		Network: network,
		Ports:   ports,
		Compose: Compose{
			Files:    composeFiles,
			Profiles: profiles,
		},
		Outputs: Outputs{
			Generated: generated,
		},
	}, nil
}

func deriveMode(adapter *config.AdapterConfig, explicitMode, branch string) (bool, string, error) {
	if explicitMode != "" && explicitMode != "stable" && explicitMode != "dev" {
		return false, "", fmt.Errorf("mode must be stable or dev")
	}

	if explicitMode == "stable" {
		return true, "stable", nil
	}

	if explicitMode == "dev" {
		return false, "dev", nil
	}

	for _, stableBranch := range adapter.Lane.StableBranches {
		if stableBranch == branch {
			return true, "stable", nil
		}
	}

	return false, "dev", nil
}

func deriveLaneName(adapter *config.AdapterConfig, explicitName string, isStable bool, branch, cwd string) string {
	if explicitName != "" {
		return explicitName
	}

	if isStable {
		return adapter.Lane.StableName
	}

	if branch != "detached" {
		return branch
	}

	return filepath.Base(cwd)
}

func buildPatternValues(adapter *config.AdapterConfig, laneName, mode, branch string) (map[string]string, string, error) {
	laneSlug := util.Slugify(laneName, false)
	if laneSlug == "" {
		laneSlug = "lane"
	}

	appSlug := util.Slugify(adapter.App, false)
	if appSlug == "" {
		appSlug = "app"
	}

	values := map[string]string{
		"app":    appSlug,
		"lane":   laneSlug,
		"mode":   mode,
		"branch": util.Slugify(branch, false),
	}

	projectPattern, err := util.RenderBracedPattern(adapter.Lane.ProjectPattern, values)
	if err != nil {
		return nil, "", fmt.Errorf("render lane.project_pattern: %w", err)
	}

	projectName := util.Slugify(projectPattern, true)
	values["project"] = projectName

	return values, projectName, nil
}

func buildNetwork(adapter *config.AdapterConfig, values map[string]string, projectName string, isStable bool) (Network, error) {
	var (
		publicHost *string
		err        error
	)

	switch {
	case isStable && adapter.Lane.HostPatterns.Stable != "":
		publicHost, err = renderHost(adapter.Lane.HostPatterns.Stable, values)
	case !isStable && adapter.Lane.HostPatterns.Dev != "":
		publicHost, err = renderHost(adapter.Lane.HostPatterns.Dev, values)
	}
	if err != nil {
		return Network{}, err
	}

	var publicURL *string
	if publicHost != nil {
		url := "http://" + *publicHost
		publicURL = &url
	}

	return Network{
		ProjectName: projectName,
		PublicHost:  publicHost,
		PublicURL:   publicURL,
	}, nil
}

func renderHost(pattern string, values map[string]string) (*string, error) {
	host, err := util.RenderBracedPattern(pattern, values)
	if err != nil {
		return nil, fmt.Errorf("render host pattern: %w", err)
	}

	return &host, nil
}

func buildPaths(adapter *config.AdapterConfig, repoRoot, laneSlug string) Paths {
	paths := Paths{
		Manifest:    util.ResolvePath(repoRoot, adapter.Outputs.ManifestPath),
		StateRoot:   filepath.Clean(filepath.Join(util.ResolvePath(repoRoot, adapter.Lane.PathRoots.State), laneSlug)),
		CacheRoot:   filepath.Clean(filepath.Join(util.ResolvePath(repoRoot, adapter.Lane.PathRoots.Cache), laneSlug)),
		RuntimeRoot: filepath.Clean(filepath.Join(util.ResolvePath(repoRoot, adapter.Lane.PathRoots.Runtime), laneSlug)),
	}

	if adapter.HasCompose() {
		path := util.ResolvePath(repoRoot, adapter.Outputs.ComposeEnvPath)
		paths.ComposeEnv = &path
	}

	return paths
}

func buildPorts(adapter *config.AdapterConfig, isStable bool) map[string]Port {
	ports := make(map[string]Port, len(adapter.Ports))
	for _, portConfig := range adapter.Ports {
		number := portConfig.Default
		if isStable && portConfig.StablePort != nil {
			number = *portConfig.StablePort
		}

		var healthURL *string
		if portConfig.HealthPath != "" {
			url := fmt.Sprintf("http://localhost:%d%s", number, portConfig.HealthPath)
			healthURL = &url
		}

		ports[portConfig.Name] = Port{
			Port:      number,
			Allocated: false,
			HealthURL: healthURL,
		}
	}

	return ports
}

func buildComposeFiles(adapter *config.AdapterConfig, repoRoot string) []string {
	files := make([]string, 0, len(adapter.Runtime.ComposeFiles))
	for _, composeFile := range adapter.Runtime.ComposeFiles {
		files = append(files, util.ResolvePath(repoRoot, composeFile))
	}

	return files
}

func deriveProfiles(adapter *config.AdapterConfig, explicitProfiles []string) []string {
	if len(explicitProfiles) > 0 {
		return util.DedupePreserveOrder(explicitProfiles)
	}

	return util.DedupePreserveOrder(adapter.Runtime.DefaultProfiles)
}

func buildGeneratedOutputs(adapter *config.AdapterConfig, repoRoot string) []GeneratedOutput {
	generated := make([]GeneratedOutput, 0, len(adapter.Outputs.Generated))
	for _, output := range adapter.Outputs.Generated {
		generated = append(generated, GeneratedOutput{
			Template:    util.ResolvePath(repoRoot, output.Template),
			Destination: util.ResolvePath(repoRoot, output.Destination),
		})
	}

	return generated
}
