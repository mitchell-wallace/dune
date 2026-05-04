````md
# Proposal: Introduce Environment Plan Boundary

## Summary

Introduce a first-stage architecture boundary that separates **what Dune environment should exist** from **how the current Docker Compose backend makes it exist**.

This change should preserve current behaviour while making Dune easier to modify safely, easier to test without full Docker/image rebuilds, and better positioned for future backend targets such as remote Docker on a VPS/Raspberry Pi or a MicroVM-based environment.

This proposal does **not** implement a new backend. Docker Compose remains the only supported backend.

## Project Understanding

Dune’s core value proposition is a single-command, profile-aware, persistent, isolated development environment for AI-assisted coding work.

Dune does not run agents. Rally owns agent-in-loop orchestration. Laps owns sequential task state. Pipelock owns outbound egress mediation and observability. Dune’s role is to provision and enter the environment where those tools operate.

Current durable product responsibilities:

- Provide one memorable host command that enters a repo-specific isolated workspace.
- Ship and use a batteries-included base image for AI coding work.
- Preserve auth, config, shell, and tooling state across sessions.
- Support separate profiles, such as work and hobby contexts.
- Mount the current repo into the environment.
- Route outbound HTTP(S) traffic through Pipelock.
- Support repo-specific extension via `Dockerfile.dune`.
- Provide lifecycle commands such as up, down, rebuild, logs, profile, and version.

The architecture should preserve this responsibility split:

```text
Dune     = provision, isolate, persist, connect, inspect
Rally    = run and recover agent workflows
Laps     = record sequential task state
Pipelock = mediate and observe egress
Base image = bundled workstation/runtime substrate
````

## Problem

Dune currently computes workspace/profile/config state and performs Docker-specific execution in one tightly coupled flow.

That makes changes risky because:

* planning logic is mixed with side effects;
* Docker/Compose assumptions leak through app-level code;
* tests either inspect rendered YAML, use brittle shell shims, or require real Docker;
* the full base-image feedback loop is slow;
* failures are difficult to isolate between planning, compose rendering, Docker execution, and image contents;
* future backends would be hard to introduce without rewriting core app flow.

The immediate goal is not backend generality for its own sake. The goal is to create a stable internal boundary that lets Dune answer:

```text
Given this repo, profile, config, Dune version, and backend target,
what environment should exist?
```

separately from:

```text
How does the selected backend make that environment exist?
```

## Objectives

This stage should:

1. Introduce a pure environment planning layer.
2. Keep Docker Compose as the only backend.
3. Preserve existing command behaviour.
4. Move Docker-specific concepts out of app-level planning where practical.
5. Make environment planning testable without Docker.
6. Keep Compose rendering testable against the semantic environment plan.
7. Create a cleaner base for later Docker backend extraction.
8. Avoid prematurely designing or implementing MicroVM/remote backends.

## Non-Goals

This stage should not:

* implement a MicroVM backend;
* implement remote Docker;
* replace Docker Compose;
* change the base image contents;
* change Rally or Laps responsibilities;
* redesign Pipelock integration;
* change profile semantics;
* change persisted auth/config paths;
* change `Dockerfile.dune` behaviour;
* introduce a plugin system;
* make the backend abstraction broad or generic before the current behaviour is fully modelled.

## Proposed Change

Create an internal environment planning layer that produces an `EnvironmentPlan`.

The plan should be a semantic description of the Dune workspace environment. It should not be a Docker Compose model.

Suggested package direction:

```text
internal/dune/
  app.go
  workspace/
  cli/
  profile/
  plan/
  runtime/
  runtime/dockercompose/
