package drift_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/auro/devlane/internal/drift"
	"github.com/auro/devlane/internal/portalloc"
)

type loaderEntry struct {
	res drift.RepoAdapters
	err error
}

// newLoader returns a fake AdapterLoader backed by an explicit per-repoPath
// result map so tests never touch the filesystem. Unmapped paths default to a
// wrapped os.ErrNotExist (a missing worktree root). When calls is non-nil it
// counts every invocation, which the symlink-folding test asserts on.
func newLoader(entries map[string]loaderEntry, calls *int) drift.AdapterLoader {
	return func(repoPath string) (drift.RepoAdapters, error) {
		if calls != nil {
			*calls++
		}
		if e, ok := entries[repoPath]; ok {
			return e.res, e.err
		}
		return drift.RepoAdapters{}, fmt.Errorf("read adapter: %w", os.ErrNotExist)
	}
}

// repoAdapters builds the aggregate app -> services view a discovery loader
// would return for a worktree root. loadErr models a malformed adapter found
// during discovery.
func repoAdapters(loadErr error, apps map[string][]string) drift.RepoAdapters {
	services := make(map[string]map[string]struct{}, len(apps))
	for app, svcs := range apps {
		set := make(map[string]struct{}, len(svcs))
		for _, s := range svcs {
			set[s] = struct{}{}
		}
		services[app] = set
	}
	return drift.RepoAdapters{Services: services, LoadErr: loadErr}
}

func alloc(app, service, repoPath string, port int) portalloc.Allocation {
	return portalloc.Allocation{
		App:          app,
		Lane:         "feature",
		Mode:         "dev",
		Branch:       "feature",
		Service:      service,
		Port:         port,
		RepoPath:     repoPath,
		LastPrepared: "2026-04-25T00:00:00Z",
	}
}

func countByCategory(findings []drift.Finding) map[drift.Category]int {
	counts := make(map[drift.Category]int)
	for _, f := range findings {
		counts[f.Category]++
	}
	return counts
}

func hasFinding(findings []drift.Finding, category drift.Category, repoPath, service string) bool {
	for _, f := range findings {
		if f.Category == category && f.Allocation.RepoPath == repoPath && f.Allocation.Service == service {
			return true
		}
	}
	return false
}

func TestDetectCleanCatalogReturnsNoFindings(t *testing.T) {
	rows := []portalloc.Allocation{
		alloc("agentchat", "web", "/repo/agentchat", 3100),
		alloc("agentchat", "api", "/repo/agentchat", 3101),
		alloc("billing", "web", "/repo/billing", 3200),
	}
	load := newLoader(map[string]loaderEntry{
		"/repo/agentchat": {res: repoAdapters(nil, map[string][]string{"agentchat": {"web", "api"}})},
		"/repo/billing":   {res: repoAdapters(nil, map[string][]string{"billing": {"web"}})},
	}, nil)

	findings := drift.Detect(rows, load)
	if len(findings) != 0 {
		t.Fatalf("expected no findings on a clean catalog, got %d: %+v", len(findings), findings)
	}
}

func TestDetectMissingRepoPath(t *testing.T) {
	rows := []portalloc.Allocation{
		alloc("agentchat", "web", "/repo/gone", 3100),
		alloc("agentchat", "api", "/repo/gone", 3101),
	}
	// Wrap os.ErrNotExist to prove classification relies on errors.Is unwrapping,
	// not a bare sentinel comparison.
	load := newLoader(map[string]loaderEntry{
		"/repo/gone": {err: fmt.Errorf("read adapter: %w", os.ErrNotExist)},
	}, nil)

	findings := drift.Detect(rows, load)
	if got := countByCategory(findings); got[drift.CategoryMissingRepoPath] != 2 || len(findings) != 2 {
		t.Fatalf("expected 2 missing-repoPath findings, got %+v", findings)
	}
	if !hasFinding(findings, drift.CategoryMissingRepoPath, "/repo/gone", "web") ||
		!hasFinding(findings, drift.CategoryMissingRepoPath, "/repo/gone", "api") {
		t.Fatalf("missing expected per-row findings: %+v", findings)
	}
}

