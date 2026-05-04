````md
## Proposal: Extract Docker Compose Backend

## Summary

Extract current Docker and Docker Compose execution into an explicit local Docker Compose backend that consumes the `EnvironmentPlan` introduced by the previous change.

This change keeps Docker Compose as Dune’s only supported backend, but moves Docker-specific execution out of app-level orchestration. The result should make Dune safer to modify, easier to test, and better prepared for later backend targets such as remote Docker or MicroVM environments.

This is a behaviour-preserving refactor. It should not change user-facing commands, generated paths, profile behaviour, persistence, Pipelock behaviour, image selection, or shell attach semantics.

## Depends On

This proposal assumes the first-stage change exists:

```text
introduce-environment-plan-boundary
```

That change should provide a semantic `EnvironmentPlan` describing what Dune environment should exist.

This proposal focuses on how the current backend makes that plan exist using local Docker Compose.

## Project Boundary

Dune’s responsibility remains:

```text
provision, isolate, persist, connect, inspect
```

Dune does not run agents. Rally runs/retries agent workflows. Laps records sequential task state. Pipelock mediates and observes outbound egress. Dune creates the environment where those tools operate.

This change should preserve that responsibility split.

## Problem

Dune’s current app flow mixes:

- CLI command orchestration;
- workspace/profile resolution;
- environment planning;
- generated file handling;
- Pipelock config preparation;
- Docker prerequisite checks;
- Docker image pull/build;
- Docker volume creation;
- Docker Compose lifecycle commands;
- shell attach;
- logs streaming.

That makes the app layer difficult to reason about and difficult to test safely. Docker/Compose command construction is spread through app-level logic, and current test seams rely heavily on golden YAML, shell shims, or real Docker.

The previous stage separates the semantic environment plan. This stage should separate Docker Compose execution.

## Objectives

This change should:

1. Introduce a backend interface at the Dune environment lifecycle level.
2. Implement a single `local-docker-compose` backend.
3. Move Docker/Compose command execution out of `app.go`.
4. Keep backend helpers Docker-specific and backend-private.
5. Add a command runner seam for backend tests.
6. Preserve current generated Compose behaviour.
7. Preserve current command behaviour.
8. Make command construction testable without shell shims where practical.
9. Keep real Docker validation/smoke coverage for confidence.
10. Avoid implementing remote Docker, MicroVMs, or generic orchestration.

## Non-Goals

This change should not:

- add a second backend;
- expose backend selection to users unless already required internally;
- implement remote Docker;
- implement MicroVM support;
- replace Docker Compose;
- redesign the base image;
- redesign Pipelock config generation;
- add `dune doctor`;
- introduce broad structured diagnostics beyond what is needed to preserve useful errors;
- change profile persistence;
- change volume naming;
- change generated Compose paths;
- change shell attach behaviour;
- change Rally or Laps integration.

## Proposed Package Shape

Suggested package direction:

```text
internal/dune/
  app.go
  plan/
  runtime/
    backend.go
    options.go
  runtime/dockercompose/
    backend.go
    compose.go
    commands.go
    runner.go
    errors.go
```

Alternative acceptable shape:

```text
internal/dune/
  backend/
    backend.go
    dockercompose/
      ...
```

The important boundary is:

```text
app.go -> runtime.Backend -> dockercompose.Backend -> docker/compose commands
```

`app.go` should orchestrate high-level command flow. It should not construct raw `docker` or `docker compose` commands directly.

## Backend Interface

Use a lifecycle-oriented interface, not a Docker-primitive interface.

Preferred shape:

```go
type Backend interface {
    Validate(ctx context.Context) error
    Prepare(ctx context.Context, plan plan.EnvironmentPlan, opts PrepareOptions) error
    Start(ctx context.Context, plan plan.EnvironmentPlan, opts StartOptions) error
    Stop(ctx context.Context, plan plan.EnvironmentPlan, opts StopOptions) error
    Rebuild(ctx context.Context, plan plan.EnvironmentPlan, opts RebuildOptions) error
    Shell(ctx context.Context, plan plan.EnvironmentPlan, opts ShellOptions) error
    Logs(ctx context.Context, plan plan.EnvironmentPlan, opts LogsOptions) error
}
```