```

For this first stage, it may be acceptable to introduce only `plan/` and leave full backend extraction for a follow-up change. The priority is to separate pure planning from side-effectful execution.

## Core Concepts

### EnvironmentPlan

`EnvironmentPlan` describes the desired Dune environment for a workspace/profile.

Candidate shape:

```go
type EnvironmentPlan struct {
    Project      ProjectIdentity
    Environment EnvironmentSpec
    Persistence PersistenceSpec
    Egress      EgressSpec
    Files       GeneratedFileSpec
    Backend     BackendTarget
}
```

### ProjectIdentity

Represents stable host-side identity and generated state locations.

```go
type ProjectIdentity struct {
    Root         string
    Slug         string
    Profile      string
    InstanceName string
    DataDir      string
}
```

Current mappings:

* `Root`: resolved workspace root.
* `Slug`: existing Dune workspace slug.
* `Profile`: selected profile.
* `InstanceName`: current Compose project equivalent, e.g. `dune-<slug>-<profile>`.
* `DataDir`: current project data directory under `~/.local/share/dune/projects/<slug>`.

### EnvironmentSpec

Represents the interactive development environment the user enters.

This replaces the misleading idea of `AgentSpec`. Dune is not modelling the agent itself; it is modelling the environment where humans, agent CLIs, Rally, and Laps operate.

```go
type EnvironmentSpec struct {
    Name       string
    Image      ImageSpec
    Workspace  WorkspaceMount
    WorkingDir string
    Shell      []string
    Env        map[string]string
}
```

Current Docker Compose mapping:

* `EnvironmentSpec` maps to the `agent` service.
* Workspace root maps to `/workspace`.
* Shell currently maps to `zsh`.
* Environment variables include proxy configuration and timezone.
* Image comes from the published base image or repo-local `Dockerfile.dune`.

### ImageSpec

Represents the image/build decision independently from Docker command execution.

```go
type ImageSpec struct {
    BaseRef      string
    RuntimeRef   string
    BuildContext string
    Dockerfile   string
    UseBuild     bool
}
```

Current behaviour:

* If `<workspace>/Dockerfile.dune` exists, use a local built image.
* Otherwise use the published Dune base image.
* `rebuild` forces a no-cache rebuild where currently supported.

### WorkspaceMount

Represents the repo mount into the environment.

```go
type WorkspaceMount struct {
    HostPath  string
    GuestPath string
    Writable  bool
}
```

Current behaviour:

* Host workspace root mounts to `/workspace`.
* It should remain writable.

### PersistenceSpec

Represents preserved Dune profile state.

```go
type PersistenceSpec struct {
    LogicalName string
    Mounts      []PersistMount
}
```

Current Docker Compose mapping:

* `LogicalName` maps to `dune-persist-<profile>`.
* Docker backend renders this as a Docker volume.
* Other future backends may map this to a remote volume, host path, disk image, or VM-attached persistent store.

### EgressSpec

Represents network egress mediation.

This replaces the narrower idea of `ProxySpec`.

```go
type EgressSpec struct {
    Provider   string
    Image      string
    ConfigPath string
    Env        map[string]string
}
```

Current Docker Compose mapping:

* `Provider` is `pipelock`.
* `Image` is the configured Pipelock image.
* `ConfigPath` is `~/.config/dune/pipelock.yaml`.
* Backend renders this as a `pipelock` sidecar service.
* Environment receives `http_proxy`, `https_proxy`, and related proxy variables.

The semantic commitment is that Dune-managed environments have mediated/observable outbound HTTP(S), not that every backend must use a Docker sidecar forever.

### GeneratedFileSpec

Represents generated host-side files.

```go
type GeneratedFileSpec struct {
    ComposePath string
}
```

For now this may include Docker-specific generated file paths because Docker Compose remains the only backend. Over time, backend-specific generated files should move behind the backend implementation.

### BackendTarget

Represents where/how the plan is intended to run.

```go
type BackendTarget struct {
    Kind string
}
```

For this stage:

```text
Kind = "local-docker-compose"
```

Do not over-design this yet. It exists to avoid hard-coding the assumption that all future environments are local Docker daemon environments.

Later possible target kinds:

```text
local-docker-compose
remote-docker-compose
local-microvm
remote-microvm
```

## Design Decisions

### 1. Use an environment plan, not a Docker-shaped runtime interface

Avoid starting with this kind of abstraction:

```go
type Runtime interface {
    EnsureVolume(...)
    PullImage(...)
    RenderOrValidateCompose(...)
    ExecShell(...)
}
```

That hides Docker behind an interface but still encodes Docker’s worldview into the architecture.

Prefer:

```text
EnvironmentPlan -> Docker Compose renderer/executor
```

This lets Docker remain the only backend while preventing the core app flow from becoming permanently Compose-shaped.

### 2. Keep backend extraction narrow in this stage

This stage should extract planning first.

It is acceptable if `app.go` still calls existing Docker helper functions after receiving a plan, provided planning data no longer has to be recomputed across the execution path.

A follow-up change can extract a full `dockercompose` backend.

### 3. Preserve current names externally

Do not rename user-facing commands, files, volumes, or generated paths in this stage.

Internal naming can improve, but externally observable behaviour should remain stable.

Especially preserve:

* `dune`
* `dune up`
* `dune down`
* `dune rebuild`
* `dune logs`
* `dune logs pipelock`
* `dune profile set`
* `dune profile list`
* `~/.config/dune/profiles.json`
* `~/.config/dune/pipelock.yaml`
* `~/.local/share/dune/projects/<slug>/compose.yaml`
* `dune-persist-<profile>`
* `Dockerfile.dune`

### 4. Do not weaken Docker confidence

The goal is not to replace real Docker tests with mocks.

The goal is layered confidence:

```text
pure planner tests
compose render/golden tests
fake command contract tests
real Docker validation tests
smoke tests against existing image
full base-image build/release smoke tests
```

This stage should improve the lower layers without removing the upper layers.

### 5. Keep Pipelock explicit

Pipelock should not disappear into a generic proxy abstraction.

The semantic model can be `EgressSpec`, but the current provider should remain visibly Pipelock. Dune’s egress mediation is part of the product’s safety and observability model.

### 6. Keep Rally and Laps inside the environment boundary

Dune should continue to treat Rally and Laps as bundled tools available inside the environment, not as host-side orchestration responsibilities.

Do not add Dune-level task execution concepts in this change.

## Proposed Implementation Steps

### Step 1: Add plan package

Create:

```text
internal/dune/plan/
  plan.go
  builder.go
  builder_test.go
