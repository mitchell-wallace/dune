## Context

`sbx-3-sbx-runtime-backend` moves execution onto an `internal/dune/runtime/sbx` backend with lifecycle operations (`Validate`, `Ensure`, `Start`, `Shell`, `Stop`, `Status`) behind a command-runner seam, and wires `up/down/rebuild/logs` as interim mappings. `sbx-4-sbx-network-and-secrets` establishes the egress baseline, sandbox-scoped rules, and `sbx policy log` access, and removes Pipelock. This change finalises the user-facing command surface on top of those, adds structured diagnostics for sbx failures, and adds `dune doctor`.

Two superseded `ref-*` drafts contribute reusable primitives (Docker-shaped, retargeted here to sbx): `ref-3-improve-runtime-diagnostics` (a `DiagnosticError` with code/summary/stderr/recovery and `WrapCommandError`) and `ref-4-add-dune-doctor` (a `Check` model with pass/warn/fail/skip and concise + `--json` output).

Per the `sbx-2` service de-emphasis and spike 3, `dune logs` centers on Dune-owned + `sbx policy log` output rather than app-service logs (project app dependencies are project-owned Compose).

## Goals / Non-Goals

**Goals**
- Finalise `dune up/down/destroy/rebuild/logs` (and optionally `ports`) on the sbx backend.
- Compose `dune logs` from Dune-owned setup/runtime logs + `sbx policy log`.
- Add a small, sbx-scoped structured-diagnostics type with stable codes, preserved stderr, recovery hints; assert codes in tests.
- Add a read-only `dune doctor` that does not start/enter the environment, with concise + optional JSON output.

**Non-Goals**
- Template/service conventions (`sbx-2`), backend ops (`sbx-3`), egress plumbing (`sbx-4`), kits/cleanup (`sbx-6`).
- A repair mode (`--fix`).
- Forcing expensive checks (e.g. full template pulls) by default.

## Decisions

### D1: Finalised command mapping
Map the `dune` verbs onto the `sbx-3` backend operations:
- `dune` / `dune up`: `Validate` → ensure template available → `Ensure` (create if missing) → `Start` (if stopped) → `Shell` at the mounted repo path. An already-running sandbox is attached without recreation.
- `dune down`: `Stop` (state retained).
- `dune destroy`: `sbx rm <instance>` (sandbox removed). Profile-scoped persisted state survives via the durable persist location (`sbx-3` D3); destroy removes the sandbox, not the profile's persisted credentials/config. Requires confirmation (or `--force`).
- `dune rebuild`: recreate from the (current) template, preserving profile-scoped persisted state; no `Dockerfile.dune` build.
- `dune ports` (optional): wrap `sbx ports` list/publish/unpublish, surfacing the loopback-vs-all-interfaces caveat (spike 2: a nested service bound only to sandbox loopback was not reachable via published host port; binding to all sandbox interfaces worked).

### D2: `dune destroy` is added; resolves the sbx-3 deferred question
`sbx-3` deferred whether a removal command exists. Because the durable persist location (`sbx-3` D3) decouples profile state from the sandbox, removing a sandbox is now safe for profile data, so `dune destroy` is added as the first-class removal verb (`sbx rm`). `down` remains stop-only.

### D3: `dune logs` composition
`dune logs` composes:
- Dune-owned setup/runtime logs (e.g. sandbox create/start/attach diagnostics and any Dune-managed in-sandbox setup output), from a documented location/command established conceptually in `sbx-2`.
- `sbx policy log <instance>` for egress observability (from `sbx-4`), replacing `dune logs pipelock`.
App-dependency service logs are intentionally **not** aggregated here; users read those via `docker compose logs` inside the sandbox for project-owned services. In-template service logs may be surfaced opportunistically but are best-effort. There is no single `sbx logs` command to wrap, so `dune logs` defines its own composition.

### D4: Structured diagnostics (sbx-scoped, small)
Introduce a small diagnostic-error type reusing the `ref-3` shape, retargeted to sbx:

