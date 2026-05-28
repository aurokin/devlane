// Package drift detects inconsistencies between the host port catalog and the
// adapters that the catalog rows point at. It is pure logic: it performs no I/O
// of its own and reaches the filesystem only through an injected AdapterLoader.
// This keeps the module trivially testable and lets it be reused by both the
// read-only `host doctor` audit and the mutating `host gc` cleanup.
//
// A catalog row's repoPath is the Git worktree root, not the adapter directory:
// a monorepo worktree can host several subtree adapters (e.g. apps/web and
// apps/api) that share one repoPath while declaring different apps. The loader
// therefore discovers every adapter under a repoPath, and Detect matches each
// row to the adapter that declares the row's app.
package drift

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/auro/devlane/internal/portalloc"
)

// RepoAdapters is the aggregate view of the adapters discovered under a catalog
// row's repoPath. It is produced by the injected loader, the package's only I/O
// boundary, so Detect itself stays pure.
type RepoAdapters struct {
	// Services maps each app declared by an adapter under the repoPath to the
	// set of service names it declares, aggregated across every adapter found.
	Services map[string]map[string]struct{}
	// LoadErr is non-nil when at least one discovered adapter exists but failed
	// to parse or validate. A row whose app is not found among Services is then
	// classified bad-adapter (surfaced, never auto-removed) rather than
	// app-mismatch, so a transient parse error can never make a healthy row look
	// safely removable to `host gc`.
	LoadErr error
}

// AdapterLoader discovers the adapters under a catalog row's repoPath. It is the
// only filesystem access in this package. It returns an error only when the
// repoPath itself cannot be scanned (e.g. it no longer exists); a returned
// error that wraps os.ErrNotExist classifies the row as missing-repoPath, any
// other returned error as bad-adapter. Individual adapter parse failures are
// reported via RepoAdapters.LoadErr, not the returned error.
type AdapterLoader func(repoPath string) (RepoAdapters, error)

// Category labels a drift finding. The first three are the categories `host gc`
// treats as safe to remove; duplicate-claim and bad-adapter are surfaced for
// the operator but require a human decision, so a malformed adapter surfaces as
// a classified finding rather than a panic or a deletion.
type Category string

const (
	// CategoryMissingRepoPath marks a row whose worktree root no longer exists.
	CategoryMissingRepoPath Category = "missing-repoPath"
	// CategoryMissingService marks a row whose service is no longer declared by
	// the adapter that declares the row's app.
	CategoryMissingService Category = "missing-service"
	// CategoryAppMismatch marks a row claiming an app that no adapter under its
	// repoPath declares any more.
	CategoryAppMismatch Category = "app-mismatch"
	// CategoryDuplicateClaim marks rows that claim the same port as another row.
	CategoryDuplicateClaim Category = "duplicate-claim"
	// CategoryBadAdapter marks a row whose app is unaccounted for while some
	// adapter under its repoPath failed to load (malformed YAML or schema error).
	CategoryBadAdapter Category = "bad-adapter"
)

// Finding is a single drift detection result. It carries the full Allocation so
// callers can render identifying context (app, lane, service, port, repoPath)
// and so host gc can filter by Category without parsing strings. Detail is a
// human-readable explanation of the specific finding.
type Finding struct {
	Category   Category
	Allocation portalloc.Allocation
	Detail     string
}

// Detect audits the catalog snapshot and returns categorized findings. It is
// pure: every filesystem touch happens inside the injected load function, which
// is invoked at most once per distinct repoPath. Duplicate-claim detection runs
// independently of adapter loading, so a row can produce both a duplicate-claim
// finding and an adapter-derived finding. Findings are returned in a
// deterministic order so callers and tests observe stable output.
func Detect(rows []portalloc.Allocation, load AdapterLoader) []Finding {
	var findings []Finding

	// Duplicate-claim is orthogonal to adapter loading: two rows claiming the
	// same host port conflict regardless of which app or repo they belong to.
	findings = append(findings, detectDuplicateClaims(rows)...)

	// Group rows by canonical repoPath so the loader is consulted once per
	// checkout, folding symlinked paths together the same way portalloc does.
	for _, group := range groupByRepoPath(rows) {
		adapters, err := load(group.repoPath)
		findings = append(findings, classifyGroup(group.rows, adapters, err)...)
	}

	sortFindings(findings)
	return findings
}

