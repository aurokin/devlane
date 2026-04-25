package initcmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/auro/devlane/internal/config"
	"github.com/auro/devlane/internal/gitutil"
	"github.com/auro/devlane/internal/util"
)

const maxScanDepth = 3

type TemplateName string

const (
	TemplateContainerizedWeb TemplateName = "containerized-web"
	TemplateBaremetalWeb     TemplateName = "baremetal-web"
	TemplateHybridWeb        TemplateName = "hybrid-web"
	TemplateCLI              TemplateName = "cli"
)

type DetectionKind string

const (
	DetectionContainerized DetectionKind = "containerized"
	DetectionBaremetal     DetectionKind = "bare-metal"
	DetectionCLI           DetectionKind = "cli"
	DetectionAmbiguous     DetectionKind = "ambiguous"
)

type Candidate struct {
	Path             string
	Template         TemplateName
	Kind             DetectionKind
	Reason           string
	HybridHint       bool
	FallbackToCLI    bool
	ComposeFiles     []string
	FoundSignals     []string
	DisplaySelection string
}

type Options struct {
	CWD      string
	AppPath  string
	Template string
	From     string
	List     bool
	Yes      bool
	All      bool
	Force    bool
	Stdin    *os.File
	Stdout   *os.File
}

type Result struct {
	Messages []string
}

type writePlan struct {
	Target   string
	Content  []byte
	Messages []string
}

func Execute(opts Options) (Result, error) {
	if err := validateOptions(opts); err != nil {
		return Result{}, err
	}

	cwd, err := filepath.Abs(opts.CWD)
	if err != nil {
		return Result{}, fmt.Errorf("resolve cwd: %w", err)
	}

	stdin := opts.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}

	if opts.List {
		return listCandidates(cwd, stdout)
	}

	if opts.AppPath != "" {
		if opts.From != "" {
			return executeFrom(cwd, opts, stdin, stdout)
		}
		if opts.Template != "" {
			return executeTemplate(cwd, opts, stdin, stdout)
		}
		return executeExplicitPath(cwd, opts, stdin, stdout)
	}

	candidates, err := selectScanCandidates(cwd, opts, stdin, stdout)
	if err != nil {
		return Result{}, err
	}

	if opts.From != "" {
		return executeFromCandidates(cwd, opts, candidates, stdin, stdout)
	}
	if opts.Template != "" {
		return executeTemplateCandidates(opts, candidates, stdin, stdout)
	}
	return writeCandidates(candidates, opts, stdin, stdout)
}

func ParseTemplateName(raw string) (TemplateName, error) {
	name := TemplateName(strings.TrimSpace(raw))
	switch name {
	case TemplateContainerizedWeb, TemplateBaremetalWeb, TemplateHybridWeb, TemplateCLI:
		return name, nil
	default:
		return "", fmt.Errorf("unknown template %q", raw)
	}
}

func Scan(root string) ([]Candidate, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve scan root: %w", err)
	}

	candidates := make([]Candidate, 0, 8)
	if err := scanDir(root, root, 0, &candidates); err != nil {
		return nil, err
	}
	return candidates, nil
}

func ClassifyPath(path string) (Candidate, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return Candidate{}, fmt.Errorf("resolve app path: %w", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return Candidate{}, fmt.Errorf("stat app path: %w", err)
	}
	if !info.IsDir() {
		return Candidate{}, fmt.Errorf("app path must be a directory: %s", path)
	}

	candidate, ok, err := detectCandidate(path)
	if err != nil {
		return Candidate{}, err
	}
	if ok {
		return candidate, nil
	}

	return Candidate{
		Path:          path,
		Template:      TemplateCLI,
		Kind:          DetectionCLI,
		Reason:        "Detected: CLI (no compose or framework signals found)",
		FallbackToCLI: true,
	}, nil
}

func scanDir(root, dir string, depth int, candidates *[]Candidate) error {
	if dir != root {
		nestedGitRoot, err := isNestedGitRoot(dir)
		if err != nil {
			return err
		}
		if nestedGitRoot {
			return nil
		}
	}

	candidate, ok, err := detectCandidate(dir)
	if err != nil {
		return err
	}
	if ok {
		*candidates = append(*candidates, candidate)
	}

	if depth >= maxScanDepth {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}

	for _, entry := range entries {
		if !shouldScanEntry(entry) {
			continue
		}

		child := filepath.Join(dir, entry.Name())
		if err := scanDir(root, child, depth+1, candidates); err != nil {
			return err
		}
	}

	return nil
}

