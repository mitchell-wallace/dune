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
- [ ] 3.3 Refactor `app.go` to be a thin entry point: parse CLI → handle `version`/`profile set`/`profile list` commands (no plan needed) → resolve workspace, config dir, data dir → load and resolve profile → call `fileExists` for `Dockerfile.dune` → call `plan.Build` → delegate to command handler functions. The resolution phase (all `os.Getenv`, `workspace.Resolve`, `version.BaseImageRef`, XDG dir reads, `fileExists`) must complete before `plan.Build` is called; no external state reads should occur after that point.
- [ ] 3.4 Remove the internal `project{}` struct.
- [ ] 3.5 Verify `app.go` contains no raw Docker command construction after the refactor. All Docker operations should be delegated to functions in `compose.go` or `docker.go`.

## 4. CommandRunner Seam

- [ ] 4.1 Create `internal/dune/runner.go` — define `CommandRunner` as a func type `func(ctx context.Context, dir string, stdout, stderr io.Writer, name string, args ...string) error`. Implement `defaultRunner` using `exec.CommandContext` as the production implementation.
- [ ] 4.2 Update functions in `docker.go` and `compose.go` that use `capture` to accept a `CommandRunner` parameter. Thread `defaultRunner` through all production call paths from `app.go`. `runStreaming` (used for PTY operations: `docker exec`, `docker compose logs -f`, `docker compose down`) is not threaded through `CommandRunner` — it remains a direct `os/exec` wrapper validated by smoke tests.
- [ ] 4.3 Add a `fakeRunner` test helper (in `runner_test.go` or a `testutil_test.go` file) that records each invocation's `dir`, `name`, and `args`, and returns preset `stdout` output and `error` values per call index or command pattern.
- [ ] 4.4 Replace `TestPrepareAgentImageReportsProgress` shell-shim with a `CommandRunner`-based test that asserts: `docker pull` is called with the correct base image ref when `UseBuild` is false; `docker compose build` is called with correct project, file, and service args when `UseBuild` is true.
- [ ] 4.5 Replace `TestEnsurePipelockConfigReconcilesExistingConfig` shell-shim with a `CommandRunner`-based test that asserts no Docker commands are issued when a valid Pipelock config already exists, and that `docker run --rm` with the correct image and args is issued when config generation is required.
- [ ] 4.6 Replace or rewrite `TestRunUsesSampleProjectFixtureForDockerfileWorkflow` to use `CommandRunner`-based assertions over the full `up` command sequence (pull, volume create, compose up) rather than shell shim output parsing. The `TestRenderComposeFilePassesDockerComposeConfig` real-Docker test continues to provide Docker Compose validation confidence.

## 5. Test Updates

- [ ] 5.1 Update `TestRenderComposeFileGolden` to construct `plan.EnvironmentPlan` directly instead of `project{}`.
- [ ] 5.2 Update `TestRenderComposeFilePassesDockerComposeConfig` to construct `plan.EnvironmentPlan` directly.

## 6. Verification

- [ ] 6.1 Run `go test ./...` — all tests pass, including golden, Docker validation (if Docker is available), and planner tests.
- [ ] 6.2 Run `go build ./cmd/dune` — binary builds cleanly.
- [ ] 6.3 Run existing smoke tests relevant to local Docker behaviour.

## 7. Documentation

- [ ] 7.1 Add or update an architecture document (e.g. `docs/architecture.md`) explaining the environment plan boundary: `EnvironmentPlan` describes what should exist; the Docker Compose layer currently describes how local Docker makes it exist; future backends must map the same semantic plan to their own execution model.
