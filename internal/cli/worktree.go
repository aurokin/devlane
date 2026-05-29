package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/auro/devlane/internal/config"
	"github.com/auro/devlane/internal/gitutil"
	"github.com/auro/devlane/internal/portalloc"
	"github.com/auro/devlane/internal/util"
	"github.com/auro/devlane/internal/write"
)

func runWorktree(args []string) int {
	if len(args) == 0 {
		printWorktreeUsage()
		return 2
	}

	switch args[0] {
	case "create":
		return runWorktreeCreate(args[1:])
	case "remove":
		return runWorktreeRemove(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown worktree subcommand %q\n", args[0])
		printWorktreeUsage()
		return 2
	}
}

func printWorktreeUsage() {
	fmt.Fprintln(os.Stderr, "usage: devlane worktree <create|remove> [flags] <lane>")
}

// worktreeValueFlags are the worktree flags that take a following value, used by
// reorderWorktreeArgs so flags may appear after the positional <lane>.
var worktreeValueFlags = map[string]struct{}{
	"config": {},
	"cwd":    {},
	"path":   {},
}

// reorderWorktreeArgs moves flags ahead of the positional <lane> so the stdlib
// flag parser (which stops at the first non-flag argument) accepts a lane that
// precedes its flags, e.g. `worktree create feature/x --cwd .`. It mirrors the
// forgiving argument handling of the port command.
func reorderWorktreeArgs(args []string, valueFlags map[string]struct{}) []string {
	flagArgs := make([]string, 0, len(args))
	positional := make([]string, 0, 1)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}
		flagArgs = append(flagArgs, arg)
		name := strings.TrimLeft(arg, "-")
		if _, ok := valueFlags[name]; ok && i+1 < len(args) {
			i++
			flagArgs = append(flagArgs, args[i])
		}
	}
	return append(flagArgs, positional...)
}

// worktreeContext is the resolved adapter/repo state shared by both worktree
// subcommands. Worktree lifecycle is only supported when the adapter lives at
// the Git worktree root; subtree adapters in monorepos are out of scope.
type worktreeContext struct {
	adapter     *config.AdapterConfig
	configBase  string
	adapterRoot string
	repoRoot    string
}

func loadWorktreeContext(configPath, cwd string) (worktreeContext, error) {
	flags := &commonFlags{config: configPath, cwd: cwd}
	cwdAbs, resolvedConfig, adapter, err := loadAdapter(flags)
	if err != nil {
		return worktreeContext{}, err
	}

	repoRoot, ok := gitutil.FindRepoRootOK(cwdAbs)
	if !ok {
		return worktreeContext{}, fmt.Errorf("not inside a Git repository: %s", cwdAbs)
	}

	adapterRoot := filepath.Dir(resolvedConfig)
	if !sameCatalogRepoPath(adapterRoot, repoRoot) {
		return worktreeContext{}, fmt.Errorf(
			"worktree commands require the adapter at the Git worktree root, but the adapter at %s sits below the worktree root %s; this is a subtree adapter — manage its worktrees with `git worktree` directly",
			adapterRoot, repoRoot)
	}

	return worktreeContext{
		adapter:     adapter,
		configBase:  filepath.Base(resolvedConfig),
		adapterRoot: adapterRoot,
		repoRoot:    repoRoot,
	}, nil
}

func runWorktreeCreate(args []string) int {
	fs := flag.NewFlagSet("worktree create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var configPath, cwd string
	fs.StringVar(&configPath, "config", "devlane.yaml", "Path to devlane.yaml")
	fs.StringVar(&cwd, "cwd", ".", "Working directory used for discovery")
	if err := fs.Parse(reorderWorktreeArgs(args, worktreeValueFlags)); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: devlane worktree create [flags] <lane>")
		return 2
	}
	lane := fs.Arg(0)

	ctx, err := loadWorktreeContext(configPath, cwd)
	if err != nil {
		return exitError(err)
	}
	if err := validateNewLane(ctx.adapter, ctx.repoRoot, lane); err != nil {
		return exitError(err)
	}

	target := siblingWorktreePath(ctx.repoRoot, util.Slugify(lane, false))
	if pathExists(target) {
		return exitError(fmt.Errorf("target worktree path already exists: %s", target))
	}
	if gitutil.BranchExists(ctx.repoRoot, lane) {
		return exitError(fmt.Errorf("branch %q already exists; worktree create only creates new lanes", lane))
	}

	// Validate the seed declaration statically before touching git, so an
	// adapter with an unusable seed entry fails without leaving an orphan
	// checkout behind.
	plan, err := planSeeds(ctx.adapter, ctx.adapterRoot)
	if err != nil {
		return exitError(err)
	}

	if err := gitutil.AddWorktree(ctx.repoRoot, target, lane, "HEAD"); err != nil {
		return exitError(fmt.Errorf("create worktree: %w", err))
	}

	copied, skipped, missing, err := applySeeds(plan, target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "seed copy failed: %v\n", err)
		printWorktreeRecovery(target)
		return 1
	}

	messages, err := prepareCreatedWorktree(target, ctx.configBase)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepare in the new worktree failed: %v\n", err)
		printWorktreeRecovery(target)
		return 1
	}

	reportSeeds(copied, skipped, missing)
	for _, message := range messages {
		fmt.Println(message)
	}
	fmt.Printf("created worktree for lane %q at %s\n", lane, target)
	return 0
}