func detectDuplicateClaims(rows []portalloc.Allocation) []Finding {
	byPort := make(map[int][]portalloc.Allocation, len(rows))
	order := make([]int, 0, len(rows))
	for _, row := range rows {
		if _, seen := byPort[row.Port]; !seen {
			order = append(order, row.Port)
		}
		byPort[row.Port] = append(byPort[row.Port], row)
	}

	var findings []Finding
	for _, port := range order {
		claimants := byPort[port]
		if len(claimants) < 2 {
			continue
		}
		for _, row := range claimants {
			findings = append(findings, Finding{
				Category:   CategoryDuplicateClaim,
				Allocation: row,
				Detail:     portClaimDetail(port, len(claimants)),
			})
		}
	}
	return findings
}

type repoGroup struct {
	repoPath string // representative (raw) repoPath to hand the loader
	rows     []portalloc.Allocation
}

func groupByRepoPath(rows []portalloc.Allocation) []repoGroup {
	groups := make(map[string]*repoGroup, len(rows))
	var order []string
	for _, row := range rows {
		key := canonicalRepoPath(row.RepoPath)
		g, ok := groups[key]
		if !ok {
			g = &repoGroup{repoPath: row.RepoPath}
			groups[key] = g
			order = append(order, key)
		}
		g.rows = append(g.rows, row)
	}

	out := make([]repoGroup, 0, len(order))
	for _, key := range order {
		out = append(out, *groups[key])
	}
	return out
}

// canonicalRepoPath folds symlinked paths to a stable key, mirroring
// portalloc.sameRepoPath (which is unexported). When the path cannot be resolved
// (e.g. it no longer exists), it falls back to the lexically cleaned path so
// identical missing paths still group together.
func canonicalRepoPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(path)
}

func classifyGroup(rows []portalloc.Allocation, adapters RepoAdapters, err error) []Finding {
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return findingsForRows(rows, CategoryMissingRepoPath, err.Error())
		}
		return findingsForRows(rows, CategoryBadAdapter, err.Error())
	}

	var findings []Finding
	for _, row := range rows {
		services, declared := adapters.Services[row.App]
		if declared {
			if _, ok := services[row.Service]; ok {
				continue // app and service both confirmed: healthy
			}
		}

		switch {
		case adapters.LoadErr != nil:
			// The row's app or service is unaccounted for while some adapter
			// under this repo failed to load — the missing piece may live in
			// that adapter, so surface the non-removable bad-adapter rather than
			// a removable missing-service / app-mismatch. Uncertainty must never
			// look safe to delete.
			findings = append(findings, Finding{
				Category:   CategoryBadAdapter,
				Allocation: row,
				Detail:     adapters.LoadErr.Error(),
			})
		case declared:
			findings = append(findings, Finding{
				Category:   CategoryMissingService,
				Allocation: row,
				Detail:     missingServiceDetail(row),
			})
		default:
			findings = append(findings, Finding{
				Category:   CategoryAppMismatch,
				Allocation: row,
				Detail:     appMismatchDetail(row),
			})
		}
	}
	return findings
}

func findingsForRows(rows []portalloc.Allocation, category Category, detail string) []Finding {
	findings := make([]Finding, 0, len(rows))
	for _, row := range rows {
		findings = append(findings, Finding{
			Category:   category,
			Allocation: row,
			Detail:     detail,
		})
	}
	return findings
}

func portClaimDetail(port, claimants int) string {
	return fmt.Sprintf("port %d is claimed by %d allocations", port, claimants)
}

func appMismatchDetail(row portalloc.Allocation) string {
	return fmt.Sprintf("no adapter under %s declares app %q", row.RepoPath, row.App)
}

func missingServiceDetail(row portalloc.Allocation) string {
	return fmt.Sprintf("service %q is no longer declared by adapter %q", row.Service, row.App)
}

func sortFindings(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.Allocation.RepoPath != b.Allocation.RepoPath {
			return a.Allocation.RepoPath < b.Allocation.RepoPath
		}
		if a.Allocation.Service != b.Allocation.Service {
			return a.Allocation.Service < b.Allocation.Service
		}
		if a.Allocation.Port != b.Allocation.Port {
			return a.Allocation.Port < b.Allocation.Port
		}
		return a.Category < b.Category
	})
}
