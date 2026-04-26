package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/auro/devlane/internal/config"
	"github.com/auro/devlane/internal/manifest"
	"github.com/auro/devlane/internal/portalloc"
)

// mutateCatalog is a test seam matching the publishPrepareSession /
// closePrepareSession pattern in app.go. Internal tests can override it to
// inject failure modes without touching the on-disk catalog.
var mutateCatalog = portalloc.Mutate

// errReassignNoOp is the sentinel callback error that tells reassign to
// short-circuit Mutate's publish step when the current allocation is already
// on a bindable port. Returning this error from inside the Mutate callback
// keeps the on-disk catalog untouched while still letting the CLI exit 0.
var errReassignNoOp = errors.New("reassign: current port is bindable; no change")

func runReassign(args []string) int {
	fs := flag.NewFlagSet("reassign", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		configPath string
		cwd        string
		force      bool
		laneName   string
	)
	fs.StringVar(&configPath, "config", "devlane.yaml", "Path to devlane.yaml")
	fs.StringVar(&cwd, "cwd", ".", "Working directory used for discovery")
	fs.BoolVar(&force, "force", false, "Always allocate a new port, even when the current allocation is bindable")
	fs.StringVar(&laneName, "lane", "", "Reassign a different lane in the current app's catalog rather than the current checkout's lane")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(os.Stderr, "usage: devlane reassign [flags] <service>")
		return 2
	}
	service := rest[0]

	flags := &commonFlags{config: configPath, cwd: cwd}
	_, _, adapter, laneManifest, err := load(flags)
	if err != nil {
		return exitError(err)
	}

	if _, ok := lookupPortConfig(adapter, service); !ok {
		return exitError(fmt.Errorf("adapter does not declare service %q", service))
	}

	target, err := resolveReassignTarget(adapter, laneManifest, laneName)
	if err != nil {
		return exitError(err)
	}

	var resultPort int
	err = mutateCatalog(func(snap *portalloc.Snapshot) error {
		row, ok := portalloc.FindAllocation(snap.Allocations, target.lane.App, target.lane.RepoPath, service)
		if !ok {
			return formatMissingAllocation(target, service)
		}

		if !force {
			if probeErr := portalloc.Probe(row.Port); probeErr == nil {
				resultPort = row.Port
				return errReassignNoOp
			}
		}

		if reassignErr := portalloc.ReassignService(snap, adapter, target.lane, service); reassignErr != nil {
			return reassignErr
		}

		updated, _ := portalloc.FindAllocation(snap.Allocations, target.lane.App, target.lane.RepoPath, service)
		resultPort = updated.Port
		return nil
	})
	if errors.Is(err, errReassignNoOp) {
		fmt.Printf("service %q is already on bindable port %d; no change\n", service, resultPort)
		return 0
	}
	if err != nil {
		return exitError(err)
	}

	fmt.Printf("reassigned service %q on lane %q (%s) to port %d\n",
		service, target.lane.Name, target.repoPathForDisplay, resultPort)
	return 0
}

type reassignTarget struct {
	lane                portalloc.Lane
	repoPathForDisplay  string
	resolvedFromCatalog bool
}

func resolveReassignTarget(adapter *config.AdapterConfig, laneManifest manifest.Manifest, requestedLane string) (reassignTarget, error) {
	if strings.TrimSpace(requestedLane) == "" {
		lane := portallocLane(adapter, laneManifest)
		return reassignTarget{
			lane:               lane,
			repoPathForDisplay: lane.RepoPath,
		}, nil
	}

	currentRepoPath := laneManifest.Lane.RepoRoot
	if strings.TrimSpace(currentRepoPath) == "" {
		return reassignTarget{}, errors.New("--lane requires a resolvable repo context but the current checkout has no repoRoot")
	}

	rows, err := portalloc.List()
	if err != nil {
		return reassignTarget{}, err
	}
	match, err := portalloc.ResolveLane(rows, adapter.App, currentRepoPath, requestedLane)
	if err != nil {
		return reassignTarget{}, err
	}

	switch match.Kind {
	case portalloc.LaneMatchNone:
		return reassignTarget{}, fmt.Errorf("no allocation found for lane %q in app %q", requestedLane, adapter.App)
	case portalloc.LaneMatchAmbiguous:
		return reassignTarget{}, formatLaneAmbiguity(adapter.App, requestedLane, match.Allocations)
	case portalloc.LaneMatchSingle:
		representative := match.Allocations[0]
		return reassignTarget{
			lane: portalloc.Lane{
				App:      adapter.App,
				RepoPath: match.RepoPath,
				Name:     representative.Lane,
				Mode:     representative.Mode,
				Branch:   representative.Branch,
				Stable:   representative.Mode == "stable",
			},
			repoPathForDisplay:  match.RepoPath,
			resolvedFromCatalog: true,
		}, nil
	}

	return reassignTarget{}, fmt.Errorf("unexpected lane match kind %v", match.Kind)
}

func formatLaneAmbiguity(app, lane string, rows []portalloc.Allocation) error {
	repoPaths := make([]string, 0)
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if _, ok := seen[row.RepoPath]; ok {
			continue
		}
		seen[row.RepoPath] = struct{}{}
		repoPaths = append(repoPaths, row.RepoPath)
	}
	sort.Strings(repoPaths)

	var b strings.Builder
	fmt.Fprintf(&b, "lane %q is ambiguous in app %q — multiple checkouts share the name; pass --cwd to target one explicitly:", lane, app)
	for _, path := range repoPaths {
		fmt.Fprintf(&b, "\n  - %s", path)
	}
	return errors.New(b.String())
}

func formatMissingAllocation(target reassignTarget, service string) error {
	if target.resolvedFromCatalog {
		return fmt.Errorf("lane %q has no allocation for service %q at %s", target.lane.Name, service, target.lane.RepoPath)
	}
	return fmt.Errorf("no catalog allocation for service %q at %s — run `devlane prepare` first",
		service, target.lane.RepoPath)
}

func lookupPortConfig(adapter *config.AdapterConfig, service string) (config.PortConfig, bool) {
	for _, port := range adapter.Ports {
		if port.Name == service {
			return port, true
		}
	}
	return config.PortConfig{}, false
}