func shouldScanEntry(entry os.DirEntry) bool {
	if !entry.IsDir() {
		return false
	}
	if entry.Type()&os.ModeSymlink != 0 {
		return false
	}
	return !shouldSkipTree(entry.Name())
}

func detectCandidate(dir string) (Candidate, bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Candidate{}, false, fmt.Errorf("read %s: %w", dir, err)
	}

	composeSignals := make([]string, 0, 2)
	composeOverrides := make([]string, 0, 1)
	bareSignals := make([]string, 0, 2)
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}

		name := entry.Name()
		switch {
		case isComposeBaseSignal(name):
			composeSignals = append(composeSignals, name)
		case isComposeOverrideSignal(name):
			composeOverrides = append(composeOverrides, name)
		case isBareSignal(name):
			bareSignals = append(bareSignals, name)
		}
	}

	if len(composeSignals) > 0 {
		composeSignals = append(composeSignals, composeOverrides...)
	}

	switch {
	case len(composeSignals) > 0 && len(bareSignals) == 0:
		return buildCandidate(dir, TemplateContainerizedWeb, DetectionContainerized, composeSignals, composeSignals,
			fmt.Sprintf("Detected: containerized (found %s)", strings.Join(composeSignals, ", "))), true, nil
	case len(composeSignals) == 0 && len(bareSignals) > 0:
		return buildCandidate(dir, TemplateBaremetalWeb, DetectionBaremetal, nil, bareSignals,
			fmt.Sprintf("Detected: bare-metal (found %s)", strings.Join(bareSignals, ", "))), true, nil
	case len(composeSignals) > 0 && len(bareSignals) > 0:
		signals := append(append([]string{}, composeSignals...), bareSignals...)
		slices.Sort(signals)
		candidate := buildCandidate(dir, TemplateCLI, DetectionAmbiguous, composeSignals, signals,
			fmt.Sprintf("Detected: ambiguous (found %s)", strings.Join(signals, ", ")))
		candidate.HybridHint = true
		candidate.FallbackToCLI = true
		return candidate, true, nil
	default:
		return Candidate{}, false, nil
	}
}

func buildCandidate(path string, template TemplateName, kind DetectionKind, composeFiles []string, signals []string, reason string) Candidate {
	result := Candidate{
		Path:         path,
		Template:     template,
		Kind:         kind,
		Reason:       reason,
		ComposeFiles: append([]string{}, composeFiles...),
		FoundSignals: append([]string{}, signals...),
	}
	result.DisplaySelection = fmt.Sprintf("%s (%s)", path, template)
	return result
}

func isNestedGitRoot(dir string) (bool, error) {
	repoRoot, ok := gitutil.FindRepoRootOK(dir)
	if !ok {
		return false, nil
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		return false, fmt.Errorf("stat scan directory %s: %w", dir, err)
	}
	repoInfo, err := os.Stat(repoRoot)
	if err != nil {
		return false, fmt.Errorf("stat git repository root %s: %w", repoRoot, err)
	}

	return os.SameFile(dirInfo, repoInfo), nil
}

func shouldSkipTree(name string) bool {
	switch name {
	case ".git", ".devlane", ".direnv", "node_modules", "vendor", "dist", "build", "target", "tmp":
		return true
	default:
		return false
	}
}

func isComposeBaseSignal(name string) bool {
	lower := strings.ToLower(name)
	if !(strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml")) {
		return false
	}
	return (strings.HasPrefix(lower, "compose") || strings.HasPrefix(lower, "docker-compose")) &&
		!strings.HasSuffix(strings.TrimSuffix(lower, filepath.Ext(lower)), ".override")
}

func isComposeOverrideSignal(name string) bool {
	lower := strings.ToLower(name)
	if !(strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml")) {
		return false
	}
	if !(strings.HasPrefix(lower, "compose") || strings.HasPrefix(lower, "docker-compose")) {
		return false
	}
	return strings.HasSuffix(strings.TrimSuffix(lower, filepath.Ext(lower)), ".override")
}

func isBareSignal(name string) bool {
	switch name {
	case "package.json", "Cargo.toml", "go.mod", "Gemfile":
		return true
	default:
		return strings.HasSuffix(name, ".csproj")
	}
}

