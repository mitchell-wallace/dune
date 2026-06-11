## ADDED Requirements

### Requirement: The dune command surface is finalised on the sbx lifecycle

Dune SHALL map its commands onto the sbx backend operations: `dune`/`dune up` validates readiness, ensures the template is available, creates the sandbox if missing, starts it if stopped, and attaches a shell at the mounted repo path; `dune down` stops the sandbox while retaining state; `dune rebuild` recreates the sandbox from the current template (no `Dockerfile.dune`) while preserving profile-scoped persisted state.

#### Scenario: up attaches, reusing a running sandbox
- **GIVEN** an instance whose sandbox is already running
- **WHEN** `dune up` runs
- **THEN** the existing sandbox is attached without being recreated

#### Scenario: down retains state
- **GIVEN** a running sandbox
- **WHEN** `dune down` runs
- **THEN** the sandbox is stopped and its state is retained

---

### Requirement: dune destroy removes the sandbox while preserving profile state

Dune SHALL provide a `dune destroy` command that removes the instance's sandbox (`sbx rm`). It SHALL require confirmation or an explicit force flag, and SHALL NOT delete the profile-scoped persisted state (agent credentials/config), which lives in the durable persist location.

#### Scenario: destroy removes the sandbox but keeps profile credentials
- **GIVEN** an instance with profile-scoped persisted credentials/config
- **WHEN** `dune destroy` is confirmed
- **THEN** the sandbox is removed
- **AND** the profile-scoped persisted state remains available to a subsequent sandbox of that profile

---

### Requirement: dune logs composes Dune-owned logs and the sbx policy log

`dune logs` SHALL compose Dune-owned setup/runtime logs with `sbx policy log <instance>` for egress observability. It SHALL NOT provide a `dune logs pipelock` surface. App-dependency service logs are not aggregated by `dune logs`; they are obtained from the project-owned Compose project inside the sandbox.

#### Scenario: logs include egress records and exclude pipelock
- **GIVEN** a running instance
- **WHEN** `dune logs` runs
- **THEN** it surfaces Dune-owned logs and `sbx policy log` records
- **AND** there is no `dune logs pipelock` subcommand

---

### Requirement: dune ports guidance accounts for bind address

Dune SHALL provide a `dune ports` command that wraps `sbx ports` list/publish/unpublish and SHALL surface that a nested service bound only to sandbox loopback may not be reachable through published host ports, so dev servers should bind to all sandbox interfaces when host exposure is desired.

#### Scenario: publishing a loopback-only service is guided
- **GIVEN** a nested service bound only to sandbox loopback
- **WHEN** the user attempts to publish it via `dune ports`
- **THEN** Dune surfaces guidance to bind the service to all sandbox interfaces for host reachability