func TestDetectMissingService(t *testing.T) {
	rows := []portalloc.Allocation{
		alloc("agentchat", "web", "/repo/agentchat", 3100),
		alloc("agentchat", "worker", "/repo/agentchat", 3101),
	}
	// Adapter no longer declares "worker".
	load := newLoader(map[string]loaderEntry{
		"/repo/agentchat": {res: repoAdapters(nil, map[string][]string{"agentchat": {"web"}})},
	}, nil)

	findings := drift.Detect(rows, load)
	if len(findings) != 1 || findings[0].Category != drift.CategoryMissingService {
		t.Fatalf("expected 1 missing-service finding, got %+v", findings)
	}
	if findings[0].Allocation.Service != "worker" {
		t.Fatalf("expected finding for service worker, got %q", findings[0].Allocation.Service)
	}
}

func TestDetectAppMismatchWhenNoAdapterDeclaresApp(t *testing.T) {
	// The row claims app "oldname", but the only adapter under the worktree now
	// declares app "newname", so no adapter claims the row's app.
	rows := []portalloc.Allocation{
		alloc("oldname", "web", "/repo/app", 3100),
	}
	load := newLoader(map[string]loaderEntry{
		"/repo/app": {res: repoAdapters(nil, map[string][]string{"newname": {"api"}})},
	}, nil)

	findings := drift.Detect(rows, load)
	if len(findings) != 1 || findings[0].Category != drift.CategoryAppMismatch {
		t.Fatalf("expected exactly 1 app-mismatch finding, got %+v", findings)
	}
}

func TestDetectDuplicateClaim(t *testing.T) {
	// Two otherwise-healthy rows in different repos claim the same port.
	rows := []portalloc.Allocation{
		alloc("agentchat", "web", "/repo/agentchat", 3100),
		alloc("billing", "web", "/repo/billing", 3100),
	}
	load := newLoader(map[string]loaderEntry{
		"/repo/agentchat": {res: repoAdapters(nil, map[string][]string{"agentchat": {"web"}})},
		"/repo/billing":   {res: repoAdapters(nil, map[string][]string{"billing": {"web"}})},
	}, nil)

	findings := drift.Detect(rows, load)
	if got := countByCategory(findings); got[drift.CategoryDuplicateClaim] != 2 || len(findings) != 2 {
		t.Fatalf("expected 2 duplicate-claim findings, got %+v", findings)
	}
}

func TestDetectBadAdapterDoesNotPanic(t *testing.T) {
	rows := []portalloc.Allocation{
		alloc("agentchat", "web", "/repo/agentchat", 3100),
	}
	// Discovery found an adapter but it failed to load, and no parsed adapter
	// declares the row's app.
	load := newLoader(map[string]loaderEntry{
		"/repo/agentchat": {res: repoAdapters(errors.New("decode adapter: bad yaml"), nil)},
	}, nil)

	findings := drift.Detect(rows, load)
	if len(findings) != 1 || findings[0].Category != drift.CategoryBadAdapter {
		t.Fatalf("expected 1 bad-adapter finding, got %+v", findings)
	}
}

func TestDetectScanErrorClassifiedAsBadAdapter(t *testing.T) {
	// A non-NotExist error returned by the loader (e.g. the worktree root could
	// not be scanned) classifies as bad-adapter, not missing-repoPath.
	rows := []portalloc.Allocation{
		alloc("agentchat", "web", "/repo/locked", 3100),
	}
	load := newLoader(map[string]loaderEntry{
		"/repo/locked": {err: errors.New("permission denied")},
	}, nil)

	findings := drift.Detect(rows, load)
	if len(findings) != 1 || findings[0].Category != drift.CategoryBadAdapter {
		t.Fatalf("expected 1 bad-adapter finding for a scan error, got %+v", findings)
	}
}

