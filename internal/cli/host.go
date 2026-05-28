package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/auro/devlane/internal/config"
	"github.com/auro/devlane/internal/drift"
	"github.com/auro/devlane/internal/portalloc"
)

func runHost(args []string) int {
	if len(args) == 0 {
		printHostUsage()
		return 2
	}

	switch args[0] {
	case "status":
		return runHostStatus(args[1:])
	case "doctor":
		return runHostDoctor(args[1:])
	case "gc":
		return runHostGc(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown host subcommand %q\n", args[0])
		printHostUsage()
		return 2
	}
}

func printHostUsage() {
	fmt.Fprintln(os.Stderr, "usage: devlane host <status|doctor|gc> [flags]")
}

func runHostStatus(args []string) int {
	fs := flag.NewFlagSet("host status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "host status takes no positional arguments\n")
		return 2
	}

	rows, err := portalloc.List()
	if err != nil {
		return exitError(err)
	}

	if len(rows) == 0 {
		fmt.Println("no allocations")
		return 0
	}

	sort.Slice(rows, func(i, j int) bool {
		left := rows[i]
		right := rows[j]
		if left.App != right.App {
			return left.App < right.App
		}
		if left.RepoPath != right.RepoPath {
			return left.RepoPath < right.RepoPath
		}
		return left.Service < right.Service
	})

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "APP\tMODE\tLANE\tSERVICE\tPORT\tREPO PATH")
	for _, row := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n",
			row.App, row.Mode, row.Lane, row.Service, row.Port, row.RepoPath)
	}
	if err := w.Flush(); err != nil {
		return exitError(err)
	}
	return 0
}

// runHostDoctor is a read-only audit of the host catalog. It loads each row's
// adapter through the drift module, prints any findings with their category and
// allocation context (including a bound/free probe), and never mutates the
// catalog. It exits 1 when any finding exists and 0 on a clean catalog.
func runHostDoctor(args []string) int {
	fs := flag.NewFlagSet("host doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "host doctor takes no positional arguments\n")
		return 2
	}

	rows, err := portalloc.List()
	if err != nil {
		return exitError(err)
	}

	findings := drift.Detect(rows, loadRepoAdapters)
	if len(findings) == 0 {
		fmt.Println("no drift detected")
		return 0
	}

	if err := writeFindingTable(os.Stdout, findings); err != nil {
		return exitError(err)
	}
	return 1
}

// findingTableHeader is the shared column header for the drift finding table
// rendered by both `host doctor` and `host gc`. The fixed leading columns
// (through PROBE) keep a stable position so output remains parseable.
const findingTableHeader = "CATEGORY\tAPP\tMODE\tLANE\tSERVICE\tPORT\tPROBE\tREPO PATH\tDETAIL"

// writeFindingTable renders findings as a tab-aligned table to out, sharing one
// layout between doctor and gc so both render findings identically.
func writeFindingTable(out io.Writer, findings []drift.Finding) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, findingTableHeader)
	for _, f := range findings {
		writeFindingRow(w, f)
	}
	return w.Flush()
}

func writeFindingRow(w io.Writer, f drift.Finding) {
	a := f.Allocation
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
		f.Category, a.App, a.Mode, a.Lane, a.Service, a.Port, probeLabel(a.Port), a.RepoPath, f.Detail)
}

// probeLabel reports whether a finding's port is currently bound. Probing is
// context only: it never creates or suppresses a finding, so a
// bound-but-singly-claimed row is shown here only when it already drifted for
// another reason.
func probeLabel(port int) string {
	if portalloc.Probe(port) != nil {
		return "bound"
	}
	return "free"
}

// loadRepoAdapters is the drift module's filesystem boundary for host doctor. A
// catalog row's repoPath is the Git worktree root, which may host several
// subtree adapters in a monorepo, so this discovers every devlane.yaml under the
// root and aggregates each adapter's app -> declared services.
//
// Discovery is deliberately conservative about uncertainty so a transient
// failure never makes a healthy row look removable to `host gc`: the walk skips
// the same trees as `init` and stops at nested Git roots (a nested checkout is a
// different worktree with its own catalog rows), but any unreadable directory or
// adapter that fails to load is recorded in RepoAdapters.LoadErr. A row whose
// app is not otherwise accounted for is then reported bad-adapter, never the
// removable app-mismatch. The walk is unbounded in depth because prepare records
// repoPath as the worktree root regardless of how deep the adapter lives.
//
// A row's adapter path is not recorded in the catalog (Allocation carries only
// repoPath), which is why discovery is required; recording configPath in the
// catalog would let doctor load the exact adapter directly.
func loadRepoAdapters(repoPath string) (drift.RepoAdapters, error) {
	info, err := os.Stat(repoPath)
	if err != nil {
		// A missing worktree root surfaces os.ErrNotExist -> missing-repoPath;
		// any other stat error is non-NotExist -> bad-adapter.
		return drift.RepoAdapters{}, err
	}
	if !info.IsDir() {
		return drift.RepoAdapters{}, fmt.Errorf("%w: repo path is not a directory: %s", os.ErrNotExist, repoPath)
	}

	result := drift.RepoAdapters{Services: map[string]map[string]struct{}{}}
	walkRepoAdapters(repoPath, &result)
	return result, nil
}

