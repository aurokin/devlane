package portalloc

import (
	"net"
	"strings"
	"testing"

	"github.com/auro/devlane/internal/config"
)

func devOwner() Allocation {
	return Allocation{
		App:      "otherapp",
		Lane:     "feature-x",
		Mode:     "dev",
		Branch:   "feature-x",
		Service:  "api",
		Port:     3000,
		RepoPath: "/home/auro/code/otherapp",
	}
}

func stableOwner() Allocation {
	owner := devOwner()
	owner.Lane = "stable"
	owner.Mode = "stable"
	return owner
}

// reassignRecipe is the exact command line the offline/bound recipes must emit:
// it names the offending lane, forces a new allocation, points --cwd at the
// offending checkout (so it works across apps), and passes the offending
// service — each shell-quoted so the recipe is safe to paste.
func reassignRecipe(owner Allocation) string {
	return "devlane reassign --lane " + shellQuote(owner.Lane) + " --force --cwd " + shellQuote(owner.RepoPath) + " " + shellQuote(owner.Service)
}

func TestFormatCollisionStableHeldIsManualProse(t *testing.T) {
	owner := stableOwner()
	msg := formatCollision(collisionStableHeld, 3000, "web", owner)

	if !strings.HasPrefix(msg, `stable port 3000 for service "web"`) {
		t.Fatalf("missing stable-port prefix:\n%s", msg)
	}
	for _, want := range []string{owner.Lane, owner.App, owner.Service, owner.RepoPath, "human decision", "default", "stable_port"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected %q in message:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "devlane reassign") {
		t.Fatalf("stable-vs-stable is manual; it must not emit a reassign recipe:\n%s", msg)
	}
}

func TestFormatCollisionOfflineDevEmitsReassignRecipe(t *testing.T) {
	owner := devOwner()
	msg := formatCollision(collisionOfflineDev, 3000, "web", owner)

	if !strings.HasPrefix(msg, `stable port 3000 for service "web"`) {
		t.Fatalf("missing stable-port prefix:\n%s", msg)
	}
	// Acceptance criteria: the recipe names the service, the lane, and the port.
	for _, want := range []string{"web", "3000", owner.Lane, owner.Service, "not currently bound", reassignRecipe(owner), "devlane prepare"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected %q in message:\n%s", want, msg)
		}
	}
	// An offline lane is not running, so the recipe must not tell the operator
	// to stop a process first.
	if strings.Contains(msg, "Stop that lane") {
		t.Fatalf("offline recipe must not include a stop step:\n%s", msg)
	}
	// The prepare re-run must name the original invocation context (not a bare
	// "here"), and a deleted-worktree row must be routed to host gc, conditionally.
	for _, want := range []string{"same --cwd/--config", "no longer exists", "devlane host gc"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected %q in message:\n%s", want, msg)
		}
	}
}

func TestFormatCollisionBoundDevAddsStopStep(t *testing.T) {
	owner := devOwner()
	msg := formatCollision(collisionBoundDev, 3000, "web", owner)

	if !strings.HasPrefix(msg, `stable port 3000 for service "web"`) {
		t.Fatalf("missing stable-port prefix:\n%s", msg)
	}
	for _, want := range []string{"web", "3000", owner.Lane, owner.Service, "currently bound", "Free the port", "devlane down", reassignRecipe(owner), "devlane prepare", "same --cwd/--config", "devlane host gc"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected %q in message:\n%s", want, msg)
		}
	}
	// A probe cannot attribute the listener, so the recipe must not assert the
	// owning lane is definitely the one bound to the port.
	if !strings.Contains(msg, "otherwise stop whatever process holds the port") {
		t.Fatalf("bound recipe must not assume the owning lane is the listener:\n%s", msg)
	}
}