Options should carry only command-specific concerns:

```go
type PrepareOptions struct {
    Stdout io.Writer
    Stderr io.Writer
}

type StartOptions struct {
    Stdout io.Writer
    Stderr io.Writer
}

type StopOptions struct {
    Stdout io.Writer
    Stderr io.Writer
}

type RebuildOptions struct {
    NoCache bool
    Stdout  io.Writer
    Stderr  io.Writer
}

type ShellOptions struct {
    Stdout io.Writer
    Stderr io.Writer
    Stdin  io.Reader
}

type LogsOptions struct {
    Service string
    Follow  bool
    Stdout  io.Writer
    Stderr  io.Writer
}
```

Exact option names can vary, but the backend interface should stay at the environment lifecycle level.

Avoid this style:

```go
type Runtime interface {
    EnsureVolume(...)
    PullImage(...)
    RenderOrValidateCompose(...)
    ExecShell(...)
}
```

Those are Docker implementation details. They can exist as private methods inside the Docker Compose backend, but they should not define Dune’s backend contract.

## Docker Compose Backend Responsibilities

The `dockercompose` backend should own:

- Docker prerequisite validation;
- Pipelock config preparation if it remains coupled to Docker image execution;
- Compose file rendering;
- Compose file validation;
- persistence volume creation;
- base image pull;
- custom `Dockerfile.dune` build;
- Compose `up`;
- Compose `down`;
- Compose `logs`;
- running shell attach;
- checking whether the environment service is already running;
- collecting recent logs when startup fails.

Current Docker call sites should move into this backend.

Expected private helper shape:

```go
type Backend struct {
    runner Runner
    paths  Paths
}

func (b *Backend) validateDockerPrerequisites(ctx context.Context) error
func (b *Backend) ensurePipelockConfig(ctx context.Context, plan plan.EnvironmentPlan) error
func (b *Backend) ensureComposeFile(ctx context.Context, plan plan.EnvironmentPlan) error
func (b *Backend) validateComposeFile(ctx context.Context, plan plan.EnvironmentPlan) error
func (b *Backend) ensurePersistence(ctx context.Context, plan plan.EnvironmentPlan) error
func (b *Backend) prepareEnvironmentImage(ctx context.Context, plan plan.EnvironmentPlan, noCache bool, stdout, stderr io.Writer) error
func (b *Backend) composeUp(ctx context.Context, plan plan.EnvironmentPlan) error
func (b *Backend) isEnvironmentRunning(ctx context.Context, plan plan.EnvironmentPlan) (bool, error)
func (b *Backend) shell(ctx context.Context, plan plan.EnvironmentPlan, opts ShellOptions) error
func (b *Backend) logs(ctx context.Context, plan plan.EnvironmentPlan, opts LogsOptions) error
```

Naming can be adjusted to existing code style.

## Command Runner

Introduce a small command runner seam for backend tests.

Candidate interface:

```go
type Runner interface {
    Capture(ctx context.Context, dir string, name string, args ...string) (stdout string, stderr string, err error)
    Stream(ctx context.Context, dir string, stdin io.Reader, stdout io.Writer, stderr io.Writer, name string, args ...string) error
}
```

If TTY behaviour needs separate handling, use an explicit method:

```go
type Runner interface {
    Capture(ctx context.Context, dir string, name string, args ...string) (CommandResult, error)
    Stream(ctx context.Context, dir string, opts StreamOptions, name string, args ...string) error
    TTY(ctx context.Context, dir string, opts TTYOptions, name string, args ...string) error
}
```

Keep this seam small. It exists to test command construction and backend sequencing, not to create a general process framework.

The production runner should preserve current behaviour:

- captured commands return stdout/stderr for validation and error messages;
- streaming commands connect to provided stdout/stderr;
- shell attach preserves interactive terminal behaviour as much as current code does.

## App Flow After Change

High-level app flow should become:

```text
parse CLI
resolve workspace
load/resolve profile
build EnvironmentPlan
select backend: local Docker Compose
dispatch command to backend
```

Command mapping:

```text
dune / dune up
  backend.Validate
  backend.Prepare
  if environment not running:
    backend.Start
  backend.Shell

dune down
  backend.Validate
  backend.Prepare, if compose file is required for down
  backend.Stop

dune rebuild
  backend.Validate
  backend.Prepare
  backend.Rebuild
  backend.Shell

dune logs [service]
  backend.Validate
  backend.Prepare, if compose file is required for logs
  backend.Logs

profile set/list
  no backend required

version
  no backend required
```

Whether `Prepare` is called for `down` and `logs` should preserve current behaviour. If current commands ensure or refresh the Compose file before running, keep doing that unless there is a deliberate and tested change.

## Compose Rendering

Compose rendering should remain Docker backend-owned or Docker backend-adjacent.

Acceptable options:

### Option A: Renderer inside dockercompose package

```text
runtime/dockercompose/compose.go
```

Pros:

- Keeps Compose concepts fully backend-local.
- Avoids leaking Compose into app-level code.
- Best aligned with future backend targets.

### Option B: Renderer remains in plan-adjacent package temporarily

Pros:

- Smaller diff if first-stage planning already moved rendering.
- Easier incremental migration.

If Option B is used, the follow-up direction should still be to make Compose rendering backend-owned.

Preferred direction: Option A.

`EnvironmentPlan` is semantic. Compose YAML is the local Docker Compose backend’s rendering of that plan.

## Pipelock Config Handling

Current Pipelock config generation uses a Docker image command. That makes it partially backend-specific.

For this proposal, keep behaviour unchanged and move the existing logic into the Docker Compose backend.

Do not redesign Pipelock config reconciliation in this change.

However, preserve the conceptual distinction:

```text
EnvironmentPlan.Egress describes desired egress mediation.
dockercompose backend decides how Pipelock config is prepared and mounted.
```

Later changes can modularise Pipelock config management separately.

## Behaviour Compatibility Requirements

Preserve:

- command names;
- command outputs unless error messages are deliberately improved;
- profile resolution semantics;
- profile persistence file format;
- workspace slug algorithm;
- Compose project name shape;
- generated Compose file location;
- persistence volume name;
- base image ref behaviour;
- `Dockerfile.dune` detection and build behaviour;
- Pipelock config path;
- Pipelock generated/reconciled config behaviour;
- `dune logs pipelock`;
- shell attach target and working directory;
- default shell;
- timezone fallback behaviour.

## Error Handling Scope

This proposal may improve local error wrapping where needed, but should not attempt the full runtime diagnostics design.

Allowed:

- Preserve stderr in backend errors.
- Add backend-specific context such as “docker compose config failed”.
- Make command results easier to inspect in tests.

Avoid for this proposal:

- full diagnostic code taxonomy;
- user-facing `dune doctor`;
- large recovery-hint framework;
- JSON diagnostics output.

Those belong in the later `runtime-diagnostics` and `dune-doctor` proposals.

## Implementation Steps

### Step 1: Define backend interface

Add `internal/dune/runtime` or equivalent package.

Define:

- `Backend`;
- command option structs;
- any small shared result types.

Do not include Docker-specific methods in this interface.

### Step 2: Add Docker Compose backend package

Create `internal/dune/runtime/dockercompose`.

Move or wrap existing Docker helper functions into this package.

Initial implementation may be mostly relocation plus adaptation to consume `EnvironmentPlan`.

### Step 3: Add command runner

Move existing `capture` and `runStreaming` behaviour behind a `Runner`.

Production implementation should use `os/exec`.

Tests should be able to use a fake runner that records commands and returns configured results.

### Step 4: Move Docker prerequisite validation

Move:

```text
docker compose version
docker info
```

into `dockercompose.Backend.Validate`.

Preserve current failure behaviour as much as practical.

### Step 5: Move Compose args construction

Move Compose command argument construction into Docker backend.

Expected private helper:

```go
func (b *Backend) composeArgs(plan plan.EnvironmentPlan, args ...string) []string
```

This should consistently apply:

```text
docker compose -f <compose path> -p <project/instance name> ...
```

### Step 6: Move Compose file lifecycle

Move compose rendering, write, and validation into Docker backend.

Preserve atomic write behaviour if currently used.

