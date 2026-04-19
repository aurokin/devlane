package manifest

import (
	"fmt"
	"path/filepath"

	"github.com/auro/devlane/internal/config"
	"github.com/auro/devlane/internal/gitutil"
	"github.com/auro/devlane/internal/portalloc"
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
	Schema  int     `json:"schema"`
	App     string  `json:"app"`
	Kind    string  `json:"kind"`
	Ready   bool    `json:"ready"`
	Lane    Lane    `json:"lane"`
	Paths   Paths   `json:"paths"`
	Network Network `json:"network"`
	Ports   Ports   `json:"ports"`
	Compose Compose `json:"compose"`
	Outputs Outputs `json:"outputs"`
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

type Ports map[string]Port

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

type Inputs struct {
	CWD          string
	ConfigPath   string
	RepoRoot     string
	Branch       string
	LaneName     string
	LaneSlug     string
	Mode         string
	Stable       bool
	Network      Network
	Paths        Paths
	ComposeFiles []string
	Profiles     []string
	Generated    []GeneratedOutput
}

func BuildInputs(adapter *config.AdapterConfig, options Options) (Inputs, error) {
	cwd, err := filepath.Abs(options.CWD)
	if err != nil {
		return Inputs{}, fmt.Errorf("resolve cwd: %w", err)
	}

	configPath, err := filepath.Abs(options.ConfigPath)
	if err != nil {
		return Inputs{}, fmt.Errorf("resolve config path: %w", err)
	}
	adapterRoot := filepath.Dir(filepath.Clean(configPath))
	repoRoot, ok := gitutil.FindRepoRootOK(cwd)
	if !ok {
		repoRoot = adapterRoot
	}
	branch := gitutil.CurrentBranch(cwd)

	isStable, mode, err := deriveMode(adapter, options.Mode, branch)
	if err != nil {
		return Inputs{}, err
	}

	laneName := deriveLaneName(adapter, options.LaneName, isStable, branch, adapterRoot)
	patternValues, projectName, err := buildPatternValues(adapter, laneName, mode, branch)
	if err != nil {
		return Inputs{}, err
	}

	network, err := buildNetwork(adapter, patternValues, projectName, isStable)
	if err != nil {
		return Inputs{}, err
	}

	laneSlug := patternValues["lane"]
	paths, err := buildPaths(adapter, adapterRoot, repoRoot, laneSlug)
	if err != nil {
		return Inputs{}, err
	}

	composeFiles, err := buildComposeFiles(adapter, adapterRoot, repoRoot)
	if err != nil {
		return Inputs{}, err
	}

	profiles := deriveProfiles(adapter, options.Profiles)
	generated, err := buildGeneratedOutputs(adapter, adapterRoot, repoRoot)
	if err != nil {
		return Inputs{}, err
	}

	return Inputs{
		CWD:          cwd,
		ConfigPath:   filepath.Clean(configPath),
		RepoRoot:     repoRoot,
		Branch:       branch,
		LaneName:     laneName,
		LaneSlug:     laneSlug,
		Mode:         mode,
		Stable:       isStable,
		Network:      network,
		Paths:        paths,
		ComposeFiles: composeFiles,
		Profiles:     profiles,
		Generated:    generated,
	}, nil
}

func Validate(adapter *config.AdapterConfig, options Options) error {
	_, err := BuildInputs(adapter, options)
	return err
}

func Build(adapter *config.AdapterConfig, options Options) (Manifest, error) {
	inputs, err := BuildInputs(adapter, options)
	if err != nil {
		return Manifest{}, err
	}

	ports, ready, err := buildPorts(adapter, inputs.RepoRoot, inputs.LaneName, inputs.Mode, inputs.Branch, inputs.Stable)
	if err != nil {
		return Manifest{}, err
	}

	return Manifest{
		Schema: 1,
		App:    adapter.App,
		Kind:   adapter.Kind,
		Ready:  ready,
		Lane: Lane{
			Name:       inputs.LaneName,
			Slug:       inputs.LaneSlug,
			Mode:       inputs.Mode,
			Stable:     inputs.Stable,
			Branch:     inputs.Branch,
			RepoRoot:   inputs.RepoRoot,
			ConfigPath: inputs.ConfigPath,
		},
		Paths:   inputs.Paths,
		Network: inputs.Network,
		Ports:   ports,
		Compose: Compose{
			Files:    inputs.ComposeFiles,
			Profiles: inputs.Profiles,
		},
		Outputs: Outputs{
			Generated: inputs.Generated,
		},
	}, nil
}

func buildPorts(adapter *config.AdapterConfig, repoRoot, laneName, mode, branch string, isStable bool) (Ports, bool, error) {
	if len(adapter.Ports) == 0 {
		return Ports{}, true, nil
	}

	states, ready, err := portalloc.Inspect(adapter, portalloc.Lane{
		App:      adapter.App,
		RepoPath: repoRoot,
		Name:     laneName,
		Mode:     mode,
		Branch:   branch,
		Stable:   isStable,
	})
	if err != nil {
		return nil, false, err
	}

	ports := make(Ports, len(states))
	for name, state := range states {
		ports[name] = Port{
			Port:      state.Port,
			Allocated: state.Allocated,
			HealthURL: state.HealthURL,
		}
	}
	return ports, ready, nil
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

func deriveLaneName(adapter *config.AdapterConfig, explicitName string, isStable bool, branch, adapterRoot string) string {
	if explicitName != "" {
		return explicitName
	}

	if isStable {
		return adapter.Lane.StableName
	}

	if branch != "detached" {
		return branch
	}

	return filepath.Base(adapterRoot)
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

func buildPaths(adapter *config.AdapterConfig, adapterRoot, repoRoot, laneSlug string) (Paths, error) {
	manifestPath, err := util.ResolveAdapterPath(adapterRoot, repoRoot, adapter.Outputs.ManifestPath)
	if err != nil {
		return Paths{}, fmt.Errorf("resolve outputs.manifest_path: %w", err)
	}

	stateRoot, err := util.ResolveAdapterPath(adapterRoot, repoRoot, adapter.Lane.PathRoots.State)
	if err != nil {
		return Paths{}, fmt.Errorf("resolve lane.path_roots.state: %w", err)
	}

	cacheRoot, err := util.ResolveAdapterPath(adapterRoot, repoRoot, adapter.Lane.PathRoots.Cache)
	if err != nil {
		return Paths{}, fmt.Errorf("resolve lane.path_roots.cache: %w", err)
	}

	runtimeRoot, err := util.ResolveAdapterPath(adapterRoot, repoRoot, adapter.Lane.PathRoots.Runtime)
	if err != nil {
		return Paths{}, fmt.Errorf("resolve lane.path_roots.runtime: %w", err)
	}

	paths := Paths{
		Manifest:    manifestPath,
		StateRoot:   filepath.Clean(filepath.Join(stateRoot, laneSlug)),
		CacheRoot:   filepath.Clean(filepath.Join(cacheRoot, laneSlug)),
		RuntimeRoot: filepath.Clean(filepath.Join(runtimeRoot, laneSlug)),
	}

	if adapter.HasCompose() {
		path, err := util.ResolveAdapterPath(adapterRoot, repoRoot, adapter.Outputs.ComposeEnvPath)
		if err != nil {
			return Paths{}, fmt.Errorf("resolve outputs.compose_env_path: %w", err)
		}
		paths.ComposeEnv = &path
	}

	return paths, nil
}

func buildComposeFiles(adapter *config.AdapterConfig, adapterRoot, repoRoot string) ([]string, error) {
	files := make([]string, 0, len(adapter.Runtime.ComposeFiles))
	for _, composeFile := range adapter.Runtime.ComposeFiles {
		resolved, err := util.ResolveAdapterPath(adapterRoot, repoRoot, composeFile)
		if err != nil {
			return nil, fmt.Errorf("resolve runtime.compose_files entry %q: %w", composeFile, err)
		}
		files = append(files, resolved)
	}

	return files, nil
}

func deriveProfiles(adapter *config.AdapterConfig, explicitProfiles []string) []string {
	if len(explicitProfiles) > 0 {
		return util.DedupePreserveOrder(explicitProfiles)
	}

	return util.DedupePreserveOrder(adapter.Runtime.DefaultProfiles)
}

func buildGeneratedOutputs(adapter *config.AdapterConfig, adapterRoot, repoRoot string) ([]GeneratedOutput, error) {
	generated := make([]GeneratedOutput, 0, len(adapter.Outputs.Generated))
	for _, output := range adapter.Outputs.Generated {
		templatePath, err := util.ResolveAdapterPath(adapterRoot, repoRoot, output.Template)
		if err != nil {
			return nil, fmt.Errorf("resolve generated template %q: %w", output.Template, err)
		}

		destinationPath, err := util.ResolveAdapterPath(adapterRoot, repoRoot, output.Destination)
		if err != nil {
			return nil, fmt.Errorf("resolve generated destination %q: %w", output.Destination, err)
		}

		generated = append(generated, GeneratedOutput{
			Template:    templatePath,
			Destination: destinationPath,
		})
	}

	return generated, nil
}
