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
- **THEN** `dune` invokes `sbx rm <instance>` and the sandbox no longer appears in `sbx ls --json`
- **AND** the profile-scoped persisted state remains available to a subsequent sandbox of that profile

#### Scenario: destroy requires confirmation or an explicit force flag
- **GIVEN** an existing sandbox
- **WHEN** `dune destroy` runs without confirmation and without `--force`
- **THEN** the sandbox is not removed

---

### Requirement: dune logs composes Dune-owned logs and the sbx policy log

`dune logs` SHALL compose Dune-owned setup/runtime logs — the host-side lifecycle log plus the in-sandbox Dune output written under `/var/log/dune/` (the `sbx-2` D5a contract) — with `sbx policy log <instance>` for egress observability. It SHALL NOT provide a `dune logs pipelock` surface. App-dependency service logs are not aggregated by `dune logs`; they are obtained from the project-owned Compose project inside the sandbox.

#### Scenario: logs include egress records and exclude pipelock
- **GIVEN** a running instance
- **WHEN** `dune logs` runs
- **THEN** it surfaces a Dune-owned log line (host-side and/or the in-sandbox `/var/log/dune/` output) and records from `sbx policy log <instance>`
- **AND** there is no `dune logs pipelock` subcommand

---

### Requirement: dune ports guidance accounts for bind address

Dune SHALL provide a `dune ports` command over the `sbx ports` surface (listing via `sbx ports <instance>` and publishing via `sbx ports <instance> --publish <port>`) and SHALL surface that a nested service bound only to sandbox loopback may not be reachable through published host ports, so dev servers should bind to all sandbox interfaces when host exposure is desired. Any unpublish/remove affordance SHALL be provided only with an `sbx` port-removal spelling confirmed against the installed `sbx`.

#### Scenario: listing uses the verified sbx ports shape
- **GIVEN** a running instance
- **WHEN** `dune ports` lists ports
- **THEN** Dune invokes `sbx ports <instance>`

#### Scenario: publishing a loopback-only service is guided
- **GIVEN** a nested service bound only to sandbox loopback
- **WHEN** the user attempts to publish it via `dune ports`
- **THEN** Dune surfaces guidance to bind the service to all sandbox interfaces for host reachability