Preserve `docker compose config` validation if currently used.

### Step 7: Move persistence preparation

Move Docker volume creation into backend.

The semantic plan should expose persistence logical name. The backend should decide that it is a Docker volume.

### Step 8: Move image preparation

Move image pull/build behaviour into backend.

Preserve:

- base image pull behaviour;
- local `Dockerfile.dune` build behaviour;
- no-cache rebuild behaviour;
- stdout/stderr streaming behaviour.

### Step 9: Move lifecycle commands

Move:

- `compose up`;
- `compose down`;
- `compose logs`;
- running-state check;
- startup failure log capture;
- shell attach.

Preserve command shapes and stream/capture behaviour.

### Step 10: Adapt app orchestration

Replace direct helper calls in `app.go` with backend calls.

`app.go` should no longer know how to spell raw Docker commands.

### Step 11: Add backend tests

Add fake runner tests covering backend sequencing and command construction.

Do not remove existing real Docker tests.

### Step 12: Update docs

Update architecture docs to explain:

```text
EnvironmentPlan is semantic.
Docker Compose backend renders and executes that plan locally.
Docker Compose remains the only supported backend.
```

## Testing Requirements

### Unit tests: backend command construction

Use fake runner tests for:

```text
Validate:
  docker compose version
  docker info

Prepare:
  pipelock config generation/reconciliation path
  compose render/write
  docker compose config
  docker volume create

Start:
  docker pull when using base image
  docker compose build when using Dockerfile.dune
  docker compose up -d

Rebuild:
  docker compose build --no-cache agent
  docker compose up -d --force-recreate

Stop:
  docker compose down

Logs:
  docker compose logs -f
  docker compose logs -f pipelock

Shell:
  docker exec / compose exec target matches current behaviour
```

Tests should assert:

- command name;
- args;
- working directory;
- capture vs stream mode;
- stdout/stderr handling where relevant.

### Unit tests: failure handling

Use fake runner tests for:

```text
docker compose version fails
docker info fails
docker compose config fails
docker volume create fails
docker pull fails
docker compose build fails
docker compose up fails
docker compose logs fallback fails
docker exec/shell fails
```

For this proposal, tests should check useful wrapping and stderr preservation, not final diagnostic code taxonomy.

### Compose rendering tests

Preserve existing golden Compose test.

Add or preserve semantic Compose checks for:

```text
environment/agent service
pipelock service
workspace mount
persist mount
proxy env vars
timezone
base image mode
Dockerfile.dune build mode
```

### Real Docker tests

Keep real Docker Compose validation where already present.

This is important because fake runner tests only validate command construction, not Docker’s interpretation of generated Compose.

### Smoke tests

Run existing smoke tests for:

```text
base image if relevant
local dune workflow
tool updates if affected
```

This proposal should not require a full base image rebuild unless container files change.

## Acceptance Criteria

This change is complete when:

- Docker/Compose execution is implemented in a Docker Compose backend package.
- App-level code no longer constructs raw Docker commands directly.
- Backend consumes `EnvironmentPlan`.
- Docker Compose remains the only supported backend.
- Existing user-facing behaviour is preserved.
- Command runner seam exists for backend tests.
- Fake runner tests cover key command construction paths.
- Existing golden Compose tests remain.
- Existing real Docker validation remains.
- Existing smoke tests pass.
- No new user config is required.
- No profile/config/data migration is required.

## Risk Areas

### Backend interface becomes Docker-shaped

Risk:

The backend contract exposes Docker primitives such as `EnsureVolume`, `PullImage`, or `ComposeUp`.

Mitigation:

Keep Docker details private to `runtime/dockercompose`. Public backend interface should use Dune lifecycle operations.

### Behaviour drift in command sequencing

Risk:

A refactor changes whether Dune pulls before building, validates Compose before creating volume, or attaches shell after start.

Mitigation:

Use fake runner tests to assert command order for major flows.

### Shell attach regression

Risk:

Interactive shell behaviour changes subtly because command execution moved behind a runner.

Mitigation:

Keep production TTY/streaming implementation close to existing behaviour. Cover command shape in unit tests and rely on smoke/manual tests for PTY behaviour.

### Compose path/project mismatch

