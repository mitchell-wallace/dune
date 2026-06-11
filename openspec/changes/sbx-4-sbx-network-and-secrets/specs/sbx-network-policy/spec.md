## ADDED Requirements

### Requirement: Dune workspaces have a defined, non-Open egress baseline

Dune SHALL use a defined, non-`Open` egress baseline for sbx workspaces, starting from `sbx`'s `Balanced` preset (default-deny plus a developer-infrastructure allowlist). Dune SHALL NOT silently weaken the egress posture to `Open`, SHALL NOT mutate the user's global `sbx` default policy/profile, and SHALL be able to surface the active posture for an instance rather than assuming it. If a non-`Open` instance posture cannot be confirmed, Dune SHALL fail or warn closed rather than proceed silently under an open posture.

#### Scenario: Egress is not silently opened
- **GIVEN** a Dune sbx workspace
- **WHEN** the runtime prepares or enters the sandbox
- **THEN** it does not set the egress posture to `Open` on the user's behalf
- **AND** the active posture can be surfaced (e.g. via `sbx policy ls` / `sbx policy log`)
- **AND** Dune does not mutate the user's global `sbx` default policy/profile

#### Scenario: A blocked-by-default domain is denied under the baseline
- **GIVEN** an instance under the non-`Open` baseline
- **WHEN** the sandbox requests a domain not in the baseline allowlist (for example an arbitrary documentation site)
- **THEN** the request is denied
- **AND** the denial is visible in `sbx policy log <instance>`

---

### Requirement: Dune-managed egress rules are scoped to the workspace's sandbox

When Dune applies egress rules itself, it SHALL scope them to the workspace's sandbox (`--sandbox <instance>`) so they do not modify the user's global `sbx` default policy or profile. The constructed `sbx` command (name and argument shape) SHALL be assertable without a live `sbx` daemon.

#### Scenario: Opening a domain does not change global policy
- **GIVEN** a workspace instance and the user's existing global `sbx` policy
- **WHEN** Dune applies an egress rule for the workspace
- **THEN** the rule is scoped to that instance's sandbox (`--sandbox <instance>`)
- **AND** the user's global default policy/profile is unchanged

#### Scenario: Rule construction is verifiable without a live daemon
- **GIVEN** Dune's egress-rule construction for a workspace
- **WHEN** an allow, deny, or remove rule is built for the sandbox
- **THEN** the exact command name and arguments are observable through the command-runner seam
- **AND** they can be asserted in tests without invoking `sbx`

---

### Requirement: Project-specific domains can be opened with correct rule shape

Dune SHALL provide an affordance and/or guidance to open project-specific domains that adds both the exact domain and a specific wildcard when both are needed, prefers exact and specific-wildcard rules over broad catch-alls, and takes effect immediately on the running sandbox.

#### Scenario: Opening exact and wildcard forms
- **GIVEN** a sandbox under the baseline where `example.org` is blocked
- **WHEN** the exact domain `example.org` and the wildcard `*.example.org` are opened for the sandbox
- **THEN** both `https://example.org/` and `https://www.example.org/` are reachable
- **AND** the change takes effect without recreating the sandbox

#### Scenario: Exact-only open does not cover subdomains
- **GIVEN** a sandbox where only the exact domain `example.org` has been opened
- **WHEN** a subdomain such as `www.example.org` is requested
- **THEN** the subdomain request remains blocked until a matching wildcard rule is added

---

### Requirement: Egress is observable via sbx policy log

Dune SHALL surface egress observability through `sbx policy log <instance>`, covering both direct shell traffic and nested Docker traffic, as the replacement for `dune logs pipelock`.

#### Scenario: Blocked and allowed requests are recorded
- **GIVEN** an instance with egress rules applied
- **WHEN** the shell and a nested Docker container make outbound requests
- **THEN** `sbx policy log <instance>` records the blocked and allowed requests for both traffic paths

---

### Requirement: Pipelock is retired; egress is governed solely by sbx policy

For the sbx backend, egress mediation SHALL be provided solely by the `sbx` network policy layer. The Pipelock sidecar, the generated `~/.config/dune/pipelock.yaml`, the proxy-env (`HTTP(S)_PROXY`) model, and the `dune logs pipelock` surface SHALL be removed, and SHALL only be removed once the `sbx` egress baseline governs the sandbox so egress is never left unmediated.

#### Scenario: No Pipelock in the sbx egress path
- **GIVEN** a Dune sbx workspace
- **WHEN** the workspace is started and used
- **THEN** no Pipelock sidecar is started and no `pipelock.yaml` is generated
- **AND** no proxy-env (`HTTP(S)_PROXY`) values are injected for egress
- **AND** outbound traffic is still mediated by `sbx` policy
