## Context

Dune is a single-command, profile-aware, persistent, isolated development environment for AI-assisted coding. The host-side `dune` CLI (Go) resolves workspace identity, selects a profile, generates a Docker Compose file, and starts a two-container workspace: an `agent` interactive shell container and a `pipelock` HTTP(S) egress proxy sidecar.

The current implementation computes all state and executes all Docker operations in `internal/dune/app.go`. The internal `project{}` struct holds derived values and is passed directly to compose rendering and Docker helpers. There is no isolated planning step and no stable seam between deciding what the environment should be and making it so.

This change introduces the planning boundary only. Docker Compose remains the only backend. The Docker Compose backend extraction (including a `Backend` interface and migration of Docker code into `runtime/dockercompose`) is the scope of `ref-2-docker-backend-abstraction`.

## Goals / Non-Goals

**Goals:**
- Introduce a pure `EnvironmentPlan` that semantically describes the desired Dune environment.
- Implement `plan.Build(BuildInput) EnvironmentPlan` as a pure function with no I/O.
- Make environment planning testable without Docker.
- Make `app.go` a thin, readable entry point using progressive disclosure.
- Move Docker helpers out of `app.go` into separate files within `package dune`.
- Update compose rendering to consume `EnvironmentPlan`.
- Introduce a `CommandRunner` func type as a low-level execution seam so Docker helper functions can be tested without a real Docker daemon.
- Replace shell-shim tests with `CommandRunner`-based command construction tests.
- Preserve all user-facing behaviour, generated paths, and volume names exactly.
- Preserve and strengthen test coverage: pure planner tests added, golden and Docker validation tests preserved, shell-shim tests replaced with `CommandRunner`-based tests, smoke tests unchanged.

**Non-Goals:**
- Introducing a `Backend` interface (ref-2).
- Moving Docker execution into a separate `runtime/dockercompose` package (ref-2).
- Adding a second backend.
- Implementing remote Docker or MicroVM support.
- Redesigning the base image, Pipelock config generation, or profile semantics.
- Changing user-facing commands, paths, volume names, or generated YAML.

## Guiding Principles

When an implementing agent encounters an unforeseen decision during this change, these principles apply:

1. **Preserve observable behaviour.** Every generated path, volume name, compose project name, and command output must remain identical. Treat these as externally observable compatibility surfaces.
2. **Keep the plan pure.** `plan.Build` must never call `os/exec`, read files, create directories, or perform any I/O. If a value is needed, it must come from `BuildInput`. If tempted to add I/O to the planner, stop and pass the result as a `BuildInput` field instead.
3. **Plan describes what, not how.** The plan must not encode Docker primitives. Fields that describe network addresses, sidecar image refs, volume types, or compose-specific names belong in the backend, not the plan.
4. **Thin app.go.** `app.go` should be readable top-to-bottom without encountering raw Docker command construction. A reader should be able to understand the overall command flow from `app.go` and follow to separate files for Docker execution detail.
5. **Do not weaken Docker confidence.** Adding pure planner tests is additive. Existing golden tests, Docker Compose validation tests, and smoke tests must remain and pass.

## Decisions

### D1: Nested struct hierarchy, not a flat struct

The `EnvironmentPlan` uses a nested struct hierarchy grouping fields by concern:

```go
type EnvironmentPlan struct {
    Project     ProjectIdentity
    Environment EnvironmentSpec
    Persistence PersistenceSpec
    Egress      EgressSpec
}
```

The internal `project{}` struct was flat and Docker-shaped. The new type is semantic. Nesting is preferred because the logical groupings are meaningful independent of any backend, the type will be consumed by future backends that should not need to understand Docker naming conventions, and a flat struct would just reproduce the problems of `project{}` under a different name.

The compose template field access changes to use nested paths (e.g. `.Environment.Image.RuntimeRef` instead of `.AgentImage`). Generated YAML values are identical.

### D2: EgressSpec encodes intent, not implementation

`EgressSpec` carries the egress provider identifier and the host-side config path. It does not carry:
- proxy environment variable values (e.g. `http://pipelock:8888`)
- Docker image references for the egress sidecar
- any network address specific to a Docker Compose topology

```go
type EgressSpec struct {
    Provider   string // "pipelock"
    ConfigPath string // absolute host path to provider config
}
```

The compose renderer, as a Docker backend artifact, knows to set `http_proxy=http://pipelock:8888` because it knows the sidecar service name. This knowledge lives in the renderer, not the plan. A future MicroVM backend would derive its own proxy address from its own network model.

This keeps the plan semantically correct: "I want HTTP(S) egress mediated by Pipelock, configured from this file" — not "I want a proxy at this specific Docker service address."

### D3: No generated file paths in the plan

`ComposePath` and any equivalent `GeneratedFileSpec` are not included in `EnvironmentPlan`. The plan describes the desired environment; where the backend stores generated files is a backend concern.