Risk:

Backend uses a different compose path or project name, causing `down`, `logs`, or `exec` not to target existing containers.

Mitigation:

Use plan-derived paths/names. Add tests asserting exact `docker compose -f ... -p ...` args.

### Persistence regression

Risk:

Volume name changes or volume creation moves to the wrong point in the lifecycle.

Mitigation:

Assert `docker volume create dune-persist-<profile>` in backend tests.

### Pipelock regression

Risk:

Pipelock config generation or env propagation breaks during relocation.

Mitigation:

Keep behaviour unchanged. Add tests for config path usage and rendered proxy env vars.

### Local build regression

Risk:

`Dockerfile.dune` build mode changes build context, image tag, or no-cache behaviour.

Mitigation:

Add backend tests for base-image mode and local-build mode.

### False confidence from fake runner

Risk:

Fake tests pass while real Docker Compose breaks.

Mitigation:

Keep golden Compose tests, real Docker Compose validation, and smoke tests.

### Scope creep into diagnostics

Risk:

Backend extraction expands into a full error taxonomy or `dune doctor`.

Mitigation:

Only preserve and modestly improve error context in this proposal. Full diagnostics remains a separate change.

## Suggested OpenSpec Layout

```text
openspec/changes/extract-docker-compose-backend/
  proposal.md
  design.md
  tasks.md
  specs/
    runtime-backend/
      spec.md
    docker-compose-backend/
      spec.md
```

If OpenSpec prefers fewer spec areas, use:

```text
openspec/changes/extract-docker-compose-backend/
  proposal.md
  design.md
  tasks.md
  specs/
    environment-runtime/
      spec.md
```

## Proposed Tasks

```text
- [ ] Define lifecycle-oriented backend interface.
- [ ] Define backend command option/result types.
- [ ] Add Docker Compose backend package.
- [ ] Add production command runner.
- [ ] Add fake command runner for tests.
- [ ] Move Docker prerequisite validation into backend.
- [ ] Move Compose argument construction into backend.
- [ ] Move Compose rendering/writing/validation into backend.
- [ ] Move Docker volume creation into backend.
- [ ] Move base image pull and custom image build into backend.
- [ ] Move Compose up/down/logs into backend.
- [ ] Move running-state check into backend.
- [ ] Move shell attach into backend.
- [ ] Adapt app flow to dispatch through backend.
- [ ] Add fake runner tests for default/up flow.
- [ ] Add fake runner tests for down flow.
- [ ] Add fake runner tests for rebuild flow.
- [ ] Add fake runner tests for logs flow.
- [ ] Add fake runner tests for shell attach command shape.
- [ ] Add fake runner tests for key failure paths.
- [ ] Preserve golden Compose tests.
- [ ] Preserve real Docker Compose validation tests.
- [ ] Run `go test ./...`.
- [ ] Run relevant smoke tests.
- [ ] Update architecture docs.
```

## Review Checklist

Before accepting this change, verify:

```text
- Does app-level code still know how to spell `docker` commands?
- Does the backend consume `EnvironmentPlan` rather than reconstructing planning data?
- Is the backend interface lifecycle-oriented rather than Docker-primitive-oriented?
- Are Docker-specific helpers private to the Docker Compose backend?
- Is Pipelock still explicit in the rendered environment?
- Are profile, slug, compose path, and volume naming unchanged?
- Is `Dockerfile.dune` behaviour unchanged?
- Is shell attach behaviour unchanged?
- Are fake runner tests checking command order and command args?
- Are real Docker validation tests still present?
- Are smoke tests still meaningful?
- Did this avoid adding remote Docker or MicroVM behaviour prematurely?
```

## Follow-Up Changes

Expected next proposals:

```text
improve-runtime-diagnostics
add-dune-doctor
assess-next-backend-targets
```

`improve-runtime-diagnostics` should build on this backend boundary by adding stable error codes, recovery hints, and stronger failure-mode tests.

`add-dune-doctor` should reuse backend checks but expose them through a user-facing diagnostic command.

`assess-next-backend-targets` should evaluate the next architectural direction: remote Docker, local MicroVM, remote MicroVM, or continued local Docker hardening.
````
