# Tech Stack

This document records the implementation-stack choices for `devlane` itself.

It is about the shared tool, not the repos that adopt it. Adopter repos can still be Go, Node, Ruby, Rust, or anything else. The point here is to make the `devlane` implementation choices explicit and stable enough for contributors and agents to reason about.

## Decisions

### Language: Go

`devlane` is implemented in Go.

Why:

- the tool is a host-level CLI, not an application framework or embedded library
- distribution matters: a single compiled binary is a better fit than requiring a language-specific environment bootstrap
- the core work is OS-facing and deterministic: files, locks, subprocesses, TCP probes, JSON/YAML, and git/compose integration
- the adapter contract is declarative, so the implementation does not benefit much from a dynamic scripting runtime

Why not Python:

- Python is fine for prototypes, but it pushes environment-management burden onto every machine that runs the tool
- `devlane` is meant to become a stable local control-plane utility, which benefits from a self-contained binary
- the implementation does not need a large Python-specific ecosystem to do its job

### CLI shape: standard `cmd/` + `internal/` layout

The repository uses the standard Go layout:

- `cmd/devlane/` for the executable entrypoint
- `internal/` for implementation packages

Why:

- this is the most legible default for a single-purpose Go CLI
- it keeps package boundaries obvious
- it reduces framework decisions and repo-specific cleverness

### Dependency posture: small runtime surface

The runtime dependency policy is conservative.

Current shape:

- standard library first
- YAML parsing via `go.yaml.in/yaml/v3`
- external tools are invoked through subprocesses when the contract calls for them (`git`, `docker compose`)

Why:

- this repo is primarily about contracts and deterministic orchestration
- fewer runtime dependencies make the CLI easier to audit, distribute, and reason about
- subprocess boundaries match the design principle that `devlane` orchestrates existing substrates rather than absorbing their logic

### Go version floor: Go 1.26

The repo currently targets Go 1.26 as declared in `go.mod`.

Why:

- it lets the repo use current language and tooling behavior without carrying legacy compatibility work
- the project is new, so there is no reason to optimize for older toolchains yet

If the version floor changes, update both `go.mod` and this document.

### Tooling model: module-pinned `go tool`

Developer tooling is pinned in `go.mod` using Go tool directives and run via `go tool`.

Current tools:

- `mvdan.cc/gofumpt`
- `golang.org/x/tools/cmd/goimports`
- `github.com/golangci/golangci-lint/cmd/golangci-lint`
- `gotest.tools/gotestsum`

Why:

- contributors do not need to install ad hoc global binaries
- tool versions live in the module instead of in tribal knowledge
- agents can run the same commands on every machine
- the workflow stays idiomatic to modern Go rather than adding another task runner layer too early

### Formatting policy

Formatting uses:

- `gofumpt` for stricter canonical formatting
- `goimports` for import organization

Why:

- the scaffold should start from a stricter formatting baseline than `gofmt` alone
- import cleanup should be mechanical and consistent

### Lint policy

Linting uses `golangci-lint` with a deliberately small enabled set, configured in `.golangci.yml`.

Current priorities:

- correctness (`govet`, `errcheck`, `staticcheck`, `ineffassign`, `unused`)
- readability and maintainability (`revive`, `misspell`)
- complexity pressure (`gocyclo`, `gocognit`)

Why:

- the project is small and contract-driven; correctness and readability matter more than maximizing lint coverage
- complexity limits are intentional because the tool should stay boring and easy to reason about
- broad linter bundles are useful, but only when the enabled rules match the repo's actual standards

### Test runner

Tests are run with `gotestsum`.

Why:

- the underlying test system is still `go test`
- `gotestsum` gives cleaner output for both humans and agents, especially as the suite grows

### Test style

The current test strategy is package-level unit tests with fixture-like temporary repos where needed.

Why:

- the implementation is still scaffold-stage
- the highest-value tests right now are deterministic contract checks around manifest generation, rendering, and CLI-adjacent helpers

As the catalog and worktree flows land, integration-style tests should expand around those boundaries.

## Commands

From the repo root:

```bash
go mod download
go tool gofumpt -w .
go tool goimports -w ./cmd ./internal
go tool golangci-lint run
go tool gotestsum -- ./...
```

## Change policy

Treat these as explicit project-policy choices.

If you change:

- the implementation language
- the Go version floor
- the formatter/import tool
- the linter entrypoint or enabled policy
- the test runner

update:

- this document
- `README.md`
- `AGENTS.md`
- any affected quickstart or contributor commands
