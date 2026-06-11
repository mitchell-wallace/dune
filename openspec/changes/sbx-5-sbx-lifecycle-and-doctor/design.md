## Context

`sbx-3-sbx-runtime-backend` moves execution onto an `internal/dune/runtime/sbx` backend with lifecycle operations (`Validate`, `Ensure`, `Start`, `Shell`, `Stop`, `Status`) behind a command-runner seam, and wires `up/down/rebuild/logs` as interim mappings. `sbx-4-sbx-network-and-secrets` establishes the egress baseline, sandbox-scoped rules, and `sbx policy log` access, and removes Pipelock. This change finalises the user-facing command surface on top of those, adds structured diagnostics for sbx failures, and adds `dune doctor`.

Two superseded `ref-*` drafts contribute reusable primitives (Docker-shaped, retargeted here to sbx): `ref-3-improve-runtime-diagnostics` (a `DiagnosticError` with code/summary/stderr/recovery and `WrapCommandError`) and `ref-4-add-dune-doctor` (a `Check` model with pass/warn/fail/skip and concise + `--json` output).

Per the `sbx-2` service de-emphasis and spike 3, `dune logs` centers on Dune-owned + `sbx policy log` output rather than app-service logs (project app dependencies are project-owned Compose).

## Goals / Non-Goals

**Goals**
- Finalise `dune up/down/destroy/rebuild/logs/ports` on the sbx backend.
- Compose `dune logs` from Dune-owned setup/runtime logs + `sbx policy log`.
- Add a small, sbx-scoped structured-diagnostics type with stable codes, preserved stderr, recovery hints; assert codes in tests.
- Add a read-only `dune doctor` that does not start/enter the environment, with concise + optional JSON output.

**Non-Goals**
- Template/service conventions (`sbx-2`), backend ops (`sbx-3`), egress plumbing (`sbx-4`), kits/cleanup (`sbx-6`).
- A repair mode (`--fix`).
- Forcing expensive checks (e.g. full template pulls) by default.

## Decisions

### D1: Finalised command mapping
Map the `dune` verbs onto the `sbx-3` backend operations. These are user-facing verbs over the `sbx-3` D1 backend; the verbs do not call `sbx` primitives directly except where this change adds a new wrapper (`dune ports`), which goes through the `sbx-3` D5 runner seam so its argument construction is fakeRunner-assertable.
- `dune` / `dune up`: `Validate` → ensure template available → `Ensure` (create if missing) → `Start` (if stopped) → `Shell` at the mounted repo path. An already-running sandbox is attached without recreation.
- `dune down`: `Stop` (state retained).
- `dune destroy`: `sbx rm <instance>` (sandbox removed). Profile-scoped persisted state survives via the durable persist location (`sbx-3` D3); destroy removes the sandbox, not the profile's persisted credentials/config. Requires confirmation (or `--force`).
- `dune rebuild`: recreate from the (current) template, preserving profile-scoped persisted state; no `Dockerfile.dune` build.
- `dune ports`: wrap the `sbx ports` surface, surfacing the loopback-vs-all-interfaces caveat (spike 2: a nested service bound only to sandbox loopback was not reachable via published host port; binding to all sandbox interfaces worked).

**Verified vs. unverified sbx spellings (pin all in fakeRunner; reconfirm via `sbx <verb> --help` before relying on any unverified shape).** sbx-5 inherits the sbx command shapes the dependency plans pinned and MUST NOT silently assume new ones:
- Verified by the spikes: `sbx ls --json` (status, `sbx-3` D7), `sbx stop <name>`, `sbx rm <name>...` (accepts multiple), `sbx ports <sandbox>` (list) and `sbx ports <sandbox> --publish <port> [--json]`, `sbx policy log <instance> --limit <n>` (`sbx-4` D4).
- Note the JSON flag is **not uniform** (`sbx-3` D6): `sbx diagnose --output json` vs. `sbx ls --json` vs. `sbx ports ... --json`. Use each command's verified form; do not assume `sbx diagnose --json`.
- **Unverified and gated on `sbx ports --help` before relied on:** any port *unpublish/remove* subcommand. The spikes only exercised list and `--publish`; no unpublish form was confirmed. `dune ports` exposes list + publish from the verified shapes and MUST confirm the unpublish spelling (or omit unpublish until confirmed) rather than assuming `sbx ports --unpublish`.
- **Unverified and gated on `sbx run --help` (inherited from `sbx-3` D7):** the `Start` invocation underlying `dune up`/`rebuild`. sbx-5 does not re-resolve this; it relies on the shape `sbx-3` records and pins.

### D2: `dune destroy` is added; resolves the sbx-3 deferred question
`sbx-3` deferred whether a removal command exists. Because the durable persist location (`sbx-3` D3) decouples profile state from the sandbox, removing a sandbox is now safe for profile data, so `dune destroy` is added as the first-class removal verb (`sbx rm`). `down` remains stop-only.