func TestDetectMonorepoMultipleAppsAreHealthy(t *testing.T) {
	// One worktree root hosts two subtree adapters declaring different apps;
	// both rows must be healthy. This is the subtree-adapter regression case.
	rows := []portalloc.Allocation{
		alloc("webui", "web", "/mono", 3100),
		alloc("core", "api", "/mono", 3101),
	}
	load := newLoader(map[string]loaderEntry{
		"/mono": {res: repoAdapters(nil, map[string][]string{"webui": {"web"}, "core": {"api"}})},
	}, nil)

	findings := drift.Detect(rows, load)
	if len(findings) != 0 {
		t.Fatalf("expected no findings for healthy monorepo subtree adapters, got %+v", findings)
	}
}

func TestDetectMonorepoRemovedAppIsAppMismatchSiblingHealthy(t *testing.T) {
	// The webui subtree adapter was removed; its sibling core remains. Only the
	// webui row drifts (app-mismatch); core stays healthy.
	rows := []portalloc.Allocation{
		alloc("webui", "web", "/mono", 3100),
		alloc("core", "api", "/mono", 3101),
	}
	load := newLoader(map[string]loaderEntry{
		"/mono": {res: repoAdapters(nil, map[string][]string{"core": {"api"}})},
	}, nil)

	findings := drift.Detect(rows, load)
	if len(findings) != 1 || findings[0].Category != drift.CategoryAppMismatch || findings[0].Allocation.App != "webui" {
		t.Fatalf("expected exactly 1 app-mismatch for webui, got %+v", findings)
	}
}

func TestDetectMalformedSiblingDoesNotPoisonHealthyApp(t *testing.T) {
	// A malformed sibling adapter sets LoadErr, but the healthy app stays
	// healthy; only the row whose app is unaccounted for is conservatively
	// reported as bad-adapter (never app-mismatch, which gc would remove).
	rows := []portalloc.Allocation{
		alloc("core", "api", "/mono", 3100),
		alloc("ghost", "web", "/mono", 3101),
	}
	load := newLoader(map[string]loaderEntry{
		"/mono": {res: repoAdapters(errors.New("decode adapter: bad yaml"), map[string][]string{"core": {"api"}})},
	}, nil)

	findings := drift.Detect(rows, load)
	if len(findings) != 1 {
		t.Fatalf("expected exactly 1 finding, got %+v", findings)
	}
	if findings[0].Category != drift.CategoryBadAdapter || findings[0].Allocation.App != "ghost" {
		t.Fatalf("expected bad-adapter for ghost only, got %+v", findings)
	}
}

func TestDetectMissingServiceUnderUncertaintyIsBadAdapter(t *testing.T) {
	// app "shop" is declared (only "web"), but a sibling adapter failed to load.
	// The row's "api" service might live in that broken sibling, so the row must
	// be surfaced as bad-adapter, never the removable missing-service.
	rows := []portalloc.Allocation{
		alloc("shop", "api", "/mono", 3100),
	}
	load := newLoader(map[string]loaderEntry{
		"/mono": {res: repoAdapters(errors.New("decode adapter: bad yaml"), map[string][]string{"shop": {"web"}})},
	}, nil)

	findings := drift.Detect(rows, load)
	if len(findings) != 1 || findings[0].Category != drift.CategoryBadAdapter {
		t.Fatalf("expected 1 bad-adapter finding under load uncertainty, got %+v", findings)
	}
}

func TestDetectDuplicateClaimIsIndependentOfLoaderOutcome(t *testing.T) {
	// Two rows in the same missing repo share a port: each must produce BOTH a
	// duplicate-claim and a missing-repoPath finding.
	rows := []portalloc.Allocation{
		alloc("agentchat", "web", "/repo/gone", 3100),
		alloc("agentchat", "api", "/repo/gone", 3100),
	}
	load := newLoader(map[string]loaderEntry{
		"/repo/gone": {err: fmt.Errorf("read adapter: %w", os.ErrNotExist)},
	}, nil)

	findings := drift.Detect(rows, load)
	counts := countByCategory(findings)
	if counts[drift.CategoryDuplicateClaim] != 2 || counts[drift.CategoryMissingRepoPath] != 2 {
		t.Fatalf("expected 2 duplicate-claim + 2 missing-repoPath findings, got %+v", findings)
	}
}