func selectScanCandidates(cwd string, opts Options, stdin, stdout *os.File) ([]Candidate, error) {
	candidates, err := Scan(cwd)
	if err != nil {
		return nil, err
	}

	switch len(candidates) {
	case 0:
		return []Candidate{{
			Path:          cwd,
			Template:      TemplateCLI,
			Kind:          DetectionCLI,
			Reason:        "Detected: CLI (no compose or framework signals found)",
			FallbackToCLI: true,
		}}, nil
	case 1:
		return candidates, nil
	default:
		printCandidates(candidates, cwd, stdout)
		if opts.All {
			return candidates, nil
		}
		if !isInteractive(stdin) || opts.Yes {
			return nil, errors.New("multiple candidates found; rerun with --all or --app <path>")
		}

		selected, err := promptForSelection(candidates, cwd, stdin, stdout)
		if err != nil {
			return nil, err
		}
		return selected, nil
	}
}

func executeExplicitPath(cwd string, opts Options, stdin, stdout *os.File) (Result, error) {
	target, err := resolveTarget(cwd, opts.AppPath)
	if err != nil {
		return Result{}, err
	}

	candidate, err := ClassifyPath(target)
	if err != nil {
		return Result{}, err
	}
	return writeCandidates([]Candidate{candidate}, opts, stdin, stdout)
}

func executeTemplate(cwd string, opts Options, stdin, stdout *os.File) (Result, error) {
	target, err := resolveTarget(cwd, opts.AppPath)
	if err != nil {
		return Result{}, err
	}

	templateName, err := ParseTemplateName(opts.Template)
	if err != nil {
		return Result{}, err
	}

	candidate, err := ClassifyPath(target)
	if err != nil {
		return Result{}, err
	}
	return writeCandidates(applyTemplateOverride([]Candidate{candidate}, templateName), opts, stdin, stdout)
}

func executeFrom(cwd string, opts Options, stdin, stdout *os.File) (Result, error) {
	target, err := resolveTarget(cwd, opts.AppPath)
	if err != nil {
		return Result{}, err
	}

	return executeFromTargets(cwd, opts, []string{target}, stdin, stdout)
}

func executeTemplateCandidates(opts Options, candidates []Candidate, stdin, stdout *os.File) (Result, error) {
	templateName, err := ParseTemplateName(opts.Template)
	if err != nil {
		return Result{}, err
	}

	return writeCandidates(applyTemplateOverride(candidates, templateName), opts, stdin, stdout)
}

func applyTemplateOverride(candidates []Candidate, templateName TemplateName) []Candidate {
	overridden := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		candidate.Template = templateName
		candidate.Reason = fmt.Sprintf("Using template override: %s", templateName)
		candidate.HybridHint = false
		candidate.FallbackToCLI = false
		candidate.DisplaySelection = fmt.Sprintf("%s (%s)", candidate.Path, templateName)
		overridden = append(overridden, candidate)
	}
	return overridden
}

func executeFromCandidates(cwd string, opts Options, candidates []Candidate, stdin, stdout *os.File) (Result, error) {
	targets := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		targets = append(targets, candidate.Path)
	}
	return executeFromTargets(cwd, opts, targets, stdin, stdout)
}

func executeFromTargets(cwd string, opts Options, targets []string, stdin, stdout *os.File) (Result, error) {
	sourcePath, err := resolveInputPath(cwd, opts.From)
	if err != nil {
		return Result{}, fmt.Errorf("resolve --from path: %w", err)
	}

	adapter, err := config.LoadAdapter(sourcePath)
	if err != nil {
		return Result{}, fmt.Errorf("validate source adapter: %w", err)
	}

	payload, err := os.ReadFile(sourcePath)
	if err != nil {
		return Result{}, fmt.Errorf("read source adapter: %w", err)
	}

	plans := make([]writePlan, 0, len(targets))
	validations := make([]copiedAdapterValidation, 0, len(targets))
	for _, target := range targets {
		validation := validateCopiedAdapter(adapter, target)
		if err := validation.Err(target); err != nil {
			return Result{}, err
		}

		plan, err := planWrite(target, payload, nil, opts)
		if err != nil {
			return Result{}, err
		}
		plans = append(plans, plan)
		validations = append(validations, validation)
	}

	if err := confirmWrites(plans, opts, stdin, stdout); err != nil {
		return Result{}, err
	}
	if err := commitWrites(plans); err != nil {
		return Result{}, err
	}

	messages := make([]string, 0, len(targets)*8)
	for index, target := range targets {
		messages = append(messages, fmt.Sprintf("wrote starter adapter from %s to %s", sourcePath, filepath.Join(target, "devlane.yaml")))
		messages = append(messages, reviewChecklist(adapter, target)...)
		messages = append(messages, validations[index].Warnings...)
	}
	return Result{Messages: messages}, nil
}