```go
type DiagnosticError struct {
    Code     string
    Summary  string
    Detail   string
    Cause    error
    Command  []string
    Stderr   string
    Recovery []string
}
func IsDiagnostic(err error) bool
func AsDiagnostic(err error) (*DiagnosticError, bool)
func WrapCommandError(code, summary string, result CommandResult, err error) error
```

Initial, deliberately small sbx code set (extend only when Dune can actually distinguish a case):

```text
sbx.not_installed
sbx.diagnose_failed        # daemon unhealthy / auth / readiness from `sbx diagnose`
sbx.version_below_min
sbx.create_failed
sbx.start_failed
sbx.stop_failed
sbx.exec_failed            # shell/attach
sbx.rm_failed
template.unavailable       # template ref cannot be pulled/loaded
policy.apply_failed        # applying an egress rule failed
workspace.invalid
profile.config_corrupt
```

The backend maps failures to codes at the boundary where context is known; stderr is preserved (not replaced by friendly summaries); recovery hints are attached for common host/setup failures. Default CLI output stays concise; verbose mode shows command/stderr. Tests assert codes and key fields, not rendered prose.

### D5: `dune doctor` (read-only)
Add `dune doctor` (+ `--verbose`, optional `--json`) that inspects readiness **without** starting or entering the environment. It reuses the `ref-4` `Check` model:

```go
type Check struct {
    ID, Group, Severity, Status, Summary, Detail string
    Recovery []string
}
// Status: pass | warn | fail | skip
```

Check groups (sbx-targeted):
- **host/sbx**: `sbx` present on PATH; `sbx diagnose` passes (daemon/auth/readiness); version ≥ minimum.
- **template**: Dune sbx template reference is known and appears available (lightweight check; no forced full pull by default — a deep pull is opt-in).
- **sandbox**: instance status via `sbx ls --json` (exists / running / stopped) — read-only.
- **workspace/profile/config**: workspace root resolves and slug computes; `profiles.json` parses (or is safely absent); effective profile resolves and name is valid; config/data/persist dirs are readable/writable or creatable.
- **egress**: the active policy posture is non-`Open` and inspectable (from `sbx-4`); reported as `warn` (not `fail`) if it cannot be confirmed.
- **services (optional, non-fatal)**: in-template service health if cheaply checkable; always `skip`/`warn` rather than `fail`, reflecting the `sbx-2` de-emphasis.

Doctor reuses the D4 diagnostic codes/recovery hints where a check corresponds to a runtime failure mode. It reports readiness; it does not guarantee the environment works (smoke tests remain necessary).

## Risks / Trade-offs

- **Doctor scope/cost.** Template availability and pull checks can be expensive; keep them lightweight by default, deep checks opt-in.
- **Over-built taxonomy.** Diagnostic codes can sprawl; keep the set small and add only distinguishable cases.
- **`dune logs` expectations.** Users migrating from `dune logs <service>` may expect app-service logs; document that app deps are project-owned Compose and surfaced via `docker compose logs` in the sandbox.
- **Brittle output tests.** Assert codes/statuses and key fields, not rendered prose.
- **`dune ports` bind caveat.** Loopback-only nested services are not publishable; surface guidance rather than silently failing.

## Migration Plan

1. Land after `sbx-3` (backend ops) and `sbx-4` (egress baseline + policy-log access; Pipelock removed).
2. Finalise the command mapping (D1) and add `dune destroy` (D2); recompose `dune logs` (D3).
3. Add the diagnostics type and codes (D4); convert backend failures to coded diagnostics; add code-asserting tests.
4. Add `dune doctor` (D5) reusing backend readiness + diagnostics; add pass/warn/fail tests.
5. `sbx-6` layers kits/cleanup on top.

## Open Questions

- Whether `dune ports` ships in this change or is deferred.
- Exact location/command for Dune-owned setup/runtime logs that `dune logs` reads (coordinate with `sbx-2` conventions).
- Whether `dune doctor` gets an opt-in deep mode (e.g. `--deep`) for template pull/availability.
- Final confirmation UX for `dune destroy` (prompt vs `--force`).