func walkRepoAdapters(dir string, result *drift.RepoAdapters) {
	loadAdapterAt(dir, result)

	entries, err := os.ReadDir(dir)
	if err != nil {
		// An unreadable directory is uncertainty, not absence: record it so an
		// adapter that might live below is never silently treated as gone.
		recordScanError(result, err)
		return
	}
	for _, entry := range entries {
		// Descend only into real subdirectories. Symlinks are skipped explicitly
		// (matching init's "does not follow symlinks" rule); this keeps a cyclic
		// directory symlink from causing unbounded recursion and stops doctor
		// from loading an adapter through an out-of-tree symlink alias. IsDir is
		// reliable here even on filesystems without d_type: os.ReadDir resolves
		// the unknown-type sentinel by falling back to lstat.
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || shouldSkipDoctorTree(entry.Name()) {
			continue
		}
		child := filepath.Join(dir, entry.Name())
		if isNestedGitRoot(child) {
			continue
		}
		walkRepoAdapters(child, result)
	}
}

func loadAdapterAt(dir string, result *drift.RepoAdapters) {
	configPath := filepath.Join(dir, "devlane.yaml")
	info, err := os.Stat(configPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			recordScanError(result, fmt.Errorf("stat %s: %w", configPath, err))
		}
		return
	}
	if info.IsDir() {
		// A devlane.yaml that is a directory (or a symlink to one) is an
		// anomalous adapter path, not an absent one: record it so the row is
		// surfaced as bad-adapter rather than the removable app-mismatch.
		recordScanError(result, fmt.Errorf("adapter path %s is a directory, not a file", configPath))
		return
	}

	adapter, err := config.LoadAdapter(configPath)
	if err != nil {
		recordScanError(result, err)
		return
	}

	// Services aggregates declarations across every adapter under the repoPath,
	// unioned by app. The catalog records only (app, repoPath, service) — never
	// which adapter produced a row — so a row is healthy exactly when some
	// adapter under its repoPath declares its (app, service). Attributing a row
	// to one specific adapter is impossible here; guessing would risk a
	// false-positive removable finding for an allocation a sibling adapter with
	// the same app still satisfies, so the union is deliberate.
	services := result.Services[adapter.App]
	if services == nil {
		services = map[string]struct{}{}
		result.Services[adapter.App] = services
	}
	for _, portConfig := range adapter.Ports {
		services[portConfig.Name] = struct{}{}
	}
}

func recordScanError(result *drift.RepoAdapters, err error) {
	if result.LoadErr == nil {
		result.LoadErr = err
	}
}

// isNestedGitRoot reports whether dir is the root of a Git checkout nested under
// the worktree being scanned. Such a checkout is a different worktree root with
// its own catalog rows, so doctor must not attribute its adapters to the parent.
//
// Skipping these is correct — not a false-positive source — because prepare
// records repoPath as `git rev-parse --show-toplevel` from the adapter's
// location, which inside any nested checkout (a submodule or a linked worktree,
// both of which use a `.git` file with "gitdir:") is that checkout's own root,
// not the parent's. So an adapter inside a submodule produces rows keyed to the
// submodule root, audited when that root is itself the scanned repoPath; no
// parent row ever depends on it.
//
// Detection is structural and never shells out to git, so doctor does not depend
// on a git binary being installed: a real checkout root has either a .git
// directory containing HEAD, or a .git file beginning with "gitdir:". A stray or
// empty .git entry is therefore not mistaken for a checkout, so skipping a real
// subtree never hides a present adapter behind a removable app-mismatch.
func isNestedGitRoot(dir string) bool {
	gitPath := filepath.Join(dir, ".git")
	info, err := os.Lstat(gitPath)
	if err != nil {
		return false
	}
	if info.IsDir() {
		_, headErr := os.Stat(filepath.Join(gitPath, "HEAD"))
		return headErr == nil
	}
	if info.Mode().IsRegular() {
		data, err := os.ReadFile(gitPath)
		if err != nil {
			return false
		}
		return strings.HasPrefix(strings.TrimSpace(string(data)), "gitdir:")
	}
	return false
}

// shouldSkipDoctorTree mirrors init's scan skip-list so doctor does not descend
// into VCS, dependency, or build trees while discovering adapters.
func shouldSkipDoctorTree(name string) bool {
	switch name {
	case ".git", ".devlane", ".direnv", "node_modules", "vendor", "dist", "build", "target", "tmp":
		return true
	default:
		return false
	}
}
