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
	"github.com/auro/devlane/internal/initcmd"
	"github.com/auro/devlane/internal/manifest"
	"github.com/auro/devlane/internal/portalloc"
	"github.com/auro/devlane/internal/write"
)

var publishPrepareSession = func(session *portalloc.PrepareSession) error {
	return session.Publish()
}

var closePrepareSession = func(session *portalloc.PrepareSession) error {
	return session.Close()
}

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
	case "init":
		return runInit(args[1:])
	case "inspect":
		return runInspect(args[1:])
	case "prepare":
		return runPrepare(args[1:])
	case "port":
		return runPort(args[1:])
	case "up":
		return runUp(args[1:])
	case "down":
		return runDown(args[1:])
	case "status":
		return runStatus(args[1:])
	case "doctor":
		return runDoctor(args[1:])
	case "host":
		return runHost(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", args[0])
		printUsage()
		return 2
	}
}

func runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		cwd      string
		appPath  string
		template string
		from     string
		list     bool
		yes      bool
		all      bool
		force    bool
	)

	fs.StringVar(&cwd, "cwd", ".", "Working directory used for scanning")
	fs.StringVar(&appPath, "app", "", "Target app path relative to --cwd")
	fs.StringVar(&template, "template", "", "Starter template to use")
	fs.StringVar(&from, "from", "", "Copy an existing adapter as the starting point")
	fs.BoolVar(&list, "list", false, "List detected candidates without writing")
	fs.BoolVar(&yes, "yes", false, "Skip interactive confirmation prompts")
	fs.BoolVar(&all, "all", false, "Scaffold every detected candidate in monorepo mode")
	fs.BoolVar(&force, "force", false, "Overwrite an existing devlane.yaml")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	result, err := initcmd.Execute(initcmd.Options{
		CWD:      cwd,
		AppPath:  appPath,
		Template: template,
		From:     from,
		List:     list,
		Yes:      yes,
		All:      all,
		Force:    force,
		Stdin:    os.Stdin,
		Stdout:   os.Stdout,
	})
	if err != nil {
		return exitError(err)
	}
	for _, message := range result.Messages {
		fmt.Println(message)
	}
	return 0
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

	catalogLane := portallocLane(adapter, laneManifest)
	needsCatalogPrepare, err := needsCatalogPrepare(adapter, catalogLane)
	if err != nil {
		return exitError(err)
	}

	if needsCatalogPrepare {
		return runCatalogPrepare(adapter, laneManifest, catalogLane)
	}

	result, err := write.Prepare(laneManifest, adapter)
	if err != nil {
		return exitError(err)
	}
	for _, message := range result.Messages {
		fmt.Println(message)
	}

	fmt.Printf("prepared lane %q at %s\n", laneManifest.Lane.Name, laneManifest.Paths.Manifest)
	return 0
}

func runPort(args []string) int {
	flags, fs := newCommonFlagSet("port")
	verbose := fs.Bool("verbose", false, "Print service, port, allocation, lane, and repo path")
	probe := fs.Bool("probe", false, "Exit 0 only when the assigned port is bindable")
	if err := fs.Parse(reorderPortArgs(args)); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: devlane port <service> [flags]")
		return 2
	}

	service := fs.Arg(0)
	cwd, configPath, adapter, err := loadAdapter(flags)
	if err != nil {
		return exitError(err)
	}

	inputs, err := manifest.BuildInputs(adapter, manifest.Options{
		CWD:        cwd,
		ConfigPath: configPath,
		LaneName:   flags.lane,
		Mode:       flags.mode,
		Profiles:   []string(flags.profiles),
	})
	if err != nil {
		return exitError(err)
	}

	row, err := assignedPort(adapter, inputs, service)
	if err != nil {
		return exitError(err)
	}

	if *verbose {
		fmt.Printf("service=%s port=%d allocated=true mode=%s lane=%s repoPath=%s\n", service, row.Port, inputs.Mode, inputs.LaneName, inputs.RepoRoot)
	} else {
		fmt.Println(row.Port)
	}

	if *probe {
		if err := portalloc.Probe(row.Port); err != nil {
			return 1
		}
	}

	return 0
}

func reorderPortArgs(args []string) []string {
	flagArgs := make([]string, 0, len(args))
	serviceArgs := make([]string, 0, 1)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			serviceArgs = append(serviceArgs, arg)
			continue
		}

		flagArgs = append(flagArgs, arg)
		if portFlagNeedsValue(arg) && i+1 < len(args) {
			i++
			flagArgs = append(flagArgs, args[i])
		}
	}

	return append(flagArgs, serviceArgs...)
}

func portFlagNeedsValue(arg string) bool {
	name := strings.TrimLeft(arg, "-")
	if name == "" || strings.Contains(name, "=") {
		return false
	}

	switch name {
	case "config", "cwd", "lane", "mode", "profile":
		return true
	default:
		return false
	}
}

