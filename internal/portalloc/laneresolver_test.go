package portalloc_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/auro/devlane/internal/portalloc"
)

func TestResolveLaneRequiresAppRepoPathAndLane(t *testing.T) {
	cases := []struct {
		name     string
		app      string
		repoPath string
		lane     string
		wantErr  string
	}{
		{name: "missing app", app: "", repoPath: "/repo", lane: "feature-x", wantErr: "app is required"},
		{name: "missing repoPath", app: "alpha", repoPath: "", lane: "feature-x", wantErr: "repoPath is required"},
		{name: "missing lane", app: "alpha", repoPath: "/repo", lane: "", wantErr: "lane name is required"},
		{name: "whitespace-only app", app: "   ", repoPath: "/repo", lane: "feature-x", wantErr: "app is required"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := portalloc.ResolveLane(nil, tc.app, tc.repoPath, tc.lane)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestResolveLaneSingleMatchInOneCheckout(t *testing.T) {
	rows := []portalloc.Allocation{
		{App: "alpha", Lane: "feature-x", Mode: "dev", Service: "web", Port: 3001, RepoPath: "/repo/alpha-feature-x"},
		{App: "alpha", Lane: "feature-x", Mode: "dev", Service: "api", Port: 3002, RepoPath: "/repo/alpha-feature-x"},
		{App: "alpha", Lane: "main", Mode: "stable", Service: "web", Port: 3000, RepoPath: "/repo/alpha"},
		{App: "beta", Lane: "feature-x", Mode: "dev", Service: "web", Port: 4001, RepoPath: "/repo/beta-feature-x"},
	}

	got, err := portalloc.ResolveLane(rows, "alpha", "/repo/alpha", "feature-x")
	if err != nil {
		t.Fatalf("ResolveLane: %v", err)
	}
	if got.Kind != portalloc.LaneMatchSingle {
		t.Fatalf("expected LaneMatchSingle, got %v", got.Kind)
	}
	if got.RepoPath != "/repo/alpha-feature-x" {
		t.Fatalf("expected matched repoPath, got %q", got.RepoPath)
	}
	if len(got.Allocations) != 2 {
		t.Fatalf("expected both service rows in match, got %d (%#v)", len(got.Allocations), got.Allocations)
	}
	if got.Allocations[0].Service != "api" || got.Allocations[1].Service != "web" {
		t.Fatalf("expected match rows sorted by service, got %#v", got.Allocations)
	}
}

func TestResolveLaneNotFound(t *testing.T) {
	rows := []portalloc.Allocation{
		{App: "alpha", Lane: "main", Mode: "stable", Service: "web", Port: 3000, RepoPath: "/repo/alpha"},
	}

	got, err := portalloc.ResolveLane(rows, "alpha", "/repo/alpha", "feature-x")
	if err != nil {
		t.Fatalf("ResolveLane: %v", err)
	}
	if got.Kind != portalloc.LaneMatchNone {
		t.Fatalf("expected LaneMatchNone, got %#v", got)
	}
	if got.RepoPath != "" || len(got.Allocations) != 0 {
		t.Fatalf("expected empty match payload, got %#v", got)
	}
}

func TestResolveLaneIgnoresRowsFromOtherApps(t *testing.T) {
	rows := []portalloc.Allocation{
		{App: "beta", Lane: "feature-x", Mode: "dev", Service: "web", Port: 4001, RepoPath: "/repo/beta-feature-x"},
		{App: "gamma", Lane: "feature-x", Mode: "dev", Service: "web", Port: 5001, RepoPath: "/repo/gamma-feature-x"},
	}

	got, err := portalloc.ResolveLane(rows, "alpha", "/repo/alpha", "feature-x")
	if err != nil {
		t.Fatalf("ResolveLane: %v", err)
	}
	if got.Kind != portalloc.LaneMatchNone {
		t.Fatalf("expected LaneMatchNone for cross-app rows, got %#v", got)
	}
}

func TestResolveLaneAmbiguousAcrossWorktrees(t *testing.T) {
	rows := []portalloc.Allocation{
		{App: "alpha", Lane: "feature-x", Mode: "dev", Service: "web", Port: 3001, RepoPath: "/repo/alpha-feature-x"},
		{App: "alpha", Lane: "feature-x", Mode: "dev", Service: "web", Port: 3002, RepoPath: "/repo/alpha-other"},
	}

	got, err := portalloc.ResolveLane(rows, "alpha", "/repo/alpha", "feature-x")
	if err != nil {
		t.Fatalf("ResolveLane: %v", err)
	}
	if got.Kind != portalloc.LaneMatchAmbiguous {
		t.Fatalf("expected LaneMatchAmbiguous, got %#v", got)
	}
	if got.RepoPath != "" {
		t.Fatalf("expected empty repoPath for ambiguous match, got %q", got.RepoPath)
	}
	if len(got.Allocations) != 2 {
		t.Fatalf("expected both colliding rows enumerated, got %d (%#v)", len(got.Allocations), got.Allocations)
	}
}

func TestResolveLaneCurrentCheckoutWinsOverOtherWorktrees(t *testing.T) {
	rows := []portalloc.Allocation{
		{App: "alpha", Lane: "feature-x", Mode: "dev", Service: "web", Port: 3001, RepoPath: "/repo/alpha-feature-x"},
		{App: "alpha", Lane: "feature-x", Mode: "dev", Service: "web", Port: 3002, RepoPath: "/repo/alpha-other"},
	}

	got, err := portalloc.ResolveLane(rows, "alpha", "/repo/alpha-other", "feature-x")
	if err != nil {
		t.Fatalf("ResolveLane: %v", err)
	}
	if got.Kind != portalloc.LaneMatchSingle {
		t.Fatalf("expected current-checkout tiebreak to produce LaneMatchSingle, got %v", got.Kind)
	}
	if got.RepoPath != "/repo/alpha-other" {
		t.Fatalf("expected current checkout to win, got %q", got.RepoPath)
	}
	if len(got.Allocations) != 1 || got.Allocations[0].Port != 3002 {
		t.Fatalf("expected the current-checkout row, got %#v", got.Allocations)
	}
}

func TestResolveLaneSymlinkedRepoPathFoldsIntoSingleMatch(t *testing.T) {
	tmp := t.TempDir()
	canonical := filepath.Join(tmp, "canonical")
	if err := os.MkdirAll(canonical, 0o755); err != nil {
		t.Fatalf("mkdir canonical: %v", err)
	}
	link := filepath.Join(tmp, "link")
	if err := os.Symlink(canonical, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	rows := []portalloc.Allocation{
		{App: "alpha", Lane: "feature-x", Mode: "dev", Service: "web", Port: 3001, RepoPath: canonical},
		{App: "alpha", Lane: "feature-x", Mode: "dev", Service: "api", Port: 3002, RepoPath: link},
	}

	got, err := portalloc.ResolveLane(rows, "alpha", canonical, "feature-x")
	if err != nil {
		t.Fatalf("ResolveLane: %v", err)
	}
	if got.Kind != portalloc.LaneMatchSingle {
		t.Fatalf("expected symlink-equivalent rows to fold into LaneMatchSingle, got %v", got.Kind)
	}
	if len(got.Allocations) != 2 {
		t.Fatalf("expected both rows in folded match, got %d (%#v)", len(got.Allocations), got.Allocations)
	}
}

func TestResolveLaneCurrentCheckoutTiebreakRespectsSymlinks(t *testing.T) {
	tmp := t.TempDir()
	current := filepath.Join(tmp, "current")
	other := filepath.Join(tmp, "other")
	if err := os.MkdirAll(current, 0o755); err != nil {
		t.Fatalf("mkdir current: %v", err)
	}
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatalf("mkdir other: %v", err)
	}
	currentLink := filepath.Join(tmp, "current-link")
	if err := os.Symlink(current, currentLink); err != nil {
		t.Fatalf("symlink current: %v", err)
	}

	rows := []portalloc.Allocation{
		{App: "alpha", Lane: "feature-x", Mode: "dev", Service: "web", Port: 3001, RepoPath: current},
		{App: "alpha", Lane: "feature-x", Mode: "dev", Service: "web", Port: 3002, RepoPath: other},
	}

	got, err := portalloc.ResolveLane(rows, "alpha", currentLink, "feature-x")
	if err != nil {
		t.Fatalf("ResolveLane: %v", err)
	}
	if got.Kind != portalloc.LaneMatchSingle {
		t.Fatalf("expected current-checkout tiebreak to recognise the symlinked caller, got %v", got.Kind)
	}
	if got.RepoPath != current {
		t.Fatalf("expected current canonical row to win, got %q", got.RepoPath)
	}
}
