package portalloc

import (
	"errors"
	"fmt"
	"time"

	"github.com/auro/devlane/internal/config"
)

// ReassignService recomputes the port for the (lane.App, lane.RepoPath,
// service) row inside snap and updates the row in place. It uses the same
// sticky port-resolution rules Prepare applies — stable lanes take the
// adapter's stable fixture, dev lanes walk the configured pool — but with one
// twist scoped to reassign: dev lanes treat the current port as held so the
// allocator displaces the row onto a different bindable port. Stable lanes
// still reclaim their fixture (the current row is excluded from the held set
// for stable so the fixture can be re-taken when bindable).
//
// The mutation is scoped to the requested service: every other catalog row is
// left untouched. The row's identity fields (App, Service, RepoPath) are
// preserved; the metadata fields (Lane, Mode, Branch, LastPrepared) are
// refreshed from lane so reassign honours operator-supplied lane/mode context.
//
// ReassignService is intended to be called from inside a Mutate callback. It
// performs no I/O of its own beyond probing candidate ports during selection.
func ReassignService(snap *Snapshot, adapter *config.AdapterConfig, lane Lane, service string) error {
	if snap == nil {
		return errors.New("ReassignService: snapshot is required")
	}
	if adapter == nil {
		return errors.New("ReassignService: adapter is required")
	}

	portConfig, ok := findPortConfig(adapter, service)
	if !ok {
		return fmt.Errorf("adapter does not declare service %q", service)
	}

	rowIndex, ok := findRowIndex(snap.Allocations, lane.App, lane.RepoPath, service)
	if !ok {
		return fmt.Errorf("no catalog allocation found for app %q service %q at %s", lane.App, service, lane.RepoPath)
	}

	cfg, err := loadHostConfig()
	if err != nil {
		return err
	}

	currentRow := snap.Allocations[rowIndex]
	reserved := reservedPorts(cfg, adapter)
	held := buildHeldExcluding(snap.Allocations, rowIndex)
	if !lane.Stable {
		// Dev reassign is a displacement: the current port must not be
		// re-picked even when no other row holds it. Stable lanes intentionally
		// leave the row excluded so the fixture is reclaimable.
		held[currentRow.Port] = currentRow
	}
	claimed := map[int]Allocation{}

	port, err := choosePort(portConfig, lane, cfg, reserved, held, claimed)
	if err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	row := &snap.Allocations[rowIndex]
	row.Port = port
	row.Lane = lane.Name
	row.Mode = lane.Mode
	row.Branch = lane.Branch
	row.LastPrepared = now
	return nil
}

// FindAllocation returns the (app, repoPath, service) row from rows when one
// exists. The lookup uses sameRepoPath so callers may pass either the literal
// catalog repoPath or a symlink-equivalent path.
func FindAllocation(rows []Allocation, app, repoPath, service string) (Allocation, bool) {
	idx, ok := findRowIndex(rows, app, repoPath, service)
	if !ok {
		return Allocation{}, false
	}
	return rows[idx], true
}

func findRowIndex(rows []Allocation, app, repoPath, service string) (int, bool) {
	for i, row := range rows {
		if row.App != app || row.Service != service {
			continue
		}
		if !sameRepoPath(row.RepoPath, repoPath) {
			continue
		}
		return i, true
	}
	return -1, false
}

func findPortConfig(adapter *config.AdapterConfig, service string) (config.PortConfig, bool) {
	for _, port := range adapter.Ports {
		if port.Name == service {
			return port, true
		}
	}
	return config.PortConfig{}, false
}

func buildHeldExcluding(rows []Allocation, skipIndex int) map[int]Allocation {
	held := make(map[int]Allocation, len(rows))
	for i, row := range rows {
		if i == skipIndex {
			continue
		}
		held[row.Port] = row
	}
	return held
}
