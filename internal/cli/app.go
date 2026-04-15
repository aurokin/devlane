package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/auro/devlane/internal/compose"
	"github.com/auro/devlane/internal/config"
	"github.com/auro/devlane/internal/doctor"
	"github.com/auro/devlane/internal/manifest"
	"github.com/auro/devlane/internal/write"
)

type commonFlags struct {
	config   string
	cwd      string
	lane     string
	mode     string
	profiles stringSlice
}

type stringSlice []string

func (s *stringSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func Run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 2
	}

	switch args[0] {
	case "inspect":
		return runInspect(args[1:])
	case "prepare":
		return runPrepare(args[1:])
	case "up":
		return runUp(args[1:])
	case "down":
		return runDown(args[1:])
	case "status":
		return runStatus(args[1:])
	case "doctor":
		return runDoctor(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", args[0])
		printUsage()
		return 2
	}
}

func runInspect(args []string) int {
	flags, fs := newCommonFlagSet("inspect")
	asJSON := fs.Bool("json", false, "Print JSON output")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	_, _, _, laneManifest, err := load(flags)
	if err != nil {
		return exitError(err)
	}

	if *asJSON {
		payload, marshalErr := json.MarshalIndent(laneManifest, "", "  ")
		if marshalErr != nil {
			return exitError(marshalErr)
		}

		fmt.Println(string(payload))
		return 0
	}

	fmt.Printf("app: %s\n", laneManifest.App)
	fmt.Printf("kind: %s\n", laneManifest.Kind)
	fmt.Printf("lane: %s (%s)\n", laneManifest.Lane.Name, laneManifest.Lane.Mode)
	fmt.Printf("project: %s\n", laneManifest.Network.ProjectName)
	if laneManifest.Network.PublicURL != nil {
		fmt.Printf("public_url: %s\n", *laneManifest.Network.PublicURL)
	} else {
		fmt.Println("public_url: -")
	}
	fmt.Printf("manifest: %s\n", laneManifest.Paths.Manifest)

	return 0
}

func runPrepare(args []string) int {
	flags, fs := newCommonFlagSet("prepare")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	_, _, adapter, laneManifest, err := load(flags)
	if err != nil {
		return exitError(err)
	}

	if err := prepare(laneManifest, adapter); err != nil {
		return exitError(err)
	}

	fmt.Printf("prepared lane %q at %s\n", laneManifest.Lane.Name, laneManifest.Paths.Manifest)
	return 0
}

func runUp(args []string) int {
	flags, fs := newCommonFlagSet("up")
	dryRun := fs.Bool("dry-run", false, "Print the compose command without executing it")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cwd, _, adapter, laneManifest, err := load(flags)
	if err != nil {
		return exitError(err)
	}

	if err := prepare(laneManifest, adapter); err != nil {
		return exitError(err)
	}

	if adapter.HasRunCommands() {
		if err := printRunCommands(laneManifest, adapter); err != nil {
			return exitError(err)
		}

		if !adapter.HasCompose() {
			return 0
		}

		fmt.Println()
	}

	if !adapter.HasCompose() {
		fmt.Println("adapter does not declare compose files; nothing to start")
		return 0
	}

	command, err := compose.BuildCommand(laneManifest, "up", flags.profiles)
	if err != nil {
		return exitError(err)
	}

	fmt.Println(strings.Join(command, " "))
	if *dryRun {
		return 0
	}

	return compose.Run(command, cwd)
}

func runDown(args []string) int {
	flags, fs := newCommonFlagSet("down")
	dryRun := fs.Bool("dry-run", false, "Print the compose command without executing it")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cwd, _, adapter, laneManifest, err := load(flags)
	if err != nil {
		return exitError(err)
	}

	if !adapter.HasCompose() {
		fmt.Println("adapter does not declare compose files; nothing to stop")
		return 0
	}

	command, err := compose.BuildCommand(laneManifest, "down", flags.profiles)
	if err != nil {
		return exitError(err)
	}

	fmt.Println(strings.Join(command, " "))
	if *dryRun {
		return 0
	}

	return compose.Run(command, cwd)
}