func assignedPort(adapter *config.AdapterConfig, inputs manifest.Inputs, service string) (portalloc.Allocation, error) {
	portConfig, ok := adapterPort(adapter, service)
	if !ok {
		return portalloc.Allocation{}, fmt.Errorf("service %q is not declared in the adapter", service)
	}

	rows, err := portalloc.List()
	if err != nil {
		return portalloc.Allocation{}, err
	}

	for _, row := range rows {
		if row.App == adapter.App && row.Service == service && sameCatalogRepoPath(row.RepoPath, inputs.RepoRoot) {
			if inputs.Stable && row.Port != stableFixture(portConfig) {
				return portalloc.Allocation{}, missingAssignedPortError(service)
			}
			return row, nil
		}
	}

	return portalloc.Allocation{}, missingAssignedPortError(service)
}

func adapterPort(adapter *config.AdapterConfig, service string) (config.PortConfig, bool) {
	for _, portConfig := range adapter.Ports {
		if portConfig.Name == service {
			return portConfig, true
		}
	}
	return config.PortConfig{}, false
}

func stableFixture(portConfig config.PortConfig) int {
	if portConfig.StablePort != nil {
		return *portConfig.StablePort
	}
	return portConfig.Default
}

func missingAssignedPortError(service string) error {
	return fmt.Errorf("no assigned port for service %q; run `devlane inspect --json` to see the current provisional candidate or `devlane prepare` to commit an allocation", service)
}

func sameCatalogRepoPath(left, right string) bool {
	if filepath.Clean(left) == filepath.Clean(right) {
		return true
	}

	leftResolved, leftErr := filepath.EvalSymlinks(left)
	rightResolved, rightErr := filepath.EvalSymlinks(right)
	if leftErr != nil || rightErr != nil {
		return false
	}

	return filepath.Clean(leftResolved) == filepath.Clean(rightResolved)
}

func portallocLane(adapter *config.AdapterConfig, laneManifest manifest.Manifest) portalloc.Lane {
	return portalloc.Lane{
		App:      adapter.App,
		RepoPath: laneManifest.Lane.RepoRoot,
		Name:     laneManifest.Lane.Name,
		Mode:     laneManifest.Lane.Mode,
		Branch:   laneManifest.Lane.Branch,
		Stable:   laneManifest.Lane.Stable,
	}
}

func needsCatalogPrepare(adapter *config.AdapterConfig, lane portalloc.Lane) (bool, error) {
	if len(adapter.Ports) > 0 {
		return true, nil
	}

	return portalloc.HasAllocation(lane)
}

func runCatalogPrepare(adapter *config.AdapterConfig, laneManifest manifest.Manifest, lane portalloc.Lane) int {
	session, err := portalloc.BeginPrepare(adapter, lane)
	if err != nil {
		return exitError(err)
	}
	defer session.Close()

	laneManifest = applyPortStates(laneManifest, session.States(), session.Ready())

	result, rollback, err := write.PrepareWithRollback(laneManifest, adapter)
	if err != nil {
		return exitError(err)
	}
	if err := publishPrepareSession(session); err != nil {
		if rollbackErr := rollback(); rollbackErr != nil {
			return exitError(fmt.Errorf("%w; rollback local prepare state: %v", err, rollbackErr))
		}
		return exitError(err)
	}
	if err := closePrepareSession(session); err != nil {
		return exitError(fmt.Errorf("prepared lane %q and published catalog state, but failed to release the catalog lock: %w", laneManifest.Lane.Name, err))
	}
	for _, message := range result.Messages {
		fmt.Println(message)
	}

	fmt.Printf("prepared lane %q at %s\n", laneManifest.Lane.Name, laneManifest.Paths.Manifest)
	return 0
}

