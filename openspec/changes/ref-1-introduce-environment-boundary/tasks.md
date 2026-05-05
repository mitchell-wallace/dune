## 1. Plan Package

- [ ] 1.1 Create `internal/dune/plan/plan.go` — define `EnvironmentPlan`, `ProjectIdentity`, `EnvironmentSpec`, `ImageSpec`, `WorkspaceMount`, `PersistenceSpec`, `EgressSpec`, and `BuildInput` types as specified in `design.md`.
- [ ] 1.2 Create `internal/dune/plan/builder.go` — implement `Build(BuildInput) EnvironmentPlan`. The function must be pure: no `os/exec`, no file I/O, no network calls. Verify the `plan` package imports do not include `os/exec` or any package that shells out.
- [ ] 1.3 Create `internal/dune/plan/builder_test.go` — add table-driven tests for all planning cases listed in the environment-planning spec. Tests must pass without a running Docker daemon.

## 2. Compose Rendering

- [ ] 2.1 Define an internal `composeRenderData` struct in `package dune` that embeds `plan.EnvironmentPlan` and adds backend-specific rendering fields (`PipelockImage string` from `pipelock.ImageRef()`).
- [ ] 2.2 Update `renderComposeFile` signature to accept `plan.EnvironmentPlan`. Build the `composeRenderData` internally and render the template from it.
- [ ] 2.3 Update `compose.yaml.tmpl` field access to use nested plan struct paths consistent with `composeRenderData`. Generated YAML values must remain semantically identical to current output.
- [ ] 2.4 Update `testdata/compose.golden.yaml` if any generated output changes (values must not change; only template path expressions change).

## 3. App Restructuring

- [ ] 3.1 Create `internal/dune/compose.go` — move `renderComposeFile`, `ensureComposeFile`, and `validateComposeFile` from `app.go` into this file. Update them to accept `plan.EnvironmentPlan`.
- [ ] 3.2 Create `internal/dune/docker.go` — move `validateDockerPrerequisites`, `ensureVolume`, `prepareAgentImage`, `composeUp`, `isAgentRunning`, `isAgentCreated`, `composeArgs`, `localImageExists`, `capture`, and `runStreaming` from `app.go`. Update any functions that took `project{}` to take `plan.EnvironmentPlan`.
- [ ] 3.3 Refactor `app.go` to be a thin entry point: parse CLI → handle `version`/`profile set`/`profile list` commands (no plan needed) → resolve workspace, config dir, data dir → load and resolve profile → call `fileExists` for `Dockerfile.dune` → call `plan.Build` → delegate to command handler functions.
- [ ] 3.4 Remove the internal `project{}` struct.
- [ ] 3.5 Verify `app.go` contains no raw Docker command construction after the refactor. All Docker operations should be delegated to functions in `compose.go` or `docker.go`.

## 4. Test Updates

- [ ] 4.1 Update `TestRenderComposeFileGolden` to construct `plan.EnvironmentPlan` directly instead of `project{}`.
- [ ] 4.2 Update `TestRenderComposeFilePassesDockerComposeConfig` to construct `plan.EnvironmentPlan` directly.
- [ ] 4.3 Update `TestRunUsesSampleProjectFixtureForDockerfileWorkflow` to reflect the refactored code paths. The Docker shim and assertions should remain substantively unchanged; only internal references to `project{}` fields need updating.
- [ ] 4.4 Update `TestPrepareAgentImageReportsProgress` to pass `plan.EnvironmentPlan` to `prepareAgentImage`.
- [ ] 4.5 Update `TestEnsurePipelockConfigReconcilesExistingConfig` if the function signature changed.

## 5. Verification

- [ ] 5.1 Run `go test ./...` — all tests pass, including golden, Docker validation (if Docker is available), and planner tests.
- [ ] 5.2 Run `go build ./cmd/dune` — binary builds cleanly.
- [ ] 5.3 Run existing smoke tests relevant to local Docker behaviour.

## 6. Documentation

- [ ] 6.1 Add or update an architecture document (e.g. `docs/architecture.md`) explaining the environment plan boundary: `EnvironmentPlan` describes what should exist; the Docker Compose layer currently describes how local Docker makes it exist; future backends must map the same semantic plan to their own execution model.