func TestFormatCollisionShellQuotesUnsafeRecipeValues(t *testing.T) {
	owner := devOwner()
	owner.RepoPath = "/tmp/app $(touch pwn)"
	msg := formatCollision(collisionOfflineDev, 3000, "web", owner)

	if !strings.Contains(msg, "--cwd '/tmp/app $(touch pwn)'") {
		t.Fatalf("an unsafe checkout path must be single-quoted in the recipe:\n%s", msg)
	}
	if strings.Contains(msg, "--cwd /tmp/app $(touch pwn)") {
		t.Fatalf("an unsafe checkout path must not appear unquoted:\n%s", msg)
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"feature-x":           "feature-x",
		"/home/auro/code/app": "/home/auro/code/app",
		"":                    "''",
		"with space":          "'with space'",
		"/tmp/$(touch pwn)":   "'/tmp/$(touch pwn)'",
		"it's":                `'it'\''s'`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Fatalf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatCollisionReservedNamesTheList(t *testing.T) {
	msg := formatCollision(collisionReserved, 3000, "web", Allocation{})

	if !strings.HasPrefix(msg, `stable port 3000 for service "web"`) {
		t.Fatalf("missing stable-port prefix:\n%s", msg)
	}
	for _, want := range []string{"reserved list", "Remove it from `reserved`", "devlane prepare"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected %q in message:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "devlane reassign") {
		t.Fatalf("a reserved port has no reassign recipe:\n%s", msg)
	}
}

func TestFormatCollisionUntrackedIsGeneric(t *testing.T) {
	msg := formatCollision(collisionUntracked, 3000, "web", Allocation{})

	if !strings.HasPrefix(msg, `stable port 3000 for service "web"`) {
		t.Fatalf("missing stable-port prefix:\n%s", msg)
	}
	for _, want := range []string{"does not track", "outside devlane", "devlane prepare"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected %q in message:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "devlane reassign") {
		t.Fatalf("an untracked holder has no reassign recipe:\n%s", msg)
	}
}

func TestClassifyStableCollision(t *testing.T) {
	if got := classifyStableCollision(3000, Allocation{}, false, false); got != collisionUntracked {
		t.Fatalf("no owner should be untracked, got %d", got)
	}
	// Reserved dominates even when a catalog row also sits on the fixture.
	if got := classifyStableCollision(3000, devOwner(), true, true); got != collisionReserved {
		t.Fatalf("reserved fixture should be reserved, got %d", got)
	}
	if got := classifyStableCollision(3000, stableOwner(), true, false); got != collisionStableHeld {
		t.Fatalf("stable owner should be stable-held, got %d", got)
	}

	// A dev owner whose port is free classifies as offline; binding the port
	// flips it to bound.
	if got := classifyStableCollision(3000, devOwner(), true, false); got != collisionOfflineDev {
		t.Fatalf("free dev owner should be offline, got %d", got)
	}
	listener, err := net.Listen("tcp4", "0.0.0.0:3000")
	if err != nil {
		t.Fatalf("listen on 3000: %v", err)
	}
	defer listener.Close()
	if got := classifyStableCollision(3000, devOwner(), true, false); got != collisionBoundDev {
		t.Fatalf("bound dev owner should be bound, got %d", got)
	}
}

// stableInspectFixture builds a minimal stable lane + single-service adapter
// (service "web", fixture 3000) and isolates the catalog under a temp config
// home so the integration cases below can seed the offending row.
func stableInspectFixture(t *testing.T) (*config.AdapterConfig, Lane) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	adapter := &config.AdapterConfig{
		App:   "myapp",
		Ports: []config.PortConfig{{Name: "web", Default: 3000}},
		Lane:  config.LaneConfig{StableName: "stable"},
	}
	lane := Lane{
		App:      "myapp",
		RepoPath: "/home/auro/code/myapp",
		Name:     "stable",
		Mode:     "stable",
		Branch:   "main",
		Stable:   true,
	}
	return adapter, lane
}

func TestInspectStableCollisionMessagesMatchFormatter(t *testing.T) {
	t.Run("held by another stable lane", func(t *testing.T) {
		adapter, lane := stableInspectFixture(t)
		owner := stableOwner()
		seedCatalogForMutate(t, owner)

		_, _, err := Inspect(adapter, lane)
		assertCollisionError(t, err, formatCollision(collisionStableHeld, 3000, "web", owner))
	})

	t.Run("held by an offline dev lane", func(t *testing.T) {
		adapter, lane := stableInspectFixture(t)
		owner := devOwner()
		seedCatalogForMutate(t, owner)

		_, _, err := Inspect(adapter, lane)
		assertCollisionError(t, err, formatCollision(collisionOfflineDev, 3000, "web", owner))
	})

	t.Run("held by a bound dev lane", func(t *testing.T) {
		adapter, lane := stableInspectFixture(t)
		owner := devOwner()
		seedCatalogForMutate(t, owner)

		listener, err := net.Listen("tcp4", "0.0.0.0:3000")
		if err != nil {
			t.Fatalf("listen on 3000: %v", err)
		}
		defer listener.Close()

		_, _, inspectErr := Inspect(adapter, lane)
		assertCollisionError(t, inspectErr, formatCollision(collisionBoundDev, 3000, "web", owner))
	})

	t.Run("reserved fixture dominates a catalog owner", func(t *testing.T) {
		adapter, lane := stableInspectFixture(t)
		adapter.Reserved = []int{3000}
		owner := devOwner()
		seedCatalogForMutate(t, owner)

		_, _, err := Inspect(adapter, lane)
		assertCollisionError(t, err, formatCollision(collisionReserved, 3000, "web", owner))
	})

	t.Run("bound by an untracked process surfaces the probe cause", func(t *testing.T) {
		adapter, lane := stableInspectFixture(t)

		listener, err := net.Listen("tcp4", "0.0.0.0:3000")
		if err != nil {
			t.Fatalf("listen on 3000: %v", err)
		}
		defer listener.Close()

		_, _, inspectErr := Inspect(adapter, lane)
		if inspectErr == nil {
			t.Fatal("expected an error when the fixture is bound by an untracked process")
		}
		// With no catalog owner and no reservation, the concrete probe cause is
		// preserved rather than guessing a catalog-collision remedy.
		msg := inspectErr.Error()
		if !strings.Contains(msg, "stable port 3000") || !strings.Contains(msg, "is unavailable:") {
			t.Fatalf("expected the wrapped probe error naming the fixture, got: %s", msg)
		}
		if strings.Contains(msg, "devlane reassign") {
			t.Fatalf("a probe failure must not emit a reassign recipe, got: %s", msg)
		}
	})
}

func assertCollisionError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a collision error, got nil")
	}
	if err.Error() != want {
		t.Fatalf("collision error mismatch\n got: %s\nwant: %s", err.Error(), want)
	}
}