func runWorktreeRemove(args []string) int {
	fs := flag.NewFlagSet("worktree remove", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var configPath, cwd, path string
	var force bool
	fs.StringVar(&configPath, "config", "devlane.yaml", "Path to devlane.yaml")
	fs.StringVar(&cwd, "cwd", ".", "Working directory used for discovery")
	fs.StringVar(&path, "path", "", "Explicit worktree path for a manually moved or renamed worktree")
	fs.BoolVar(&force, "force", false, "Discard uncommitted and untracked changes when removing the worktree")
	if err := fs.Parse(reorderWorktreeArgs(args, worktreeValueFlags)); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: devlane worktree remove [flags] <lane>")
		return 2
	}
	lane := fs.Arg(0)

	ctx, err := loadWorktreeContext(configPath, cwd)
	if err != nil {
		return exitError(err)
	}

	explicit := strings.TrimSpace(path) != ""
	target := strings.TrimSpace(path)
	if !explicit {
		target = siblingWorktreePath(ctx.repoRoot, util.Slugify(lane, false))
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return exitError(fmt.Errorf("resolve worktree path: %w", err))
	}
	targetAbs = filepath.Clean(targetAbs)

	if !pathExists(targetAbs) {
		if explicit {
			return exitError(fmt.Errorf("worktree path does not exist: %s", targetAbs))
		}
		return exitError(fmt.Errorf("no worktree found for lane %q at %s; pass --path if it was moved or renamed", lane, targetAbs))
	}

	// Capture the identity key before removal so scoped cleanup still works
	// after the directory is gone. Canonicalize while the path still exists:
	// prepare stored catalog rows under the symlink-resolved repo root, and
	// sameCatalogRepoPath cannot resolve symlinks once git deletes the worktree.
	app := ctx.adapter.App
	repoPath := targetAbs
	if canonical, canonErr := util.CanonicalPath(targetAbs); canonErr == nil {
		repoPath = canonical
	}

	if err := gitutil.RemoveWorktree(ctx.repoRoot, targetAbs, force); err != nil {
		return exitError(fmt.Errorf("remove worktree: %w (re-run with --force to discard uncommitted or untracked changes, including prepare's generated outputs)", err))
	}

	removed, err := removeScopedAllocations(app, repoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "removed worktree %s but scoped catalog cleanup failed: %v\n", targetAbs, err)
		fmt.Fprintf(os.Stderr, "run `devlane host gc --app %s` to drop the orphaned rows\n", app)
		return 1
	}

	fmt.Printf("removed worktree %s and %d catalog allocation(s) for app %q\n", targetAbs, removed, app)
	return 0
}

func validateNewLane(adapter *config.AdapterConfig, repoRoot, lane string) error {
	trimmed := strings.TrimSpace(lane)
	if trimmed == "" {
		return errors.New("lane name must not be empty")
	}
	if strings.EqualFold(trimmed, strings.TrimSpace(adapter.Lane.StableName)) {
		return fmt.Errorf("lane %q is the stable lane; worktree create is for new dev lanes only", lane)
	}
	for _, stableBranch := range adapter.Lane.StableBranches {
		if trimmed == strings.TrimSpace(stableBranch) {
			return fmt.Errorf("lane %q matches lane.stable_branches and would prepare as the stable lane; worktree create is for new dev lanes only", lane)
		}
	}
	if util.Slugify(lane, false) == "" {
		return fmt.Errorf("lane %q does not produce a usable path slug", lane)
	}
	if !gitutil.IsValidBranchName(repoRoot, lane) {
		return fmt.Errorf("lane %q is not a valid Git branch name", lane)
	}
	return nil
}

func siblingWorktreePath(repoRoot, slug string) string {
	return filepath.Join(filepath.Dir(repoRoot), filepath.Base(repoRoot)+"-"+slug)
}

// seedAction is one resolved worktree.seed entry. rel is the repoRoot-relative
// path (used for both the destination and reporting); generated marks an entry
// that overlaps a generated output and must be skipped (prepare renders it).
type seedAction struct {
	rel       string
	src       string
	generated bool
}

