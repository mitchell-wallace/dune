## ADDED Requirements

### Requirement: EnvironmentPlan is a pure semantic type

`plan.EnvironmentPlan` SHALL be a pure Go struct with no methods that perform I/O, call external processes, or access the filesystem. All fields SHALL describe what the Dune environment should be. No field SHALL encode a Docker primitive, a backend container image reference, a proxy network address, or a Docker volume type.

#### Scenario: Plan carries no Docker-specific values
- **GIVEN** an `EnvironmentPlan` built from any valid `BuildInput`
- **THEN** the plan contains no field whose value encodes `docker`, `compose`, a sidecar network address, or a Docker volume declaration

---

### Requirement: Build is a pure function

`plan.Build(BuildInput) EnvironmentPlan` SHALL be a pure function. It SHALL NOT import or call `os/exec`, `os.ReadFile`, `os.Stat`, `os.MkdirAll`, or any equivalent that accesses the filesystem or runs external processes. All external state required to construct the plan SHALL be resolved by the caller and passed through `BuildInput`.

#### Scenario: Build does not require Docker
- **GIVEN** any valid `BuildInput`
- **WHEN** `Build` is called on a machine where Docker is not installed
- **THEN** it returns a valid `EnvironmentPlan` without error

---

### Requirement: InstanceName is derived from slug and profile

`ProjectIdentity.InstanceName` SHALL be `dune-<slug>-<profile>`. It is the user-visible identifier for the workspace/profile combination and is treated as an externally observable compatibility surface. It must not change in format without a documented migration.

#### Scenario: Default profile
- **GIVEN** slug `demo-app-96` and profile `default`
- **WHEN** `Build` is called
- **THEN** `Project.InstanceName` is `"dune-demo-app-96-default"`

#### Scenario: Named profile
- **GIVEN** slug `demo-app-96` and profile `work`
- **WHEN** `Build` is called
- **THEN** `Project.InstanceName` is `"dune-demo-app-96-work"`

---

### Requirement: DataDir is derived from base data directory and slug

`ProjectIdentity.DataDir` SHALL be `<DataDir>/dune/projects/<slug>`, where `DataDir` is the `BuildInput.DataDir` field (typically `~/.local/share`).

#### Scenario: DataDir derivation
- **GIVEN** `BuildInput.DataDir = "/home/agent/.local/share"` and slug `demo-app-96`
- **WHEN** `Build` is called
- **THEN** `Project.DataDir` is `"/home/agent/.local/share/dune/projects/demo-app-96"`

---

### Requirement: Persistence logical name is derived from profile

`PersistenceSpec.LogicalName` SHALL be `dune-persist-<profile>`. This name is treated as an externally observable compatibility surface. Changing it would orphan existing user data. It must not change format without a documented migration.

`PersistenceSpec.MountPath` SHALL be `"/persist/agent"`.

#### Scenario: Work profile persistence
- **GIVEN** profile `work`
- **WHEN** `Build` is called
- **THEN** `Persistence.LogicalName` is `"dune-persist-work"`
- **THEN** `Persistence.MountPath` is `"/persist/agent"`

#### Scenario: Default profile persistence
- **GIVEN** profile `default`
- **WHEN** `Build` is called
- **THEN** `Persistence.LogicalName` is `"dune-persist-default"`

---

### Requirement: Image selection reflects Dockerfile.dune presence

When `BuildInput.HasDockerfile` is true:
- `Environment.Image.UseBuild` SHALL be true
- `Environment.Image.RuntimeRef` SHALL be `"dune-local-<slug>:latest"`
- `Environment.Image.BuildContext` SHALL be the workspace root
- `Environment.Image.Dockerfile` SHALL be `"Dockerfile.dune"`

When `BuildInput.HasDockerfile` is false:
- `Environment.Image.UseBuild` SHALL be false
- `Environment.Image.RuntimeRef` SHALL equal `Environment.Image.BaseRef`

In both cases, `Environment.Image.BaseRef` SHALL equal `BuildInput.BaseImageRef`.

