package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"
)

type GeneratedOutput struct {
	Template    string `yaml:"template"`
	Destination string `yaml:"destination"`
}

type LanePaths struct {
	State   string `yaml:"state"`
	Cache   string `yaml:"cache"`
	Runtime string `yaml:"runtime"`
}

type HostPatterns struct {
	Stable string `yaml:"stable"`
	Dev    string `yaml:"dev"`
}

type LaneConfig struct {
	StableName     string       `yaml:"stable_name"`
	StableBranches []string     `yaml:"stable_branches"`
	ProjectPattern string       `yaml:"project_pattern"`
	PathRoots      LanePaths    `yaml:"path_roots"`
	HostPatterns   HostPatterns `yaml:"host_patterns"`
}

type RunCommand struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Command     string `yaml:"command"`
}

type RunConfig struct {
	Commands []RunCommand `yaml:"commands"`
}

type RuntimeConfig struct {
	ComposeFiles     []string       `yaml:"compose_files"`
	DefaultProfiles  []string       `yaml:"default_profiles"`
	OptionalProfiles []string       `yaml:"optional_profiles"`
	Env              map[string]any `yaml:"env"`
	Run              RunConfig      `yaml:"run"`
}

type OutputsConfig struct {
	ManifestPath   string            `yaml:"manifest_path"`
	ComposeEnvPath string            `yaml:"compose_env_path"`
	Generated      []GeneratedOutput `yaml:"generated"`
}

type PortConfig struct {
	Name       string `yaml:"name"`
	Default    int    `yaml:"default"`
	HealthPath string `yaml:"health_path"`
	StablePort *int   `yaml:"stable_port"`
	PoolHint   []int  `yaml:"pool_hint"`
}

type WorktreeConfig struct {
	Seed []string `yaml:"seed"`
}

type AdapterConfig struct {
	Schema   int            `yaml:"schema"`
	App      string         `yaml:"app"`
	Kind     string         `yaml:"kind"`
	Lane     LaneConfig     `yaml:"lane"`
	Runtime  RuntimeConfig  `yaml:"runtime"`
	Outputs  OutputsConfig  `yaml:"outputs"`
	Ports    []PortConfig   `yaml:"ports"`
	Reserved []int          `yaml:"reserved"`
	Worktree WorktreeConfig `yaml:"worktree"`
}

func (a *AdapterConfig) AllowedProfiles() []string {
	return dedupePreserveOrder(append(append([]string{}, a.Runtime.DefaultProfiles...), a.Runtime.OptionalProfiles...))
}

func (a *AdapterConfig) HasCompose() bool {
	return len(a.Runtime.ComposeFiles) > 0
}

func (a *AdapterConfig) HasRunCommands() bool {
	return len(a.Runtime.Run.Commands) > 0
}

func LoadAdapter(configPath string) (*AdapterConfig, error) {
	payload, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read adapter: %w", err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(payload))
	decoder.KnownFields(true)

	var adapter AdapterConfig
	if err := decoder.Decode(&adapter); err != nil {
		return nil, fmt.Errorf("decode adapter: %w", err)
	}

	if err := validateAdapter(&adapter); err != nil {
		return nil, err
	}

	return &adapter, nil
}

func validateAdapter(adapter *AdapterConfig) error {
	if adapter.Schema != 1 {
		return fmt.Errorf("only schema 1 is supported, got %d", adapter.Schema)
	}

	switch adapter.Kind {
	case "web", "cli", "hybrid":
	default:
		return fmt.Errorf("kind must be one of web, cli, hybrid")
	}

	if strings.TrimSpace(adapter.App) == "" {
		return errors.New("app is required")
	}

	if err := validateLane(adapter); err != nil {
		return err
	}

	if err := validateOutputs(adapter); err != nil {
		return err
	}

	if err := validatePorts(adapter); err != nil {
		return err
	}

	for _, command := range adapter.Runtime.Run.Commands {
		if strings.TrimSpace(command.Command) == "" {
			return errors.New("runtime.run.commands[].command is required")
		}
	}

	return nil
}

func validateLane(adapter *AdapterConfig) error {
	if strings.TrimSpace(adapter.Lane.StableName) == "" {
		return errors.New("lane.stable_name is required")
	}

	if strings.TrimSpace(adapter.Lane.ProjectPattern) == "" {
		return errors.New("lane.project_pattern is required")
	}

	if strings.TrimSpace(adapter.Lane.PathRoots.State) == "" ||
		strings.TrimSpace(adapter.Lane.PathRoots.Cache) == "" ||
		strings.TrimSpace(adapter.Lane.PathRoots.Runtime) == "" {
		return errors.New("lane.path_roots.state, cache, and runtime are required")
	}

	if adapter.Lane.HostPatterns.Dev != "" && !strings.Contains(adapter.Lane.HostPatterns.Dev, "{lane}") {
		return errors.New("lane.host_patterns.dev must contain {lane}")
	}

	if adapter.Lane.HostPatterns.Dev != "" &&
		adapter.Lane.HostPatterns.Stable != "" &&
		adapter.Lane.HostPatterns.Dev == adapter.Lane.HostPatterns.Stable {
		return errors.New("lane.host_patterns.dev and stable must differ")
	}

	return nil
}

func validateOutputs(adapter *AdapterConfig) error {
	if strings.TrimSpace(adapter.Outputs.ManifestPath) == "" {
		return errors.New("outputs.manifest_path is required")
	}

	if adapter.HasCompose() && strings.TrimSpace(adapter.Outputs.ComposeEnvPath) == "" {
		return errors.New("outputs.compose_env_path is required when runtime.compose_files is declared")
	}

	return nil
}

func validatePorts(adapter *AdapterConfig) error {
	seenPorts := make(map[string]struct{}, len(adapter.Ports))
	for _, port := range adapter.Ports {
		if strings.TrimSpace(port.Name) == "" {
			return errors.New("ports[].name is required")
		}

		if _, ok := seenPorts[port.Name]; ok {
			return fmt.Errorf("duplicate port name %q", port.Name)
		}
		seenPorts[port.Name] = struct{}{}

		if len(port.PoolHint) != 0 && len(port.PoolHint) != 2 {
			return fmt.Errorf("ports[%s].pool_hint must be a [low, high] pair", port.Name)
		}

		if len(port.PoolHint) == 2 && port.PoolHint[0] > port.PoolHint[1] {
			return fmt.Errorf("ports[%s].pool_hint low must be <= high", port.Name)
		}
	}

	return nil
}

func dedupePreserveOrder(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}

		seen[item] = struct{}{}
		result = append(result, item)
	}

	return result
}
