## Why

`internal/dune/app.go` currently computes workspace/profile/config state and performs Docker execution in a single tightly coupled flow. There is no internal boundary between "what environment should exist" and "how Docker Compose makes it exist." Planning logic is interleaved with side effects, Docker assumptions leak into app-level code, and there is no testable surface for environment planning without a running Docker daemon. Changes to planning logic carry uncontrolled risk because failures cannot be isolated between the planning step, compose rendering, Docker execution, and image availability.

This change introduces a stable environment planning boundary that separates the semantic description of a Dune workspace from the Docker-specific operations that realise it. The goal is not backend generality for its own sake. The goal is to make environment planning testable without Docker, make `app.go` a legible entry point, and create a foundation for the Docker Compose backend extraction in the next change.

## What Changes

- **NEW** Add `internal/dune/plan` package with `EnvironmentPlan` and supporting types as a pure semantic description of a Dune environment.
- **NEW** Add `plan.Build(BuildInput) EnvironmentPlan` — a pure function that transforms pre-resolved inputs into an environment plan with no Docker, Git, or filesystem calls.
- **NEW** Add pure planner tests in `internal/dune/plan/builder_test.go` covering all planning cases without Docker.
- Move Docker execution helpers out of `app.go` into separate files within `package dune`, leaving `app.go` as a thin entry point.
- Refactor `app.go` to: parse CLI → handle profile/version commands → resolve workspace and profile → call `plan.Build` → delegate to command handlers.
- Update `renderComposeFile` and `ensureComposeFile` to consume `plan.EnvironmentPlan` instead of the internal `project` struct.
- Update `compose.yaml.tmpl` field access to use nested plan struct paths. Generated YAML output remains semantically identical.
- Remove the internal `project{}` struct.
- Preserve all existing golden Compose tests, Docker Compose validation tests, and smoke tests.

## Capabilities

### New Capabilities
- `environment-planning`: Pure environment plan construction that separates what a Dune environment should be from how any backend makes it exist.

### Modified Capabilities
- `compose-lifecycle`: Compose rendering now consumes `plan.EnvironmentPlan` rather than the internal `project` struct. No change to generated output or user-visible behaviour.

## Impact

- **Internal architecture**: Environment planning is now a pure, isolated transform from pre-resolved inputs to `EnvironmentPlan`. No Docker is required to build or test the plan.
- **`app.go`**: Becomes a thin, readable entry point. Docker execution helpers move to separate files within `package dune`.
- **Compose rendering**: Template field access changes to use nested plan paths. Values and generated YAML are unchanged.
- **User-facing behaviour**: None. All commands, file paths, volume names, config locations, and shell behaviour are preserved exactly.
- **Test coverage**: Pure planner tests added. All existing golden, Docker validation, and smoke tests preserved.
- **Stage 2 readiness**: The `plan.EnvironmentPlan` type and clean `app.go` entry point prepare directly for the Docker Compose backend extraction in `ref-2-docker-backend-abstraction`, which will introduce the `Backend` interface, `Runner` seam, and fake runner tests.