#### Scenario: Workspace with Dockerfile.dune
- **GIVEN** `HasDockerfile: true`, slug `demo-app-96`, workspace root `/workspace/demo-app`
- **WHEN** `Build` is called
- **THEN** `Environment.Image.UseBuild` is `true`
- **THEN** `Environment.Image.RuntimeRef` is `"dune-local-demo-app-96:latest"`
- **THEN** `Environment.Image.BuildContext` is `"/workspace/demo-app"`
- **THEN** `Environment.Image.Dockerfile` is `"Dockerfile.dune"`

#### Scenario: Workspace without Dockerfile.dune
- **GIVEN** `HasDockerfile: false`, base image ref `"ghcr.io/mitchell-wallace/dune-base:0.2.3"`
- **WHEN** `Build` is called
- **THEN** `Environment.Image.UseBuild` is `false`
- **THEN** `Environment.Image.RuntimeRef` is `"ghcr.io/mitchell-wallace/dune-base:0.2.3"`

---

### Requirement: Workspace mount targets /workspace

`Environment.Workspace.HostPath` SHALL equal the workspace root. `Environment.Workspace.GuestPath` SHALL be `"/workspace"`. `Environment.Workspace.Writable` SHALL be `true`.

#### Scenario: Workspace mount
- **GIVEN** workspace root `/workspace/my-project`
- **WHEN** `Build` is called
- **THEN** `Environment.Workspace.HostPath` is `"/workspace/my-project"`
- **THEN** `Environment.Workspace.GuestPath` is `"/workspace"`
- **THEN** `Environment.Workspace.Writable` is `true`

---

### Requirement: Default shell is zsh

`Environment.Shell` SHALL be `["zsh"]`.

#### Scenario: Shell is always zsh
- **GIVEN** any valid `BuildInput`
- **WHEN** `Build` is called
- **THEN** `Environment.Shell` is `["zsh"]`

---

### Requirement: WorkingDir is /workspace

`Environment.WorkingDir` SHALL be `"/workspace"`.

---

### Requirement: Timezone falls back to UTC

`Environment.TZ` SHALL be set to `BuildInput.Timezone` when non-empty, and `"UTC"` when empty.

#### Scenario: Timezone from input
- **GIVEN** `Timezone: "Australia/Melbourne"`
- **WHEN** `Build` is called
- **THEN** `Environment.TZ` is `"Australia/Melbourne"`

#### Scenario: Timezone fallback
- **GIVEN** `Timezone: ""`
- **WHEN** `Build` is called
- **THEN** `Environment.TZ` is `"UTC"`

---

### Requirement: EgressSpec carries intent, not implementation

`EgressSpec.Provider` SHALL be `"pipelock"`. `EgressSpec.ConfigPath` SHALL be `<ConfigDir>/dune/pipelock.yaml` where `ConfigDir` is `BuildInput.ConfigDir`.

`EgressSpec` SHALL NOT include:
- proxy environment variable values (e.g. `http://pipelock:8888`)
- Docker image references for the egress sidecar
- any network address specific to a Docker topology

#### Scenario: Pipelock config path derivation
- **GIVEN** `BuildInput.ConfigDir = "/home/agent/.config"`
- **WHEN** `Build` is called
- **THEN** `Egress.Provider` is `"pipelock"`
- **THEN** `Egress.ConfigPath` is `"/home/agent/.config/dune/pipelock.yaml"`

---

### Requirement: Planner tests run without Docker

All tests in `internal/dune/plan/` SHALL pass on a machine where Docker is not installed or the Docker daemon is not running. No test in this package SHALL skip based on Docker availability.

#### Scenario: All planning cases are covered by pure tests
- **GIVEN** a machine without Docker
- **WHEN** `go test ./internal/dune/plan/...` is run
- **THEN** all tests pass

Required test cases:
- default profile plan
- explicit profile plan
- named stored profile plan
- workspace with `Dockerfile.dune` (HasDockerfile: true)
- workspace without `Dockerfile.dune` (HasDockerfile: false)
- timezone from input
- timezone fallback to UTC
- slug and InstanceName composition
- DataDir derivation
- Persistence.LogicalName derivation
- Egress.ConfigPath derivation
- workspace mount shape
- shell and working dir