```

Move or wrap current project construction logic from `app.go` into a pure builder.

Candidate function:

```go
func BuildEnvironmentPlan(input BuildInput) (EnvironmentPlan, error)
```

Candidate input:

```go
type BuildInput struct {
    WorkspaceRoot string
    WorkspaceSlug string
    Profile       string
    ConfigDir     string
    DataDir       string
    BaseImageRef  string
    PipelockImage string
    Timezone      string
    HasDockerfile bool
}
```

The builder should not:

* call Docker;
* call Git;
* read or write files;
* create directories;
* render Compose;
* create Pipelock config;
* inspect image availability.

It should only transform already-known inputs into a semantic plan.

### Step 2: Adapt app flow to build and pass plan

Current app flow should become roughly:

```text
parse CLI
resolve workspace
load profile store
resolve profile
check Dockerfile.dune existence
build EnvironmentPlan
perform existing side effects using the plan
```

This is not yet a full backend extraction. It is a planning boundary.

### Step 3: Render Compose from EnvironmentPlan

Update Compose rendering so it consumes `EnvironmentPlan`, not an ad hoc project struct.

Current generated YAML should remain byte-for-byte identical if practical. If exact byte-for-byte stability is not practical, semantic equivalence must be covered by tests and reviewed carefully.

### Step 4: Add planner tests

Add table tests covering:

* default profile;
* explicit profile;
* stored profile;
* workspace with no `Dockerfile.dune`;
* workspace with `Dockerfile.dune`;
* timezone from `TZ`;
* timezone fallback to `UTC`;
* slug and project/instance name composition;
* compose path generation;
* persist logical name generation;
* Pipelock config path;
* base image ref;
* local image ref.

Planner tests should not require Docker.

### Step 5: Keep golden Compose tests

Existing golden Compose tests should remain.

Add at least one test that constructs an `EnvironmentPlan` directly and renders Compose from that plan.

This checks that the renderer depends on the semantic plan rather than hidden app state.

### Step 6: Add regression tests around current risk areas

Where possible in this stage, add focused tests for:

* profile name affects persistence name;
* profile name affects instance/project name;
* workspace slug stability for known input paths;
* `Dockerfile.dune` switches to local build image;
* absence of `Dockerfile.dune` uses base image;
* Pipelock env vars are present on the environment service;
* Pipelock service remains present in rendered Compose;
* workspace mount remains `/workspace`;
* default shell remains `zsh`.

### Step 7: Document the architecture boundary

Add or update architecture docs to explain:

```text
EnvironmentPlan describes what should exist.
Docker Compose currently describes how local Docker makes it exist.
Future backends must map the same product concepts, not Docker primitives.
```

This doc should call out that future backend candidates include:

* local Docker Compose;
* remote Docker Compose on a VPS/Raspberry Pi;
* local MicroVM;
* remote MicroVM.

But only local Docker Compose is currently supported.

## Acceptance Criteria

This change is complete when:

* A pure `EnvironmentPlan` or equivalent exists.
* Current app flow builds this plan before Docker execution.
* Compose rendering consumes the plan.
* Planner tests run without Docker.
* Current command behaviour is preserved.
* Existing Compose golden coverage remains.
* Existing smoke tests still pass.
* No new backend is exposed to users.
* No user-facing command or config migration is required.
* Internal naming avoids implying Dune runs agents directly.

## Test Plan

### Unit tests

Add pure tests for environment plan building.

Required cases:

```text
default profile
explicit profile
stored profile result passed into plan
workspace path to slug/project identity mapping
base image mode
Dockerfile.dune build mode
timezone override
timezone fallback
persist logical name
compose path
pipelock config path
proxy env shape
workspace mount shape
shell command shape
```

### Compose rendering tests

Keep existing golden test.

Add semantic assertions over rendered Compose:

```text
agent/environment service exists
pipelock service exists
workspace mount exists
persist mount exists
proxy env vars exist
timezone env var exists
expected image selected
local build config exists when Dockerfile.dune is present
```

### Integration-ish fake tests

If this stage touches command construction, keep shell-shim tests or introduce a minimal fake runner only where useful.

Do not let fake-runner work expand this stage into full backend extraction.

### Real Docker validation

Keep existing Docker-backed Compose validation test.

This is important because planner and renderer tests alone can still drift from actual Docker Compose behaviour.

### Smoke tests

Run existing smoke tests after the change.

This stage should not require a full base image rebuild unless files under `container/base` change.

## Key Risk Areas

### Profile persistence

Risk:

* Selected profile changes instance name and persistence.
* Stored mapping behaviour accidentally changes.

Mitigation:

* Tests for explicit profile vs stored profile vs default profile.
* Avoid changing `profiles.json` format.

### Persist volume naming

Risk:

* Changing `dune-persist-<profile>` would orphan existing user state.

Mitigation:

* Snapshot test persist logical name.
* Treat volume name as externally observable compatibility surface.

### Workspace slug stability

Risk:

* Changing slug derivation or project identity would orphan generated Compose state and local images.

Mitigation:

* Do not modify slug algorithm in this stage.
* Add fixed-input tests for slug/project identity.

### Compose project name stability

Risk:

* Existing containers may not be found by `down`, `logs`, or `exec`.

Mitigation:

* Preserve current project name shape.
* Test generated instance/project name.

### Compose path stability

Risk:

* Generated Compose files move unexpectedly.

Mitigation:

* Preserve `~/.local/share/dune/projects/<slug>/compose.yaml`.
* Test path derivation.

### Pipelock config preservation

Risk:

* Planning refactor accidentally changes config path, generation timing, or reconciliation behaviour.

Mitigation:

* Keep Pipelock config generation behaviour unchanged.
* Add tests around config path in the plan.
* Do not redesign merge/reconcile logic in this stage.

### Proxy env propagation

Risk:

* Environment starts but egress no longer routes through Pipelock.

Mitigation:

* Semantic Compose tests for `http_proxy`, `https_proxy`, and related env vars.
* Smoke test outbound access through Pipelock where currently available.

### Dockerfile.dune handling

Risk:

* Repo-specific custom image path breaks.
* Local image tag changes.
* Build context changes.

Mitigation:

* Test both base-image and local-build plan cases.
* Preserve current local image naming.
* Preserve build context as workspace root.

### Shell attach behaviour

Risk:

* `dune` enters the wrong container, wrong directory, or wrong shell.

Mitigation:

* Preserve shell spec.
* Add plan-level assertion for working dir and shell.
* Leave full PTY behaviour to existing smoke/manual tests.

### False confidence from mocks

Risk:

* More unit tests pass but Docker environment breaks.

Mitigation:

* Keep real Docker Compose validation.
* Keep smoke tests.
* Treat fake tests as command/plan contract tests, not proof of runtime correctness.

### Over-abstraction

Risk:

* First stage turns into a generic runtime framework.

Mitigation:

* Only model current Dune environment concepts.
* Keep Docker Compose as the only implementation.
* Do not introduce remote/MicroVM config beyond a placeholder backend target kind.

## Follow-Up Changes

This stage should prepare, but not include, these later changes:

1. `docker-compose-backend-extraction`

Move Docker command execution behind a backend implementation that consumes `EnvironmentPlan`.

2. `runtime-diagnostics`

Introduce structured user-facing errors with stable codes and recovery hints.

3. `dune-doctor`

Add host/config/backend diagnostic checks.

4. `smoke-observability`

Improve smoke-test phase logging, verbosity, and CI tiering.

5. `remote-backend-concept`

Explore remote Docker on VPS/Raspberry Pi as a separate backend target.

6. `microvm-backend-concept`

Document MicroVM portability constraints without committing to an implementation.

## Suggested OpenSpec Layout

```text
openspec/changes/introduce-environment-plan-boundary/
  proposal.md
  design.md
  tasks.md
  specs/
    environment-planning/
      spec.md
