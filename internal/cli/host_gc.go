package cli

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/auro/devlane/internal/drift"
	"github.com/auro/devlane/internal/portalloc"
)

// stdinIsInteractive reports whether the given stream is an interactive
// terminal. It uses term.IsTerminal (an actual termios query) rather than the
// os.ModeCharDevice bit, because non-terminal character devices such as
// /dev/null and /dev/zero also set that bit: an unattended `host gc < /dev/null`
// must hit the documented refusal path, not the prompt path, and must never
// block reading from a non-terminal device. It is a package variable so internal
// tests can force the prompt path, which is otherwise unreachable under the
// pipe-based CLI test harness.
var stdinIsInteractive = func(f *os.File) bool {
	if f == nil {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// runHostGc removes catalog rows that `host doctor` would classify as safely
// removable (missing-repoPath, missing-service, app-mismatch). Findings that
// require a human decision (duplicate-claim, bad-adapter) are surfaced but never
// removed. It reuses the exact same drift detector and adapter loader as doctor,
// so the two commands can never disagree about what is removable.
//
// Removal is gated: --dry-run prints and exits without mutating; otherwise the
// command requires either --yes or an interactive confirmation, and refuses
// (without mutating) when neither is available so an unattended run can never
// delete rows silently.
func runHostGc(args []string) int {
	fs := flag.NewFlagSet("host gc", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		appFilter string
		dryRun    bool
		assumeYes bool
	)
	fs.StringVar(&appFilter, "app", "", "Limit cleanup to allocations whose app matches")
	fs.BoolVar(&dryRun, "dry-run", false, "Print what would be removed without modifying the catalog")
	fs.BoolVar(&assumeYes, "yes", false, "Remove without an interactive confirmation prompt")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "host gc takes no positional arguments")
		return 2
	}

	rows, err := portalloc.List()
	if err != nil {
		return exitError(err)
	}

	// Detect over the full catalog so cross-app duplicate-claim is computed
	// correctly, then narrow to the requested app for display and removal.
	findings := drift.Detect(rows, loadRepoAdapters)
	if appFilter != "" {
		findings = filterFindingsByApp(findings, appFilter)
	}

	removable, surfaced := partitionFindings(findings)

	// Surfaced findings are never removed; print them so the operator knows
	// manual action is still required, but they do not gate gc's exit code.
	if len(surfaced) > 0 {
		fmt.Println("drift requiring a manual decision (not removed):")
		if err := writeFindingTable(os.Stdout, surfaced); err != nil {
			return exitError(err)
		}
	}

	if len(removable) == 0 {
		fmt.Println("no removable drift")
		return 0
	}

	fmt.Println("removable allocations:")
	if err := writeFindingTable(os.Stdout, removable); err != nil {
		return exitError(err)
	}

	if dryRun {
		fmt.Printf("dry-run: %d allocation(s) would be removed; catalog unchanged\n", len(removable))
		return 0
	}

	if !assumeYes {
		if !stdinIsInteractive(os.Stdin) {
			fmt.Fprintln(os.Stderr, "refusing to remove without confirmation: re-run with --yes or from an interactive terminal")
			return 1
		}
		if !confirmRemoval(os.Stdin, os.Stdout, len(removable)) {
			fmt.Fprintln(os.Stderr, "gc cancelled; catalog unchanged")
			return 1
		}
	}

	removed, err := removeAllocations(removable)
	if err != nil {
		return exitError(err)
	}
	fmt.Printf("removed %d allocation(s)\n", removed)
	return 0
}

// isRemovableCategory reports whether a drift category is one `host gc` treats
// as safe to delete. duplicate-claim and bad-adapter are deliberately excluded:
// each needs a human to decide which claimant or adapter is authoritative.
func isRemovableCategory(c drift.Category) bool {
	switch c {
	case drift.CategoryMissingRepoPath, drift.CategoryMissingService, drift.CategoryAppMismatch:
		return true
	default:
		return false
	}
}

// partitionFindings splits findings per finding, not per row. A single row may
// produce both a removable finding and a surfaced-only one — duplicate-claim is
// orthogonal (detected by port, independent of the adapter loader), so a row
// whose worktree/app/service is gone can also collide on a port. Such a row
// lands in BOTH lists: it is removed (its removable finding is dispositive) and
// also printed under the surfaced section. duplicate-claim and bad-adapter
// therefore never drive removal and never protect a row that independently
// qualifies as removable; only a row whose findings are entirely surfaced-only
// is preserved. Removing a dead claimant resolves the duplicate as a side effect
// by leaving the healthy claimant — which has no removable finding and so is
// never touched — the sole owner of the port.
func partitionFindings(findings []drift.Finding) (removable, surfaced []drift.Finding) {
	for _, f := range findings {
		if isRemovableCategory(f.Category) {
			removable = append(removable, f)
		} else {
			surfaced = append(surfaced, f)
		}
	}
	return removable, surfaced
}

func filterFindingsByApp(findings []drift.Finding, app string) []drift.Finding {
	var out []drift.Finding
	for _, f := range findings {
		if f.Allocation.App == app {
			out = append(out, f)
		}
	}
	return out
}

// removeAllocations deletes drifted rows inside a single atomic Mutate. It does
// not trust the unlocked pre-prompt scan to decide what to delete: drift depends
// on adapter state on disk, which can change between that scan (and any
// interactive prompt the operator sits at) and this locked write. Instead it
// re-runs drift detection against the freshly locked snapshot and removes a row
// only when BOTH hold:
//
//   - the row is STILL classified removable by the re-detect (so a row whose
//     adapter was repaired, whose worktree was restored, or that was refreshed by
//     a concurrent prepare is re-validated as healthy and left untouched rather
//     than deleted on a stale classification); and
//   - the row, matched by its entire value (every field, including port and
//     lastPrepared), was in the set shown to and confirmed by the operator (so a
//     row that drifted only after the preview is never removed without consent,
//     and a row refreshed to a different value no longer matches the confirmed
//     entry).
//
// The intersection is the safe direction for a destructive command: anything
// uncertain — changed, repaired, or unconfirmed — is preserved. The confirmed
// set is already scoped to --app, so the re-detect (which covers every app) is
// naturally narrowed back to the operator's scope by the intersection.
func removeAllocations(confirmedRemovable []drift.Finding) (int, error) {
	confirmed := make(map[portalloc.Allocation]struct{}, len(confirmedRemovable))
	for _, f := range confirmedRemovable {
		confirmed[f.Allocation] = struct{}{}
	}

	removed := 0
	err := mutateCatalog(func(snap *portalloc.Snapshot) error {
		// Re-detect under the lock so deletion reflects current adapter state, not
		// the pre-prompt classification. This is the same discovery host doctor
		// runs; it touches the filesystem but not the catalog, so holding the lock
		// across it cannot deadlock, and the prompt already happened before this.
		findings := drift.Detect(snap.Allocations, loadRepoAdapters)
		removableNow, _ := partitionFindings(findings)

		drop := make(map[portalloc.Allocation]struct{}, len(removableNow))
		for _, f := range removableNow {
			if _, ok := confirmed[f.Allocation]; ok {
				drop[f.Allocation] = struct{}{}
			}
		}

		kept := snap.Allocations[:0]
		for _, row := range snap.Allocations {
			if _, ok := drop[row]; ok {
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

// confirmRemoval prompts the operator before deleting rows, mirroring the
// confirm prefix used by `devlane init`. Only an explicit y/yes proceeds.
func confirmRemoval(in *os.File, out *os.File, count int) bool {
	fmt.Fprintf(out, "Remove %d allocation(s)? [y/N]: ", count)
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}