The Docker Compose backend (currently `package dune`, later `runtime/dockercompose`) derives `ComposePath` from `Project.DataDir`. This derivation happens in the execution layer, not the planner.

### D4: No BackendTarget in the plan

The plan does not carry a `BackendTarget` or `Kind` field. Backend selection is the caller's responsibility (`app.go`). Embedding the target in the plan would create a circular concern: the plan describes what to build, and the caller already knows how to build it.

### D5: InstanceName in ProjectIdentity

The user-visible environment instance identifier (currently the Docker Compose project name, e.g. `dune-demo-app-96-work`) is included in the plan as `Project.InstanceName`. This is semantically justified: it is the stable, user-visible name for this workspace/profile combination that appears in `dune down`, `dune logs`, and future `dune doctor` output. The Docker backend uses it as the Compose project name; other backends would use it as their equivalent instance identifier.

### D6: TZ as a direct field on EnvironmentSpec

`EnvironmentSpec.TZ` is a named field rather than a key in a generic `Env map[string]string`. The timezone is the only environment variable that comes from the semantic plan. Proxy env vars (`http_proxy`, etc.) are backend-derived and not in the plan (see D2). A generic map would be premature until there are additional semantic env vars to carry.

### D7: Docker helpers move to separate files, not a new package

For this stage, Docker execution helpers move out of `app.go` into separate files within `package dune` (e.g. `compose.go`, `docker.go`). A new package for Docker execution is the scope of ref-2 when the `Backend` interface is introduced. Introducing a package without an interface would just create an intermediate state to be refactored again immediately.

The result after ref-1 is: `app.go` is thin and delegates to functions in other files. After ref-2, those functions migrate to `runtime/dockercompose/`.

### D8: Pipelock config generation stays in the execution layer

`ensurePipelockConfig` calls Docker to run the Pipelock container and generate a baseline config. This is fundamentally a Docker operation and stays in the Docker execution layer. The plan carries `Egress.ConfigPath` as the semantic location of the config. The execution layer is responsible for ensuring that file exists and is up to date.

### D9: Compose renderer uses an internal render context

`renderComposeFile` constructs an internal `composeRenderData` struct that embeds `plan.EnvironmentPlan` and adds backend-specific rendering fields (currently `PipelockImage string` from `pipelock.ImageRef()`). The template renders from this struct. This allows the template to remain a single embedded text file while keeping backend-specific details out of the plan.

When ref-2 moves the template into `runtime/dockercompose/`, this render context naturally moves with it.

### D10: plan package enforced as pure

`internal/dune/plan` must not import `os/exec`, `os` (for I/O operations), or any package that performs Docker, Git, or filesystem calls. Reviewers should verify this at the import level. Violations indicate that a value should be passed via `BuildInput` instead.

### D11: app.go owns a synchronous resolution phase before plan.Build

Before calling `plan.Build`, `app.go` performs a single synchronous resolution phase that reads all external state needed to populate `BuildInput`. This phase is responsible for:

- Calling `workspace.Resolve` to obtain `WorkspaceRoot` and `WorkspaceSlug`
- Reading `XDG_CONFIG_HOME` (defaulting to `~/.config`) for `ConfigDir`
- Reading `XDG_DATA_HOME` (defaulting to `~/.local/share`) for `DataDir`
- Reading `os.Getenv("TZ")` for `Timezone`
- Calling `version.BaseImageRef()` for `BaseImageRef`
- Calling `fileExists(ws.Root + "/Dockerfile.dune")` for `HasDockerfile`
- Loading and resolving the profile from the profile store

After this phase, `plan.Build` receives a fully populated `BuildInput` with no residual external state reads. The plan package never calls `os.Getenv`, `os.Stat`, or any workspace functions.

This is important for three reasons. First, `BuildInput` becomes a complete, testable record of all resolved state — tests can construct any `BuildInput` directly without touching the filesystem. Second, future backends can invoke `plan.Build` with the same `BuildInput` without re-implementing resolution logic. Third, it gives `app.go` a clear two-phase structure: resolve → plan → execute.

### D12: CommandRunner seam at the command execution level

Docker helper functions in `docker.go` and `compose.go` accept a `CommandRunner` parameter rather than calling `os/exec` directly. `CommandRunner` is a func type:

```go
// CommandRunner executes a command in the given directory, writing output to stdout and stderr.
// It is the sole execution primitive for Docker command invocations that capture output.
type CommandRunner func(ctx context.Context, dir string, stdout, stderr io.Writer, name string, args ...string) error
```

A `defaultRunner` wraps `exec.CommandContext`. Tests pass a `fakeRunner` that records each call's dir, name, and args, and returns preset output without shelling out.

This is distinct from the `Backend` interface (ref-2). `CommandRunner` is the *execution primitive* — the seam between application code and `os/exec`. The `Backend` interface will be a *strategy* — the seam between `app.go` and an entire Docker Compose implementation. `CommandRunner` lives inside the Docker implementation; `Backend` wraps it from the outside.