func runStatus(args []string) int {
	flags, fs := newCommonFlagSet("status")
	dryRun := fs.Bool("dry-run", false, "Print the compose command without executing it")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cwd, _, adapter, laneManifest, err := load(flags)
	if err != nil {
		return exitError(err)
	}

	if len(laneManifest.Ports) > 0 {
		fmt.Printf("Lane: %s (%s)\n", laneManifest.Lane.Name, laneManifest.Lane.Mode)
		fmt.Println("Services:")
		for name, port := range laneManifest.Ports {
			fmt.Printf("  %s\tport %d\tallocated=%t\n", name, port.Port, port.Allocated)
		}
	}

	if !adapter.HasCompose() {
		if len(laneManifest.Ports) == 0 {
			fmt.Println("adapter does not declare compose files or ports")
		}
		return 0
	}

	if len(laneManifest.Ports) > 0 {
		fmt.Println()
	}

	command, err := compose.BuildCommand(laneManifest, "status", flags.profiles)
	if err != nil {
		return exitError(err)
	}

	fmt.Println(strings.Join(command, " "))
	if *dryRun {
		return 0
	}

	return compose.Run(command, cwd)
}

func runDoctor(args []string) int {
	flags, fs := newCommonFlagSet("doctor")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	_, configPath, adapter, _, err := load(flags)
	if err != nil {
		return exitError(err)
	}

	for _, message := range doctor.Run(adapter, configPath) {
		fmt.Println(message)
	}

	return 0
}

func newCommonFlagSet(name string) (*commonFlags, *flag.FlagSet) {
	flags := &commonFlags{}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&flags.config, "config", "devlane.yaml", "Path to devlane.yaml")
	fs.StringVar(&flags.cwd, "cwd", ".", "Working directory used for discovery")
	fs.StringVar(&flags.lane, "lane", "", "Override lane name")
	fs.StringVar(&flags.mode, "mode", "", "Override lane mode")
	fs.Var(&flags.profiles, "profile", "Extra compose profile(s)")
	return flags, fs
}

func load(flags *commonFlags) (string, string, *config.AdapterConfig, manifest.Manifest, error) {
	cwd, err := filepath.Abs(flags.cwd)
	if err != nil {
		return "", "", nil, manifest.Manifest{}, fmt.Errorf("resolve cwd: %w", err)
	}

	configPath, err := resolveConfig(flags.config, cwd)
	if err != nil {
		return "", "", nil, manifest.Manifest{}, err
	}

	adapter, err := config.LoadAdapter(configPath)
	if err != nil {
		return "", "", nil, manifest.Manifest{}, err
	}

	laneManifest, err := manifest.Build(adapter, manifest.Options{
		CWD:        cwd,
		ConfigPath: configPath,
		LaneName:   flags.lane,
		Mode:       flags.mode,
		Profiles:   []string(flags.profiles),
	})
	if err != nil {
		return "", "", nil, manifest.Manifest{}, err
	}

	return cwd, configPath, adapter, laneManifest, nil
}

func prepare(laneManifest manifest.Manifest, adapter *config.AdapterConfig) error {
	if err := write.Manifest(laneManifest); err != nil {
		return err
	}

	if err := write.ComposeEnv(laneManifest, adapter); err != nil {
		return err
	}

	return write.Outputs(laneManifest, adapter)
}

func printRunCommands(laneManifest manifest.Manifest, adapter *config.AdapterConfig) error {
	commands, err := write.RunCommands(laneManifest, adapter)
	if err != nil {
		return err
	}

	if len(commands) == 0 {
		return nil
	}

	fmt.Printf("Bare-metal commands for lane %q:\n\n", laneManifest.Lane.Name)
	for _, command := range commands {
		if command.Description != "" {
			fmt.Printf("  # %s\n", command.Description)
		}
		fmt.Printf("  %s\n\n", command.Command)
	}

	return nil
}

func resolveConfig(raw, cwd string) (string, error) {
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw), nil
	}

	if _, err := os.Stat(raw); err == nil {
		return filepath.Clean(raw), nil
	}

	candidate := filepath.Join(cwd, raw)
	if _, err := os.Stat(candidate); err == nil {
		return filepath.Clean(candidate), nil
	}

	if raw == "devlane.yaml" {
		current := cwd
		for {
			probe := filepath.Join(current, raw)
			if _, err := os.Stat(probe); err == nil {
				return filepath.Clean(probe), nil
			}

			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}

	return "", fmt.Errorf("config not found: %s", raw)
}

func exitError(err error) int {
	fmt.Fprintln(os.Stderr, err)
	return 1
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "usage: devlane <inspect|prepare|up|down|status|doctor> [flags]")
}