// planSeeds resolves and classifies every worktree.seed entry against the
// adapter root. Worktree commands require adapterRoot == repoRoot, so the
// adapter root is the single containment boundary; using it for both the
// containment check and the relative-path computation keeps paths in one
// symlink form (mixing it with a git-resolved repo root would make
// filepath.Rel emit a `..`-laden path on platforms where /tmp is a symlink).
func planSeeds(adapter *config.AdapterConfig, adapterRoot string) ([]seedAction, error) {
	generated := generatedDestinations(adapter)
	actions := make([]seedAction, 0, len(adapter.Worktree.Seed))
	for _, seed := range adapter.Worktree.Seed {
		if strings.TrimSpace(seed) == "" {
			continue
		}
		if filepath.IsAbs(seed) {
			return nil, fmt.Errorf("worktree.seed entry %q must be a repo-relative path, not absolute", seed)
		}
		src, err := util.ResolveAdapterPath(adapterRoot, adapterRoot, seed)
		if err != nil {
			return nil, fmt.Errorf("worktree.seed entry %q: %w", seed, err)
		}
		rel, err := filepath.Rel(adapterRoot, src)
		if err != nil {
			return nil, fmt.Errorf("worktree.seed entry %q: %w", seed, err)
		}
		if rel == "." {
			return nil, fmt.Errorf("worktree.seed entry %q must reference a path inside the repo, not the repo root", seed)
		}
		_, isGenerated := generated[rel]
		actions = append(actions, seedAction{rel: rel, src: src, generated: isGenerated})
	}
	return actions, nil
}

// generatedDestinations returns the cleaned repoRoot-relative destinations of
// every generated output. Because worktree commands require adapterRoot ==
// repoRoot, an adapterRoot-relative destination is already repoRoot-relative.
func generatedDestinations(adapter *config.AdapterConfig) map[string]struct{} {
	set := make(map[string]struct{}, len(adapter.Outputs.Generated))
	for _, output := range adapter.Outputs.Generated {
		if strings.TrimSpace(output.Destination) == "" {
			continue
		}
		set[filepath.Clean(output.Destination)] = struct{}{}
	}
	return set
}

func applySeeds(actions []seedAction, target string) (copied, skipped, missing []string, err error) {
	for _, action := range actions {
		dst := filepath.Join(target, action.rel)
		if !util.IsWithin(target, dst) {
			return copied, skipped, missing, fmt.Errorf("worktree.seed entry %q would escape the new worktree root", action.rel)
		}
		if action.generated {
			skipped = append(skipped, action.rel)
			continue
		}

		info, statErr := os.Lstat(action.src)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				missing = append(missing, action.rel)
				continue
			}
			return copied, skipped, missing, fmt.Errorf("stat seed %q: %w", action.rel, statErr)
		}
		if err := copyTree(action.src, dst, info); err != nil {
			return copied, skipped, missing, fmt.Errorf("copy seed %q: %w", action.rel, err)
		}
		copied = append(copied, action.rel)
	}
	return copied, skipped, missing, nil
}

func copyTree(src, dst string, info os.FileInfo) error {
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return copySymlink(src, dst)
	case info.IsDir():
		return copyDir(src, dst)
	default:
		return copyFile(src, dst, info.Mode())
	}
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, rel)

		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			return copySymlink(path, targetPath)
		case entry.IsDir():
			return os.MkdirAll(targetPath, 0o755)
		default:
			return copyFile(path, targetPath, info.Mode())
		}
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	// Remove any existing destination first so the write never follows a
	// pre-existing symlink out of the worktree (O_TRUNC would truncate the
	// link target), matching copySymlink's deterministic-overwrite behavior.
	if err := os.Remove(dst); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	// Pin the source bits explicitly; O_CREATE honors the umask.
	return os.Chmod(dst, mode.Perm())
}

func copySymlink(src, dst string) error {
	linkTarget, err := os.Readlink(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	// Overwrite any existing destination so re-seeding is deterministic.
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return os.Symlink(linkTarget, dst)
}

func reportSeeds(copied, skipped, missing []string) {
	for _, rel := range copied {
		fmt.Printf("seeded %s\n", rel)
	}
	for _, rel := range skipped {
		fmt.Printf("skipped seed %s (a generated output; prepare renders it)\n", rel)
	}
	for _, rel := range missing {
		fmt.Printf("warning: seed source %s is missing in the source checkout; skipped\n", rel)
	}
}

func prepareCreatedWorktree(worktreePath, configBase string) ([]string, error) {
	flags := &commonFlags{config: configBase, cwd: worktreePath}
	_, _, adapter, laneManifest, err := load(flags)
	if err != nil {
		return nil, err
	}

	lane := portallocLane(adapter, laneManifest)
	needs, err := needsCatalogPrepare(adapter, lane)
	if err != nil {
		return nil, err
	}
	if needs {
		messages, _, err := executeCatalogPrepare(adapter, laneManifest, lane)
		return messages, err
	}

	result, err := write.Prepare(laneManifest, adapter)
	if err != nil {
		return nil, err
	}
	return result.Messages, nil
}

func removeScopedAllocations(app, repoPath string) (int, error) {
	removed := 0
	err := mutateCatalog(func(snap *portalloc.Snapshot) error {
		kept := snap.Allocations[:0]
		for _, row := range snap.Allocations {
			if row.App == app && sameCatalogRepoPath(row.RepoPath, repoPath) {
				removed++
				continue
			}
			kept = append(kept, row)
		}
		snap.Allocations = kept
		return nil
	})
	if err != nil {
		return 0, err
	}
	return removed, nil
}

func printWorktreeRecovery(target string) {
	fmt.Fprintf(os.Stderr, "the new checkout remains at %s; fix the issue and run `devlane prepare --cwd %s`, or run `git worktree remove --force %s` to abandon it\n", target, target, target)
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