Shell-shim tests (`TestPrepareAgentImageReportsProgress`, `TestEnsurePipelockConfigReconcilesExistingConfig`, and the Dockerfile workflow test) are replaced with `CommandRunner`-based tests that assert the exact Docker arguments passed. This provides the same command-construction coverage without PATH manipulation or filesystem shims.

`runStreaming` (used for PTY operations: `docker exec`, `docker compose logs -f`, `docker compose down`) passes raw file descriptors to the child process and cannot be meaningfully faked. It remains a direct `os/exec` wrapper and is not threaded through `CommandRunner`. Its behaviour is validated by smoke tests, not unit tests.

## Type Reference

```go
// BuildInput carries all pre-resolved inputs for plan construction.
// The caller is responsible for resolving external state before calling Build.
type BuildInput struct {
    WorkspaceRoot string // absolute workspace root path
    WorkspaceSlug string // derived workspace slug
    Profile       string // resolved profile name
    ConfigDir     string // e.g. ~/.config
    DataDir       string // e.g. ~/.local/share
    BaseImageRef  string // published base image reference
    Timezone      string // TZ env value; empty falls back to "UTC"
    HasDockerfile bool   // whether Dockerfile.dune exists at workspace root
}

// EnvironmentPlan is a pure semantic description of the desired Dune environment.
// No field encodes a Docker primitive or backend implementation detail.
type EnvironmentPlan struct {
    Project     ProjectIdentity
    Environment EnvironmentSpec
    Persistence PersistenceSpec
    Egress      EgressSpec
}

// ProjectIdentity captures stable host-side identity and state locations.
type ProjectIdentity struct {
    Root         string // absolute workspace root path
    Slug         string // workspace slug, e.g. "demo-app-96"
    Profile      string // profile name, e.g. "work"
    InstanceName string // user-visible instance id, e.g. "dune-demo-app-96-work"
    DataDir      string // project state directory, e.g. "~/.local/share/dune/projects/demo-app-96"
}

// EnvironmentSpec describes the interactive development environment the user enters.
type EnvironmentSpec struct {
    Image      ImageSpec
    Workspace  WorkspaceMount
    WorkingDir string   // "/workspace"
    Shell      []string // ["zsh"]
    TZ         string   // timezone
}

// ImageSpec describes the image selection and build decision.
type ImageSpec struct {
    BaseRef      string // published base image reference
    RuntimeRef   string // image to use at runtime (base ref or local build ref)
    BuildContext string // workspace root when UseBuild is true
    Dockerfile   string // "Dockerfile.dune" when UseBuild is true
    UseBuild     bool
}

// WorkspaceMount describes the repo mount into the environment.
type WorkspaceMount struct {
    HostPath  string // absolute workspace root on host
    GuestPath string // "/workspace"
    Writable  bool
}

// PersistenceSpec describes preserved profile state across sessions.
type PersistenceSpec struct {
    LogicalName string // e.g. "dune-persist-work"
    MountPath   string // "/persist/agent"
}

// EgressSpec describes the desired egress mediation for the environment.
// It does not encode backend-specific proxy addresses, network names, or container images.
type EgressSpec struct {
    Provider   string // "pipelock"
    ConfigPath string // absolute host path to provider config file
}
```

## Package Shape After This Change

```
internal/dune/
  app.go             ← thin entry point: parse → resolve → plan.Build → delegate
  runner.go          ← CommandRunner type, defaultRunner implementation
  compose.go         ← renderComposeFile, ensureComposeFile, validateComposeFile
  docker.go          ← validateDockerPrerequisites, ensureVolume, prepareAgentImage,
                       composeUp, isAgentRunning, isAgentCreated, capture, runStreaming
  plan/
    plan.go          ← EnvironmentPlan and all supporting types
    builder.go       ← Build(BuildInput) EnvironmentPlan (pure)
    builder_test.go  ← pure planner table tests (no Docker required)
  workspace/         ← unchanged
  cli/               ← unchanged
  pipelock/          ← unchanged
```

## Ref-2 Boundary

This section exists to prevent scope creep. The following are **not** part of ref-1 and must not be introduced here, even partially.

**`ref-2-docker-backend-abstraction` will:**
- Introduce a `Backend` interface that maps `EnvironmentPlan` to execution.
- Move `compose.go`, `docker.go`, and the compose template into `runtime/dockercompose/` as a package implementing `Backend`.
- Refactor `app.go` to call `backend.Execute(plan)` rather than individual Docker functions directly.

**The clean line between ref-1 and ref-2:**
- Ref-1 introduces the *plan* (what to build) and the *execution primitive* (`CommandRunner`). `app.go` calls Docker functions directly after building the plan.
- Ref-2 introduces the *backend strategy* (a `Backend` interface that maps a plan to execution). `app.go` calls a backend; the backend calls Docker functions internally.

An implementing agent that finds itself wanting to introduce a `Backend` interface, a `backend.Execute` call, or a `runtime/dockercompose` package during ref-1 must stop and defer that work to ref-2.