func writeCandidates(candidates []Candidate, opts Options, stdin, stdout *os.File) (Result, error) {
	plans := make([]writePlan, 0, len(candidates))
	messages := make([]string, 0, len(candidates)*3)
	cwd := opts.CWD
	if absoluteCWD, err := filepath.Abs(opts.CWD); err == nil {
		cwd = absoluteCWD
	}

	for _, candidate := range candidates {
		content, err := ScaffoldTemplate(candidate.Template, filepath.Base(candidate.Path), candidate.ComposeFiles)
		if err != nil {
			return Result{}, err
		}

		targetMessages := candidateMessages(candidate, cwd)
		plan, err := planWrite(candidate.Path, []byte(content), targetMessages, opts)
		if err != nil {
			return Result{}, err
		}
		plans = append(plans, plan)
	}

	if err := confirmWrites(plans, opts, stdin, stdout); err != nil {
		return Result{}, err
	}
	if err := commitWrites(plans); err != nil {
		return Result{}, err
	}

	for _, plan := range plans {
		messages = append(messages, plan.Messages...)
		messages = append(messages, fmt.Sprintf("wrote %s", filepath.Join(plan.Target, "devlane.yaml")))
	}

	return Result{Messages: messages}, nil
}

func listCandidates(cwd string, stdout *os.File) (Result, error) {
	candidates, err := Scan(cwd)
	if err != nil {
		return Result{}, err
	}

	if len(candidates) == 0 {
		fmt.Fprintf(stdout, "No candidates detected under %s\n", cwd)
		fmt.Fprintf(stdout, "Fallback: cli at %s\n", cwd)
		return Result{}, nil
	}

	printCandidates(candidates, cwd, stdout)
	return Result{}, nil
}

func printCandidates(candidates []Candidate, cwd string, stdout *os.File) {
	fmt.Fprintln(stdout, "Detected candidates:")
	for index, candidate := range candidates {
		relative := displayPath(cwd, candidate.Path)
		fmt.Fprintf(stdout, "%d. %s [%s]\n", index+1, relative, candidate.Template)
		fmt.Fprintf(stdout, "   %s\n", candidate.Reason)
		if candidate.HybridHint {
			fmt.Fprintln(stdout, "   Notice: mixed compose + bare-metal signals; use --template hybrid-web if intentional.")
		}
	}
}

func promptForSelection(candidates []Candidate, cwd string, stdin, stdout *os.File) ([]Candidate, error) {
	reader := bufio.NewReader(stdin)
	for {
		fmt.Fprint(stdout, "Select candidate number, 'all', or 'q': ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read selection: %w", err)
		}

		choice := strings.TrimSpace(line)
		switch choice {
		case "q", "quit":
			return nil, errors.New("init cancelled")
		case "all":
			return candidates, nil
		}

		index := parseSelection(choice)
		if index >= 0 && index < len(candidates) {
			fmt.Fprintf(stdout, "Selected %s\n", displayPath(cwd, candidates[index].Path))
			return []Candidate{candidates[index]}, nil
		}
		fmt.Fprintln(stdout, "Invalid selection")
	}
}

func parseSelection(value string) int {
	var index int
	if _, err := fmt.Sscanf(value, "%d", &index); err != nil {
		return -1
	}
	return index - 1
}

