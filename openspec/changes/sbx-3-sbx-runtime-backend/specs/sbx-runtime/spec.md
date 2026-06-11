## ADDED Requirements

### Requirement: sbx readiness is validated before runtime operations

Before any sandbox lifecycle operation, the CLI SHALL validate host `sbx` readiness: `sbx` is present on `PATH`, the `sbx` daemon is healthy and authenticated (via `sbx diagnose`), and the installed `sbx` version meets the minimum supported version. On failure it SHALL return a clear, actionable error and SHALL NOT attempt the operation. Docker SHALL NOT be required for any `dune` command.

#### Scenario: Missing or unhealthy sbx is reported clearly
- **GIVEN** a host where `sbx` is not installed, not authenticated, or below the minimum version
- **WHEN** a `dune` command that needs the runtime is invoked
- **THEN** it fails with an actionable error identifying the unmet sbx requirement
- **AND** no sandbox operation is attempted

#### Scenario: Docker is not required
- **GIVEN** a host with `sbx` ready but without Docker installed
- **WHEN** a `dune` runtime command is invoked
- **THEN** it does not fail for lack of Docker

---

### Requirement: A Dune instance maps to a sandbox named dune-<slug>-<profile>

The runtime SHALL map a Dune workspace/profile instance to an `sbx` sandbox named `dune-<workspace-slug>-<profile>`. Profile-scoped persisted state (agent credentials/config) SHALL be backed by a durable, profile-scoped location decoupled from any single sandbox, so it is shared across that profile's sandboxes; the remaining sandbox state SHALL persist via the sandbox lifecycle (until removed) rather than a Docker volume.

#### Scenario: Instance name composition
- **GIVEN** workspace slug `demo-app-96` and profile `work`
- **WHEN** the runtime resolves the instance
- **THEN** the sandbox name is `dune-demo-app-96-work`

#### Scenario: Default profile
- **GIVEN** workspace slug `demo-app-96` and profile `default`
- **WHEN** the runtime resolves the instance
- **THEN** the sandbox name is `dune-demo-app-96-default`

#### Scenario: Profile-scoped state is shared across the profile's sandboxes
- **GIVEN** two workspaces realised as separate sandboxes under the same profile
- **WHEN** profile-scoped state (agent credentials/config) is written from one sandbox
- **THEN** the same persisted state is visible to the other sandbox via the profile's durable persist location

---

### Requirement: The workspace is direct-mounted and the shell attaches at the mounted repo path

The runtime SHALL create the sandbox from the Dune sbx template with the workspace direct-mounted, and SHALL attach the interactive shell with its working directory set to the real mounted repository path (not the sandbox default home). It SHALL supply the mounted path to the template so `/workspace` resolves to the repository.

#### Scenario: Shell starts in the repository
- **GIVEN** a sandbox created for a workspace rooted at an absolute host path
- **WHEN** the runtime attaches the shell
- **THEN** the shell's working directory is the mounted repository path
- **AND** `git rev-parse --show-toplevel` from that directory succeeds

---

### Requirement: up creates, starts, and attaches; reusing an existing sandbox

The default/`up` flow SHALL validate readiness, ensure the sandbox exists (creating it from the Dune template when missing), start it when stopped, and attach a shell. An already-running sandbox SHALL be attached without recreation.

#### Scenario: First run creates and attaches
- **GIVEN** no sandbox exists for the instance
- **WHEN** `dune up` runs
- **THEN** the sandbox is created from the Dune template, started, and a shell is attached

#### Scenario: Existing running sandbox is reused
- **GIVEN** the instance's sandbox already exists and is running
- **WHEN** `dune up` runs
- **THEN** the existing sandbox is attached without being recreated

---

### Requirement: down stops the sandbox and rebuild recreates from the template

`down` SHALL stop the instance's sandbox while retaining its state. `rebuild` SHALL recreate the sandbox from the (current) Dune sbx template without discarding profile-scoped persisted state (agent credentials/config). There SHALL be no `Dockerfile.dune` build path.

#### Scenario: down stops without destroying state
- **GIVEN** a running sandbox for the instance
- **WHEN** `dune down` runs
- **THEN** the sandbox is stopped and its persisted state is retained

#### Scenario: rebuild does not build a local Dockerfile
- **GIVEN** a workspace that previously relied on `Dockerfile.dune`
- **WHEN** `dune rebuild` runs
- **THEN** the sandbox is recreated from the Dune sbx template
- **AND** no `Dockerfile.dune` image build is performed

#### Scenario: rebuild preserves profile-scoped persisted state
- **GIVEN** a sandbox whose profile has persisted agent credentials/config under the durable `/persist/agent` mapping
- **WHEN** `dune rebuild` removes and recreates the sandbox from the template
- **THEN** a sentinel previously written under `/persist/agent` is still present in the recreated sandbox
- **AND** the previously persisted profile state is still available in the recreated sandbox

---

### Requirement: Docker Compose execution is removed from the runtime

The runtime SHALL NOT render or execute Docker Compose, create Docker volumes, pull/build agent images, or start a Pipelock sidecar. The Docker-shaped project model and compose template SHALL be removed from the active runtime path.

#### Scenario: No Docker Compose or Pipelock invocation on up
- **GIVEN** a host with `sbx` ready
- **WHEN** `dune up` runs
- **THEN** no `docker compose` command is invoked
- **AND** no Pipelock sidecar is started and no `pipelock.yaml` is generated

---

### Requirement: sbx command construction is testable without an sbx daemon

The runtime SHALL route command execution through a runner seam so that command construction and lifecycle sequencing can be asserted in unit tests without a running `sbx` daemon, using a fake runner that records invocations and returns preset output.

#### Scenario: up command sequence is asserted with a fake runner
- **GIVEN** a fake runner injected into the sbx backend
- **WHEN** the `up` flow runs against the fake runner
- **THEN** the recorded invocations show the expected `sbx` subcommands, the instance name, and the mounted workspace path, in the expected order