func TestDetectCombinationCoversAllCategoriesDeterministically(t *testing.T) {
	rows := []portalloc.Allocation{
		alloc("agentchat", "web", "/repo/agentchat", 3100),    // clean
		alloc("agentchat", "worker", "/repo/agentchat", 3101), // missing-service
		alloc("oldname", "web", "/repo/renamed", 3200),        // app-mismatch
		alloc("billing", "web", "/repo/gone", 3300),           // missing-repoPath
		alloc("payments", "web", "/repo/broken", 3400),        // bad-adapter
		alloc("dup-a", "web", "/repo/dup-a", 3500),            // duplicate-claim (port 3500)
		alloc("dup-b", "web", "/repo/dup-b", 3500),            // duplicate-claim (port 3500)
	}
	load := newLoader(map[string]loaderEntry{
		"/repo/agentchat": {res: repoAdapters(nil, map[string][]string{"agentchat": {"web"}})},
		"/repo/renamed":   {res: repoAdapters(nil, map[string][]string{"newname": {"web"}})},
		"/repo/gone":      {err: fmt.Errorf("read adapter: %w", os.ErrNotExist)},
		"/repo/broken":    {res: repoAdapters(errors.New("decode adapter: bad yaml"), nil)},
		"/repo/dup-a":     {res: repoAdapters(nil, map[string][]string{"dup-a": {"web"}})},
		"/repo/dup-b":     {res: repoAdapters(nil, map[string][]string{"dup-b": {"web"}})},
	}, nil)

	findings := drift.Detect(rows, load)

	counts := countByCategory(findings)
	want := map[drift.Category]int{
		drift.CategoryMissingService:  1,
		drift.CategoryAppMismatch:     1,
		drift.CategoryMissingRepoPath: 1,
		drift.CategoryBadAdapter:      1,
		drift.CategoryDuplicateClaim:  2,
	}
	for category, n := range want {
		if counts[category] != n {
			t.Fatalf("category %q: want %d findings, got %d\nall: %+v", category, n, counts[category], findings)
		}
	}
	if len(findings) != 6 {
		t.Fatalf("expected 6 total findings, got %d: %+v", len(findings), findings)
	}

	// Deterministic order: non-decreasing by (RepoPath, Service, Port, Category).
	if !sort.SliceIsSorted(findings, func(i, j int) bool {
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
	}) {
		t.Fatalf("findings are not in deterministic order: %+v", findings)
	}

	// Determinism across calls: a second run yields an identical slice.
	again := drift.Detect(rows, load)
	if len(again) != len(findings) {
		t.Fatalf("non-deterministic length: %d vs %d", len(again), len(findings))
	}
	for i := range findings {
		if again[i] != findings[i] {
			t.Fatalf("non-deterministic ordering at %d: %+v vs %+v", i, again[i], findings[i])
		}
	}
}

func TestDetectFoldsSymlinkedRepoPathsIntoOneLoaderCall(t *testing.T) {
	realDir := t.TempDir()
	linkDir := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	// Two rows for the same checkout reached via the real path and a symlink.
	rows := []portalloc.Allocation{
		alloc("agentchat", "web", realDir, 3100),
		alloc("agentchat", "api", linkDir, 3101),
	}

	var calls int
	// The loader is keyed by the group's representative (raw) repoPath, which is
	// the first row's path (realDir). The adapter declares both services so the
	// folded group is clean.
	load := newLoader(map[string]loaderEntry{
		realDir: {res: repoAdapters(nil, map[string][]string{"agentchat": {"web", "api"}})},
	}, &calls)

	findings := drift.Detect(rows, load)
	if calls != 1 {
		t.Fatalf("expected loader to be called once for the folded group, got %d", calls)
	}
	if len(findings) != 0 {
		t.Fatalf("expected no findings for a clean folded group, got %+v", findings)
	}
}
