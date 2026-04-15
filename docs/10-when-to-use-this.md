# When to use devlane

Devlane is not a general-purpose local-dev tool. It targets a specific pain that shows up above a certain scale, and below that scale lighter-weight tools are a better fit.

## The trigger

**Use devlane when you have multiple agents working in parallel on the same machine.**

That's the sharpest way to state it. Coding agents are the forcing function: they work concurrently, they cannot read each other's shell history, they don't remember which port last worked, and they will cheerfully fight each other over `localhost:3000` until something is coordinated at host scope.

The same friction shows up — more slowly — with human developers who run many worktrees of the same repo, or who juggle several repos with overlapping port conventions. Those are also valid adoption triggers. But the "multiple agents" case is the one where the problem becomes unignorable fastest.

Secondary signals that reinforce the trigger:

- you run 3+ repos locally that all have opinions about `3000` or `8080`
- you keep more than one worktree of the same repo active at a time
- you hand-edit `.env.local` files across worktrees and lose track of which is current
- agents in your workflow regularly report "port already in use" or guess the wrong URL
- stable installs and work-in-progress installs co-exist on the host and have stepped on each other

Any one of these, by itself, is borderline. Two or more at once means you are probably past the adoption threshold.

## When devlane is overkill

If you are a single developer with one repo, one worktree, and one agent (or none), devlane is likely more machinery than you need. The contract, manifest, catalog, and adapter are worth learning when you have cross-repo or cross-worktree coordination problems; they are overhead when you don't.

For the single-repo case, lighter tools cover most of the same ground:

- per-directory env vars with a shell plugin
- a small task runner to keep commands consistent
- a version manager to keep toolchains reproducible

These tools do not coordinate across projects on the same host. That's the feature devlane adds, and it's the feature you only need once you have parallel work to coordinate.

## When devlane pays off

The ROI curve bends sharply as soon as any of these are true:

- **Multiple agents.** Agents running `devlane inspect --json` get authoritative port and URL information without scraping shell scripts or guessing. The manifest is the single contract they all read.
- **Many worktrees of the same repo.** Each worktree gets its own lane, its own ports allocated from the pool, and its own generated files. Stable keeps its fixture. No manual juggling.
- **Many repos with port collisions.** The host catalog arbitrates across repos. `3000` can belong to repo A's stable lane and `3100`+ to repo B's dev lanes, forever, without anyone remembering which is which.
- **Stable installs that must keep working.** Dev-lane churn cannot accidentally take stable's port or stable's hostname. The asymmetry is enforced, not a convention.

The tool pays for itself in "agent never asks 'what port is this on again'" moments.

## Degrees of adoption

You do not need to adopt all of devlane at once.

- **Minimum useful adoption:** an adapter, `inspect --json` as the agent contract, and `prepare` generating the files the repo currently hand-manages. This alone eliminates most cross-agent confusion.
- **Full adoption:** add `ports`, let the host catalog coordinate allocation across every repo on the host, and use `worktree create` / `worktree remove` to let the tool own lane lifecycle end-to-end when the adapter lives at the checkout root.

Many teams sit at the minimum level for a while and expand when they hit the next coordination problem. That is the intended path.

## Summary

Devlane exists because **coordinating ports, URLs, and generated files across many concurrent workers on one host is a real problem that nothing else solves cleanly**. If that's not your problem yet, use something lighter. If it is, the adapter + manifest + catalog model is designed to scale with you.

Start with `docs/30-quickstart.md` when you are ready.