func applyPortStates(base manifest.Manifest, states map[string]portalloc.State, ready bool) manifest.Manifest {
	base.Ready = ready
	base.Ports = make(manifest.Ports, len(states))
	for name, state := range states {
		base.Ports[name] = manifest.Port{
			Port:      state.Port,
			Allocated: state.Allocated,
			HealthURL: state.HealthURL,
		}
	}
	return base
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

	if err := verifyUpPortReadiness(adapter, laneManifest); err != nil {
		return exitError(err)
	}

	printedRunCommands, done, err := handleRunCommandsForUp(laneManifest, adapter)
	if err != nil {
		return exitError(err)
	}
	if done {
		return 0
	}

	if !printedRunCommands {
		if err := write.VerifyPreparedOutputs(laneManifest, adapter); err != nil {
			return exitError(err)
		}
	}
	if printedRunCommands {
		fmt.Println()
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

func verifyUpPortReadiness(adapter *config.AdapterConfig, laneManifest manifest.Manifest) error {
	needsPreparedPorts := len(adapter.Ports) > 0 && (adapter.HasCompose() || adapter.HasRunCommands())
	if needsPreparedPorts && !laneManifest.Ready {
		return fmt.Errorf("lane %q has unallocated ports; run `devlane prepare` first", laneManifest.Lane.Name)
	}

	return nil
}

func handleRunCommandsForUp(laneManifest manifest.Manifest, adapter *config.AdapterConfig) (bool, bool, error) {
	if adapter.HasRunCommands() {
		if err := printRunCommands(laneManifest, adapter); err != nil {
			return false, false, err
		}

		if err := write.VerifyPreparedOutputs(laneManifest, adapter); err != nil {
			return true, false, err
		}
		return true, !adapter.HasCompose(), nil
	}

	if !adapter.HasCompose() {
		fmt.Println("adapter does not declare compose files; nothing to start")
		return false, true, nil
	}

	return false, false, nil
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

	printStatusSummary(laneManifest, adapter)
	if err := printPortStatus(laneManifest, adapter); err != nil {
		return exitError(err)
	}

	if !adapter.HasCompose() {
		return 0
	}

	if adapter.HasRunCommands() || len(adapter.Ports) > 0 {
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

	cwd, configPath, adapter, err := loadAdapter(flags)
	if err != nil {
		return exitError(err)
	}

	inputs, err := manifest.BuildInputs(adapter, manifest.Options{
		CWD:        cwd,
		ConfigPath: configPath,
		LaneName:   flags.lane,
		Mode:       flags.mode,
		Profiles:   []string(flags.profiles),
	})
	if err != nil {
		return exitError(err)
	}

	result := doctor.Run(adapter, inputs.ComposeFiles, configPath)
	for _, message := range result.Messages {
		fmt.Println(message)
	}

	if result.Failed {
		return 1
	}

	return 0
}

func loadAdapter(flags *commonFlags) (string, string, *config.AdapterConfig, error) {
	cwd, err := filepath.Abs(flags.cwd)
	if err != nil {
		return "", "", nil, fmt.Errorf("resolve cwd: %w", err)
	}

	configPath, err := resolveConfig(flags.config, cwd)
	if err != nil {
		return "", "", nil, err
	}

	adapter, err := config.LoadAdapter(configPath)
	if err != nil {
		return "", "", nil, err
	}

	return cwd, configPath, adapter, nil
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
	cwd, configPath, adapter, err := loadAdapter(flags)
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

	candidate := filepath.Join(cwd, raw)
	if _, err := os.Stat(candidate); err == nil {
		absolute, absErr := filepath.Abs(candidate)
		if absErr != nil {
			return "", fmt.Errorf("resolve config: %w", absErr)
		}
		return filepath.Clean(absolute), nil
	}

	if _, err := os.Stat(raw); err == nil {
		absolute, absErr := filepath.Abs(raw)
		if absErr != nil {
			return "", fmt.Errorf("resolve config: %w", absErr)
		}
		return filepath.Clean(absolute), nil
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

	if raw == "devlane.yaml" {
		return "", fmt.Errorf("config not found: %s (run `devlane init` or pass --config)", raw)
	}

	return "", fmt.Errorf("config not found: %s", raw)
}

func printStatusSummary(laneManifest manifest.Manifest, adapter *config.AdapterConfig) {
	fmt.Printf("Lane: %s (%s)\n", laneManifest.Lane.Name, laneManifest.Lane.Mode)
	fmt.Printf("App: %s (%s)\n", laneManifest.App, laneManifest.Kind)
	fmt.Printf("Project: %s\n", laneManifest.Network.ProjectName)
	if laneManifest.Network.PublicURL != nil {
		fmt.Printf("Public URL: %s\n", *laneManifest.Network.PublicURL)
	} else {
		fmt.Println("Public URL: -")
	}
	if len(laneManifest.Ports) > 0 {
		fmt.Printf("Ports ready: %t\n", laneManifest.Ready)
	}

	if adapter.HasRunCommands() {
		fmt.Printf("Bare-metal commands: %d declared\n", len(adapter.Runtime.Run.Commands))
	} else if !adapter.HasCompose() {
		fmt.Println("Bare-metal commands: none declared")
	}
}

func printPortStatus(laneManifest manifest.Manifest, adapter *config.AdapterConfig) error {
	if len(adapter.Ports) == 0 {
		return nil
	}

	fmt.Println("Services:")
	for _, portConfig := range adapter.Ports {
		portState, ok := laneManifest.Ports[portConfig.Name]
		if !ok {
			return fmt.Errorf("manifest is missing port state for service %q", portConfig.Name)
		}

		status := "unallocated"
		if portState.Allocated {
			status = "free"
			if err := portalloc.Probe(portState.Port); err != nil {
				status = "bound"
			}
		}

		fmt.Printf("  %-6s port %-5d %s\n", portConfig.Name, portState.Port, status)
	}

	return nil
}

func exitError(err error) int {
	fmt.Fprintln(os.Stderr, err)
	return 1
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "usage: devlane <init|inspect|prepare|port|up|down|status|doctor|host> [flags]")
}
