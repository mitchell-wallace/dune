# Spec 003: Dune/Rally Boundary And Naming

## Summary

- Introduce a clearer product and package split between the host control plane and the in-container agent runtime.
- Adopt `dune` as the host-side CLI and `rally` as the in-container agent orchestrator CLI.
- Adopt `gear` as the themed name for optional container capabilities currently called addons.
- Keep `001-ralph-go-migration.md` unchanged as the pre-rebrand runtime migration reference; this spec defines the naming and architectural boundary that later implementation work should follow.

## Why This Exists

- The term "orchestration" is overloaded today:
  - host-side `sand` orchestrates container lifecycle and provisioning
  - in-container Ralph is intended to orchestrate agent sessions and batches
- The current naming obscures the boundary between those responsibilities.
- The repo should support a future where `dune` and `rally` could be split into separate repos, without paying that cost yet.

## Naming Decisions

- Host CLI name: `dune`
- In-container agent runtime name: `rally`
- Optional installable capabilities name: `gear`

### Meaning

- `dune` represents the host-side environment and control plane around sandbox creation and access.
- `rally` represents the action of gathering and coordinating agents inside a sandbox.
- `gear` represents optional capabilities layered into a sandbox, whether CLI tools, service helpers, or runtime extras.

### Terms To Avoid Going Forward

- Avoid using "orchestrator" as a primary user-facing noun without qualification.
- Avoid using `sand-orch` as the long-term runtime CLI name.
- Avoid mixing host and in-container concerns under the same package or doc terminology.

## Product Boundary

### `dune` Owns

- host CLI entrypoints and UX
- loading and merging workspace config
- devcontainer generation and provisioning
- container naming and reconciliation
- workspace mount/copy behavior
- installing, mounting, or updating the `rally` artifact in containers
- injecting env vars and file paths needed by `rally`
- invoking stable shell runtime contracts in `container/runtime/`

### `rally` Owns

- in-container agent session and batch execution
- runner state and message/event models
- TUI and progress views
- progress record and repair commands
- import of legacy Ralph state if migration support is retained
- deterministic agent selection and transcript/session artifacts

### `gear` Owns

- the user-facing vocabulary for optional container capabilities
- install/list/status commands for those capabilities
- manifest entries describing installable capabilities
- helper commands exposed after capability installation

### `container/runtime/` Owns

- shell entrypoint and post-start behavior
- persistence seeding and symlink setup
- privileged command dispatch
- firewall setup
- service startup helpers
- `gear` installation plumbing

## Structural Rules

### Binaries

- Host binary: `cmd/dune`
- In-container runtime binary: `cmd/rally`

### Internal Packages

- Host packages live under `internal/dune/...`
- In-container runtime packages live under `internal/rally/...`
- Shared cross-boundary definitions live under `internal/contracts/rally/...`

### Hard Boundary Rule

- `cmd/dune` may not import `internal/rally/...`
- `cmd/rally` may depend on `internal/contracts/rally/...`
- `cmd/dune` may depend on `internal/contracts/rally/...`
- Host/runtime interaction should happen through artifact install, env vars, files, and CLI invocation, not direct in-process package reuse

## Contract Surface Between `dune` And `rally`

Keep the shared contract intentionally small and explicit.

### Allowed Shared Contract

- artifact name and install or mount destination
- required env vars
- data directory locations
- repo progress file location
- version handshake behavior
- health or doctor command output shape

### Examples

- `RALLY_CONTAINER_NAME`
- `RALLY_DATA_DIR`
- `RALLY_REPO_PROGRESS_PATH`
- `rally version --json`
- `rally doctor`

Exact names are implementation details to settle during migration work, but they should live in one contract package and one doc section.

## Repository Strategy

- Keep `dune` and `rally` in the same repo for now.
- Treat them as separate products with a hard architectural boundary.
- Design the code layout so a later repo split is mechanical rather than conceptual.

### Why Stay In One Repo Now

- host/runtime integration is still changing quickly
- env and artifact contracts are not stable yet
- agents working in one workspace can reason across provisioning and runtime behavior together
- coordinated refactors are cheaper before the boundary settles

### Conditions For Future Repo Split

- `rally` needs an independent release cadence
- non-`dune` hosts need to install or manage `rally`
- the contract between host and runtime has stabilized
- ownership or release friction becomes materially costly

## CLI Vocabulary Direction

### Host

- `dune`
- `dune config`
- `dune rebuild`

### Runtime

- `rally tui`
- `rally progress record`
- `rally progress repair`
- `rally import-legacy`

### Capabilities

- `gear list`
- `gear install <name>`
- `gear status`

Whether `gear` is its own binary or a subcommand exposed by runtime shell plumbing is an implementation decision. The user-facing term should still be `gear`.

## Migration Guidance

### Phase 1: Naming And Layout

- add this naming and boundary spec
- introduce package layout targets for `dune`, `rally`, and contract code
- keep `001` untouched as a historical runtime migration spec

### Phase 2: Runtime Implementation

- implement the in-container runtime as `rally`
- wire host-side provisioning to install or mount `rally`
- retain compatibility shims only where they materially reduce migration risk

### Phase 3: Capability Renaming

- rename user-facing "addons" language to `gear`
- update manifests, CLI help text, docs, and runtime scripts in a coordinated pass
- keep internal compatibility aliases where useful during transition

### Phase 4: Legacy Cleanup Completion

- remove internal `Addon*` type and field names in favor of `Gear*`
- remove parsing and persistence compatibility for legacy `addons` config and state paths
- remove host-side legacy `sand-*` container rename handling once the cleanup lands
- update tests and docs so `dune`, `gear`, and `rally` are the only active names in the supported path

## Documentation Rules

- `001-ralph-go-migration.md` remains unchanged for reference.
- New implementation work should cite both:
  - `001` for the original runtime migration intent
  - `003` for the chosen naming and boundary model
- Architecture docs should distinguish:
  - host control plane (`dune`)
  - in-container agent runtime (`rally`)
  - shell runtime substrate (`container/runtime/`)
  - optional capabilities (`gear`)

## Acceptance

- The repo has a single source of truth for the naming and boundary decision.
- Future implementation work can place code into `dune`, `rally`, and contract namespaces without re-debating ownership.
- `001` remains readable as the original migration reference without being rewritten around the rebrand.
- The architecture allows a future split into separate repos without requiring another conceptual redesign first.
