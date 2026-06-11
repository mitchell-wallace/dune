## ADDED Requirements

### Requirement: dune doctor reports readiness without starting the environment

`dune doctor` SHALL inspect readiness without creating, starting, or entering a sandbox. It SHALL report structured checks across host/sbx readiness (`sbx` on PATH, `sbx diagnose`, minimum version), Dune sbx template availability, sandbox status, workspace/profile/config/persist readiness, and the egress policy baseline, each with a status of `pass`, `warn`, `fail`, or `skip`. It SHALL NOT probe in-template service health.

#### Scenario: doctor does not start the environment
- **GIVEN** an instance whose sandbox is stopped or absent
- **WHEN** `dune doctor` runs
- **THEN** it reports readiness checks
- **AND** it invokes only read-only `sbx` commands (such as `sbx diagnose`, `sbx version`, `sbx ls`, `sbx policy ls`)
- **AND** it does not invoke `sbx create`, `sbx run`, `sbx exec`, or `sbx rm`

#### Scenario: failing checks include recovery hints
- **GIVEN** a host where `sbx diagnose` fails
- **WHEN** `dune doctor` runs
- **THEN** the host/sbx check reports `fail`
- **AND** it includes an actionable recovery hint

#### Scenario: unconfirmable egress posture warns rather than fails
- **GIVEN** an instance whose egress posture cannot be confirmed via `sbx policy ls`
- **WHEN** `dune doctor` runs
- **THEN** the egress check reports `warn`, not `fail`

---

### Requirement: dune doctor output is concise with an optional machine-readable mode

`dune doctor` SHALL produce concise human-readable output by default and SHALL support an optional machine-readable (`--json`) mode emitting the structured check list. The checks SHALL focus on the core sbx backing and exclude in-template service health.

#### Scenario: JSON mode emits structured checks
- **GIVEN** `dune doctor --json`
- **WHEN** it runs
- **THEN** it emits the structured list of checks with their statuses
