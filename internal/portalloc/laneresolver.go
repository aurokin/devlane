package portalloc

import (
	"errors"
	"sort"
	"strings"
)

// LaneMatchKind classifies the outcome of ResolveLane.
type LaneMatchKind int

const (
	// LaneMatchNone means no catalog row matched the (app, lane) tuple.
	LaneMatchNone LaneMatchKind = iota
	// LaneMatchSingle means a single checkout matched. Allocations holds every
	// row for that checkout (one per declared service); RepoPath is the
	// checkout's repoPath as recorded in the catalog.
	LaneMatchSingle
	// LaneMatchAmbiguous means the lane name maps to more than one checkout in
	// the same app. Allocations enumerates every colliding row so callers can
	// surface a deterministic error to the operator.
	LaneMatchAmbiguous
)

// LaneMatch is the outcome of ResolveLane. RepoPath is populated for
// LaneMatchSingle and is left empty for LaneMatchNone / LaneMatchAmbiguous.
// Allocations is non-nil for the matched cases and lists every catalog row
// that participated in the match.
type LaneMatch struct {
	Kind        LaneMatchKind
	RepoPath    string
	Allocations []Allocation
}

// ResolveLane locates a target checkout for an operator command that selects
// by lane name in the current app's context. It scopes resolution to the
// caller's repo context — app and repoPath are mandatory and the resolver
// never crosses app boundaries — and returns one of three outcomes:
//
//   - LaneMatchSingle when exactly one checkout in the app uses the lane name,
//     OR when multiple matches share the same canonical repoPath (worktrees
//     symlinked to the same on-disk path), OR when one of the matches is the
//     caller's own repoPath (current-checkout-wins tiebreak so operators in a
//     worktree can target their local lane unambiguously).
//   - LaneMatchAmbiguous when distinct checkouts of the same app share the
//     lane name and none of them is the caller's repoPath. Allocations holds
//     every colliding row so callers can format an enumerated error.
//   - LaneMatchNone when no row in the catalog matches the (app, lane) tuple.
//
// repoPath comparisons evaluate symlinks (via sameRepoPath) so worktrees
// resolving to the same canonical path collapse into a single match.
func ResolveLane(catalog []Allocation, app, repoPath, lane string) (LaneMatch, error) {
	if strings.TrimSpace(app) == "" {
		return LaneMatch{}, errors.New("ResolveLane: app is required")
	}
	if strings.TrimSpace(repoPath) == "" {
		return LaneMatch{}, errors.New("ResolveLane: repoPath is required")
	}
	if strings.TrimSpace(lane) == "" {
		return LaneMatch{}, errors.New("ResolveLane: lane name is required")
	}

	groups := groupMatchingRowsByRepoPath(catalog, app, lane)
	switch len(groups) {
	case 0:
		return LaneMatch{Kind: LaneMatchNone}, nil
	case 1:
		group := groups[0]
		return LaneMatch{
			Kind:        LaneMatchSingle,
			RepoPath:    group.repoPath,
			Allocations: cloneAllocations(group.rows),
		}, nil
	}

	if winner, ok := pickCurrentCheckoutGroup(groups, repoPath); ok {
		return LaneMatch{
			Kind:        LaneMatchSingle,
			RepoPath:    winner.repoPath,
			Allocations: cloneAllocations(winner.rows),
		}, nil
	}

	return LaneMatch{
		Kind:        LaneMatchAmbiguous,
		Allocations: flattenGroups(groups),
	}, nil
}

type laneRowGroup struct {
	repoPath string
	rows     []Allocation
}

func groupMatchingRowsByRepoPath(catalog []Allocation, app, lane string) []laneRowGroup {
	groups := make([]laneRowGroup, 0)
	for _, row := range catalog {
		if row.App != app || row.Lane != lane {
			continue
		}
		idx := -1
		for i := range groups {
			if sameRepoPath(groups[i].repoPath, row.RepoPath) {
				idx = i
				break
			}
		}
		if idx == -1 {
			groups = append(groups, laneRowGroup{repoPath: row.RepoPath, rows: []Allocation{row}})
			continue
		}
		groups[idx].rows = append(groups[idx].rows, row)
	}
	for i := range groups {
		sortRowsByService(groups[i].rows)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].repoPath < groups[j].repoPath
	})
	return groups
}

func pickCurrentCheckoutGroup(groups []laneRowGroup, repoPath string) (laneRowGroup, bool) {
	for _, group := range groups {
		if sameRepoPath(group.repoPath, repoPath) {
			return group, true
		}
	}
	return laneRowGroup{}, false
}

func flattenGroups(groups []laneRowGroup) []Allocation {
	total := 0
	for _, group := range groups {
		total += len(group.rows)
	}
	out := make([]Allocation, 0, total)
	for _, group := range groups {
		out = append(out, group.rows...)
	}
	return out
}

func cloneAllocations(rows []Allocation) []Allocation {
	out := make([]Allocation, len(rows))
	copy(out, rows)
	return out
}

func sortRowsByService(rows []Allocation) {
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Service < rows[j].Service
	})
}