### D3: `dune logs` composition
`dune logs` composes:
- Dune-owned setup/runtime logs from two pinned sources: the host-side log that the lifecycle commands already write (sandbox create/start/attach diagnostics emitted via the `sbx-3` runner seam), plus the in-sandbox Dune setup/runtime output that `sbx-2` D5a writes to `/var/log/dune/` (e.g. the re-homed `setup-persist` boot hook's `setup-persist.log`), read via `sbx exec`. Both sources are recorded and asserted in tests (their presence in the composed output), so "what `dune logs` reads" is a verification point rather than a standing open question.
- `sbx policy log <instance> --limit <n>` for egress observability (from `sbx-4` D4), replacing `dune logs pipelock`. Whether a `--json` form exists is unverified per `sbx-4` D4; if absent, `dune logs` passes the raw policy-log output through and does not parse fields it cannot rely on.
App-dependency service logs are intentionally **not** aggregated here; users read those via `docker compose logs` inside the sandbox for project-owned services. There are no in-container app services to aggregate (`sbx-2` removed Postgres/Redis/Mailpit). There is no single `sbx logs` command to wrap (spike 2), so `dune logs` defines its own composition; the absence of `sbx logs` is itself a confirmed point, not an assumption.

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

Check groups (sbx-targeted), each naming the read-only `sbx` command it relies on (pinned in fakeRunner; reconfirm via `sbx <verb> --help`):
- **host/sbx**: `sbx` present on PATH; `sbx diagnose --output json` passes (daemon/auth/readiness — use the `--output json` form from `sbx-3` D6, not `--json`); version ≥ minimum via `sbx version` (candidate `v0.32.0`, the version `sbx-3` pins).
- **template**: Dune sbx template reference (`version.SbxTemplateRef()`, `sbx-3` D10) is known and appears available (lightweight check; no forced full pull by default — a deep pull is opt-in, see below).
- **sandbox**: instance status via `sbx ls --json` (exists / running / stopped) — read-only; this is the same parse `sbx-3` D7 `Status` uses, so doctor reuses that path rather than introducing a second `sbx ls` parser.
- **workspace/profile/config**: workspace root resolves and slug computes; `profiles.json` parses (or is safely absent); effective profile resolves and name is valid; config/data/persist dirs are readable/writable or creatable.
- **egress**: the active policy posture is non-`Open` and inspectable via `sbx policy ls` (the inspection source `sbx-4` D4 exposes). Per `sbx-4` D1's read-only contract, doctor reports `warn` (not `fail`) when the posture cannot be confirmed, and `fail` only when it positively observes an `Open` posture; it never mutates policy.

Doctor reuses the D4 diagnostic codes/recovery hints where a check corresponds to a runtime failure mode. It reports readiness; it does not guarantee the environment works (smoke tests remain necessary). All `sbx` calls doctor makes are read-only (`diagnose`/`version`/`ls`/`policy ls`); doctor MUST NOT invoke `sbx create`/`run`/`exec`/`rm`, and tests assert no sandbox-mutating command is constructed.

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
5. Confirm any unverified `sbx` spellings this change relies on (port unpublish via `sbx ports --help`) and pin the verified shapes in fakeRunner; smoke-verify the finalised commands against a sandbox built from the Dune sbx template (D6 below).
6. `sbx-6` layers kits/cleanup on top.

### Smoke verification against the `sbx` binary (acceptance gates)
fakeRunner covers argument construction/sequencing; the following outcomes can only be proven against a real sandbox and are the acceptance gates for this change:
- `dune destroy` actually removes the sandbox: after `dune up` then `dune destroy --force`, `sbx ls --json` no longer lists the instance, and a subsequent `dune up` recreates it with profile-scoped persisted state intact (ties into `sbx-3` D3's persist sentinel).
- `dune doctor` is non-mutating: running it against an absent and against a stopped sandbox does not change what `sbx ls --json` reports (no create/start), and against a healthy host reports the host/sbx and sandbox checks as `pass`.
- `dune logs` surfaces both a Dune-owned log line and `sbx policy log` records for a running instance, and exposes no `pipelock` subcommand.
- `dune ports` lists published ports via the verified `sbx ports <sandbox>` shape; the loopback-vs-all-interfaces guidance is exercised on a publish attempt.
Temporary sandboxes created for these checks are removed (`sbx rm`).

## Open Questions

Each open question has a designated resolution point against the installed `sbx` binary or an implementation decision recorded in tests, so it is closed by verification rather than left standing.

- The exact Dune-owned setup/runtime log source `dune logs` reads is **resolved during implementation** (D3): pick the host-side lifecycle log the runner already writes plus any `sbx-2`-recorded in-sandbox setup path, and assert its presence in the composed output. Not a standing open question.
- Whether a port *unpublish* subcommand exists is resolved by `sbx ports --help` (D1) before `dune ports` exposes unpublish; until confirmed, `dune ports` ships list + publish only.
- Whether `dune doctor` gets an opt-in deep mode (e.g. `--deep`) for template pull/availability — the default-lightweight behavior is fixed; only the deep-mode flag name is open.
- Final confirmation UX for `dune destroy` (interactive prompt vs `--force`); both paths are tested regardless of the prompt wording.