```

## Proposed Task Breakdown

```text
- [ ] Add `internal/dune/plan` package.
- [ ] Define `EnvironmentPlan` and supporting structs.
- [ ] Move current project/environment derivation into pure plan builder.
- [ ] Adapt app flow to build an environment plan before runtime side effects.
- [ ] Update Compose rendering to consume `EnvironmentPlan`.
- [ ] Preserve current generated Compose output or document/review intentional diffs.
- [ ] Add pure planner tests.
- [ ] Add semantic Compose rendering tests.
- [ ] Preserve existing golden Compose test.
- [ ] Preserve existing Docker Compose validation test.
- [ ] Run `go test ./...`.
- [ ] Run existing smoke tests relevant to local Docker behaviour.
- [ ] Update architecture docs with the environment-plan boundary.
```

## Review Checklist

Before accepting the change, verify:

```text
- Does Dune still clearly own environment provisioning, not agent execution?
- Does the model avoid the misleading name `AgentSpec`?
- Does `EnvironmentSpec` map cleanly to the current `agent` service?
- Does `EgressSpec` map cleanly to Pipelock without hiding it?
- Are profile and persistence behaviours unchanged?
- Are generated paths unchanged?
- Is Docker Compose still the only backend?
- Are planner tests independent of Docker?
- Are Docker-backed checks still present?
- Would a future remote Docker backend be easier after this change?
- Would a future MicroVM backend be less blocked after this change?
```

```
```