func planWrite(target string, payload []byte, messages []string, opts Options) (writePlan, error) {
	info, err := os.Stat(target)
	if err != nil {
		return writePlan{}, fmt.Errorf("stat target %s: %w", target, err)
	}
	if !info.IsDir() {
		return writePlan{}, fmt.Errorf("target must be a directory: %s", target)
	}

	configPath := filepath.Join(target, "devlane.yaml")
	configInfo, err := os.Lstat(configPath)
	if err == nil {
		if !configInfo.Mode().IsRegular() {
			return writePlan{}, fmt.Errorf("refusing to overwrite non-regular existing %s", configPath)
		}
		if !opts.Force {
			return writePlan{}, fmt.Errorf("refusing to overwrite existing %s without --force", configPath)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return writePlan{}, fmt.Errorf("stat existing %s: %w", configPath, err)
	}

	return writePlan{
		Target:   target,
		Content:  payload,
		Messages: messages,
	}, nil
}

func confirmWrites(plans []writePlan, opts Options, stdin, stdout *os.File) error {
	if opts.Yes || opts.All || !isInteractive(stdin) {
		return nil
	}

	reader := bufio.NewReader(stdin)
	for _, plan := range plans {
		fmt.Fprintf(stdout, "Write %s? [y/N]: ", filepath.Join(plan.Target, "devlane.yaml"))
		line, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read confirmation: %w", err)
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer != "y" && answer != "yes" {
			return errors.New("init cancelled")
		}
	}
	return nil
}

func commitWrites(plans []writePlan) error {
	for _, plan := range plans {
		configPath := filepath.Join(plan.Target, "devlane.yaml")
		if err := os.WriteFile(configPath, plan.Content, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", configPath, err)
		}
	}
	return nil
}

func candidateMessages(candidate Candidate, cwd string) []string {
	messages := []string{candidate.Reason}
	if candidate.Path != cwd {
		messages = append(messages, fmt.Sprintf("Selected app root: %s", displayPath(cwd, candidate.Path)))
	}
	if candidate.HybridHint {
		messages = append(messages, "Notice: overlapping compose + bare-metal signals default to the cli template; use --template hybrid-web for an intentional mixed setup.")
	} else if candidate.FallbackToCLI {
		messages = append(messages, "Notice: no confident app signals found; wrote the cli template. Use --template baremetal-web or --template containerized-web if needed.")
	}
	return messages
}

func reviewChecklist(adapter *config.AdapterConfig, target string) []string {
	messages := []string{"Review copied adapter fields that may be repo-coupled:"}
	if adapter.App != "" {
		messages = append(messages, fmt.Sprintf("- app: %s", adapter.App))
	}
	messages = append(messages,
		fmt.Sprintf("- lane.path_roots.state: %s", adapter.Lane.PathRoots.State),
		fmt.Sprintf("- lane.path_roots.cache: %s", adapter.Lane.PathRoots.Cache),
		fmt.Sprintf("- lane.path_roots.runtime: %s", adapter.Lane.PathRoots.Runtime),
	)
	if adapter.Lane.HostPatterns.Stable != "" || adapter.Lane.HostPatterns.Dev != "" {
		messages = append(messages, "- lane.host_patterns")
	}
	if len(adapter.Runtime.ComposeFiles) > 0 {
		messages = append(messages, "- runtime.compose_files")
	}
	if adapter.Outputs.ManifestPath != "" {
		messages = append(messages, fmt.Sprintf("- outputs.manifest_path: %s", adapter.Outputs.ManifestPath))
	}
	if adapter.Outputs.ComposeEnvPath != "" {
		messages = append(messages, fmt.Sprintf("- outputs.compose_env_path: %s", adapter.Outputs.ComposeEnvPath))
	}
	for _, output := range adapter.Outputs.Generated {
		messages = append(messages, fmt.Sprintf("- outputs.generated: %s -> %s", output.Template, output.Destination))
	}
	for _, seed := range adapter.Worktree.Seed {
		messages = append(messages, fmt.Sprintf("- worktree.seed: %s", seed))
	}
	messages = append(messages, fmt.Sprintf("Target repo: %s", target))
	return messages
}

type copiedAdapterValidation struct {
	Warnings []string
	Errors   []string
}

func (v copiedAdapterValidation) Err(target string) error {
	if len(v.Errors) == 0 {
		return nil
	}

	return fmt.Errorf(
		"copied adapter contains repo-relative paths that are invalid for target repo %s:\n- %s",
		target,
		strings.Join(v.Errors, "\n- "),
	)
}

func validateCopiedAdapter(adapter *config.AdapterConfig, target string) copiedAdapterValidation {
	adapterRoot := target
	repoRoot := gitutil.FindRepoRoot(target)
	validation := copiedAdapterValidation{
		Warnings: make([]string, 0, 8),
		Errors:   make([]string, 0, 8),
	}

	validateCopiedAdapterPath(&validation, adapterRoot, repoRoot, adapter.Lane.PathRoots.State, "lane.path_roots.state", false, "")
	validateCopiedAdapterPath(&validation, adapterRoot, repoRoot, adapter.Lane.PathRoots.Cache, "lane.path_roots.cache", false, "")
	validateCopiedAdapterPath(&validation, adapterRoot, repoRoot, adapter.Lane.PathRoots.Runtime, "lane.path_roots.runtime", false, "")
	validateCopiedAdapterPath(&validation, adapterRoot, repoRoot, adapter.Outputs.ManifestPath, "outputs.manifest_path", false, "")
	if adapter.Outputs.ComposeEnvPath != "" {
		validateCopiedAdapterPath(&validation, adapterRoot, repoRoot, adapter.Outputs.ComposeEnvPath, "outputs.compose_env_path", false, "")
	}

	for _, composeFile := range adapter.Runtime.ComposeFiles {
		validateCopiedAdapterPath(&validation, adapterRoot, repoRoot, composeFile, "runtime.compose_files", true, "compose file")
	}

	for _, output := range adapter.Outputs.Generated {
		validateCopiedAdapterPath(&validation, adapterRoot, repoRoot, output.Template, "outputs.generated[].template", true, "template")
		validateCopiedAdapterPath(&validation, adapterRoot, repoRoot, output.Destination, "outputs.generated[].destination", false, "")
	}

	for _, seed := range adapter.Worktree.Seed {
		validateCopiedAdapterPath(&validation, adapterRoot, repoRoot, seed, "worktree.seed", true, "worktree seed path")
	}

	return validation
}

func validateCopiedAdapterPath(validation *copiedAdapterValidation, adapterRoot, repoRoot, raw, field string, warnIfMissing bool, missingLabel string) {
	if raw == "" {
		return
	}

	resolved, err := util.ResolveAdapterPath(adapterRoot, repoRoot, raw)
	if err != nil {
		validation.Errors = append(validation.Errors, fmt.Sprintf("%s %q: %v", field, raw, err))
		return
	}

	if !warnIfMissing {
		return
	}

	if _, statErr := os.Stat(resolved); os.IsNotExist(statErr) {
		validation.Warnings = append(validation.Warnings, fmt.Sprintf("warning: copied adapter references missing %s in target repo: %s", missingLabel, raw))
	}
}

func resolveTarget(cwd, raw string) (string, error) {
	if raw == "" {
		return cwd, nil
	}

	if filepath.IsAbs(raw) {
		return "", fmt.Errorf("app path must be relative to --cwd: %s", raw)
	}

	target := filepath.Clean(filepath.Join(cwd, raw))
	relative, err := filepath.Rel(cwd, target)
	if err != nil {
		return "", fmt.Errorf("resolve app path relative to --cwd: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, fmt.Sprintf("..%c", os.PathSeparator)) {
		return "", fmt.Errorf("app path escapes --cwd: %s", raw)
	}
	if err := validateResolvedTarget(cwd, target, raw); err != nil {
		return "", err
	}
	return target, nil
}

func validateResolvedTarget(cwd, target, raw string) error {
	resolvedCWD, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return fmt.Errorf("resolve --cwd symlinks: %w", err)
	}

	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("resolve app path symlinks: %w", err)
	}

	if !util.IsWithin(resolvedCWD, resolvedTarget) {
		return fmt.Errorf("app path escapes --cwd via symlink: %s", raw)
	}

	return nil
}

func resolveInputPath(cwd, raw string) (string, error) {
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw), nil
	}
	return filepath.Abs(filepath.Join(cwd, raw))
}

func validateOptions(opts Options) error {
	if opts.Template != "" && opts.From != "" {
		return errors.New("--template and --from cannot be combined")
	}
	if opts.List && (opts.Template != "" || opts.From != "" || opts.AppPath != "") {
		return errors.New("--list cannot be combined with --template, --from, or --app")
	}
	if opts.All && opts.AppPath != "" {
		return errors.New("--all cannot be combined with --app")
	}
	return nil
}

func displayPath(cwd, target string) string {
	relative, err := filepath.Rel(cwd, target)
	if err != nil {
		return target
	}
	if relative == "." {
		return target
	}
	return relative
}

func isInteractive(stdin *os.File) bool {
	if stdin == nil {
		return false
	}

	info, err := stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
