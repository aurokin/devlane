package portalloc

import (
	"errors"
	"fmt"
	"strings"
)

// collisionScenario classifies why a stable lane's fixture port cannot be
// claimed so the error can carry a copy-pasteable recovery recipe instead of a
// bare "unavailable". collisionOfflineDev/collisionBoundDev/collisionStableHeld
// map to the documented scenarios in docs/65-host-catalog.md; the remaining two
// are catch-alls for host constraints the catalog does not own.
type collisionScenario int

const (
	// collisionStableHeld: the fixture is the stable fixture of another lane.
	// Two stable fixtures cannot share a port; which one moves is a human
	// decision, so no command can resolve it automatically.
	collisionStableHeld collisionScenario = iota
	// collisionOfflineDev: the fixture is claimed by a dev lane and the port is
	// not currently bound. Reassigning that lane off the fixture frees it.
	collisionOfflineDev
	// collisionBoundDev: the fixture is claimed by a dev lane and the port is
	// currently bound. The listener must be stopped to release the port and the
	// lane reassigned off the fixture. A probe can tell that the port is bound
	// but not by whom, so the recipe does not assume the owning lane is the
	// listener.
	collisionBoundDev
	// collisionReserved: the fixture is on the host or adapter reserved list.
	// Reserved is checked before catalog ownership in isAvailable, so it
	// dominates any catalog row that also sits on the port.
	collisionReserved
	// collisionUntracked: defensive fallback for a fixture with no catalog owner
	// that is not reserved. chooseStablePort surfaces genuine bind failures with
	// their concrete probe cause before reaching the classifier, so this is not
	// produced on the live path; it guards against a future caller passing an
	// unowned, unreserved fixture rather than emitting a recipe for a zero owner.
	collisionUntracked
)

// stableCollisionError builds the operator-facing error returned when a stable
// lane's fixture port cannot be claimed. It classifies the collision — a probe
// distinguishes an offline dev lane from a bound one — and renders the recipe
// via the pure formatCollision helper.
func stableCollisionError(fixture int, service string, owner Allocation, owned, reserved bool) error {
	return errors.New(formatCollision(classifyStableCollision(fixture, owner, owned, reserved), fixture, service, owner))
}

// classifyStableCollision decides which scenario applies. owner is the catalog
// row holding the fixture when owned is true. reserved mirrors isAvailable's
// reserved check, which runs before catalog ownership, so a reserved fixture is
// reported as reserved even when a catalog row also sits on it — reassigning
// that row would not make the port usable. This is the only impure step: for a
// dev-lane owner it probes the fixture to tell an offline lane from a bound one.
func classifyStableCollision(fixture int, owner Allocation, owned, reserved bool) collisionScenario {
	if reserved {
		return collisionReserved
	}
	if !owned {
		return collisionUntracked
	}
	if strings.EqualFold(strings.TrimSpace(owner.Mode), "stable") {
		return collisionStableHeld
	}
	if probePort(fixture) == nil {
		return collisionOfflineDev
	}
	return collisionBoundDev
}

// reassignNote is appended to the reassign command line. The recipe's --cwd is
// the offending lane's worktree root (the catalog does not store the adapter's
// config path); for a monorepo subtree adapter that lives below the worktree
// root, the operator must point --cwd/--config at the adapter instead.
const reassignNote = "    # adjust --cwd/--config if that lane's adapter is not at the checkout root"

// shellQuote renders s as a single shell word that is safe to paste into a
// POSIX shell. Values made only of unambiguous characters are returned as-is;
// anything else is wrapped in single quotes (with embedded single quotes
// escaped as '\”), so a path, lane, or service containing whitespace or shell
// metacharacters cannot split arguments or trigger command substitution when
// the recipe is pasted.
func shellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool { return !isShellSafe(r) }) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func isShellSafe(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	default:
		return strings.ContainsRune("_@%+=:,./-", r)
	}
}

// formatCollision renders the recovery message for an already-classified
// collision. It is pure — no I/O — and is the unit under test in
// collision_test.go. Every message begins with "stable port %d for service %q"
// so callers and tests that match on that prefix keep working.
func formatCollision(scenario collisionScenario, fixture int, service string, owner Allocation) string {
	switch scenario {
	case collisionStableHeld:
		return fmt.Sprintf(
			"stable port %d for service %q is unavailable: it is the stable fixture of lane %q (app %q, service %q) at %s.\n"+
				"Two stable fixtures cannot share a port — choosing which one moves is a human decision.\n"+
				"Edit one adapter's `default`/`stable_port` so the fixtures differ, then re-run `devlane prepare`.",
			fixture, service, owner.Lane, owner.App, owner.Service, owner.RepoPath)

	case collisionOfflineDev:
		return fmt.Sprintf(
			"stable port %d for service %q is unavailable: it is claimed by dev lane %q (app %q, service %q) at %s, which is not currently bound.\n"+
				"Reassign that lane off the fixture, then re-run your original prepare (with the same --cwd/--config/--mode you ran it with):\n"+
				"  devlane reassign --lane %s --force --cwd %s %s%s\n"+
				"  devlane prepare\n"+
				"(If %s no longer exists, that worktree was removed — drop the stale row with `devlane host gc`, then re-run prepare.)",
			fixture, service, owner.Lane, owner.App, owner.Service, owner.RepoPath,
			shellQuote(owner.Lane), shellQuote(owner.RepoPath), shellQuote(owner.Service), reassignNote,
			owner.RepoPath)

	case collisionBoundDev:
		return fmt.Sprintf(
			"stable port %d for service %q is unavailable: it is claimed by dev lane %q (app %q, service %q) at %s and the port is currently bound.\n"+
				"Free the port, reassign that lane off the fixture, then re-run your original prepare (with the same --cwd/--config/--mode you ran it with):\n"+
				"  # release port %d: if dev lane %q is the listener, run `devlane down` in %s; otherwise stop whatever process holds the port\n"+
				"  devlane reassign --lane %s --force --cwd %s %s%s\n"+
				"  devlane prepare\n"+
				"(If %s no longer exists, that worktree was removed — drop the stale row with `devlane host gc`, then re-run prepare.)",
			fixture, service, owner.Lane, owner.App, owner.Service, owner.RepoPath,
			fixture, owner.Lane, owner.RepoPath,
			shellQuote(owner.Lane), shellQuote(owner.RepoPath), shellQuote(owner.Service), reassignNote,
			owner.RepoPath)

	case collisionReserved:
		return fmt.Sprintf(
			"stable port %d for service %q is unavailable: it is on the host or adapter reserved list.\n"+
				"Remove it from `reserved`, or choose a different stable fixture in the adapter, then re-run `devlane prepare`.",
			fixture, service)

	default: // collisionUntracked
		return fmt.Sprintf(
			"stable port %d for service %q is unavailable: it is held by a process devlane does not track (started outside devlane).\n"+
				"Free the port (stop that process) or choose a different stable fixture in the adapter, then re-run `devlane prepare`.",
			fixture, service)
	}
}

// collisionOwner returns the catalog row currently holding fixture, preferring a
// persisted row (held) over one claimed earlier in the current resolve pass.
func collisionOwner(fixture int, held, claimed map[int]Allocation) (Allocation, bool) {
	if owner, ok := held[fixture]; ok {
		return owner, true
	}
	if owner, ok := claimed[fixture]; ok {
		return owner, true
	}
	return Allocation{}, false
}
