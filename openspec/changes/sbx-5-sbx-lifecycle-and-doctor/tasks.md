## 1. Finalise lifecycle commands

- [x] 1.1 Finalise `dune` / `dune up`: Validate → ensure template available → Ensure → Start (if stopped) → Shell at the mounted repo path; reuse a running sandbox without recreation (D1).
- [x] 1.2 Finalise `dune down` → Stop (state retained) and `dune rebuild` → recreate from the template preserving profile-scoped persisted state (D1).
- [x] 1.3 Add `dune destroy` → `sbx rm <instance>` with confirmation (or `--force`); profile-scoped persisted state survives (D2).
- [x] 1.4 Add `dune ports` over the verified `sbx ports` surface (list via `sbx ports <sandbox>` [+ `--json`], publish via `--publish [[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]`, unpublish via `--unpublish [HOST_IP:]HOST_PORT:SANDBOX_PORT[/PROTOCOL]` — spelling confirmed by spike 4), surfacing the loopback-vs-all-interfaces caveat (D1).

## 2. dune logs composition

- [x] 2.1 Compose `dune logs` from Dune-owned setup/runtime logs (host-side lifecycle log + the in-sandbox `/var/log/dune/` output per `sbx-2` D5a) + `sbx policy log <instance>`; remove the `dune logs pipelock` surface (D3, `sbx-4`). Assert both Dune-owned sources appear in the composed output.
- [x] 2.2 Document that app-dependency service logs come from `docker compose logs` inside the sandbox (project-owned Compose), not `dune logs`.

## 3. Structured diagnostics

- [x] 3.1 Add a small `DiagnosticError` type (code/summary/detail/cause/command/stderr/recovery) with `IsDiagnostic`/`AsDiagnostic`/`WrapCommandError` (D4).
- [x] 3.2 Map sbx backend failures to the initial code set (`sbx.not_installed`, `sbx.diagnose_failed`, `sbx.version_below_min`, `sbx.create_failed`, `sbx.start_failed`, `sbx.stop_failed`, `sbx.exec_failed`, `sbx.rm_failed`, `template.unavailable`, `policy.apply_failed`, `workspace.invalid`, `profile.config_corrupt`), preserving stderr and attaching recovery hints (D4).
- [x] 3.3 Keep default CLI output concise; show command/stderr under verbose mode.

## 4. dune doctor

- [x] 4.1 Add a `Check` model (id/group/severity/status/summary/detail/recovery; status pass/warn/fail/skip) and a read-only `dune doctor` that does not start or enter the environment (D5).
- [x] 4.2 Implement checks: host/sbx (PATH, `sbx diagnose --output json`, min version via `sbx version`), template availability (lightweight; deep pull opt-in), sandbox status (`sbx ls --json`, reusing the `sbx-3` D7 parse), workspace/profile/config/persist dirs, and egress baseline (inspect via `sbx policy ls` from `sbx-4`; `warn` when unconfirmable, `fail` only on an observed `Open` posture). Use only read-only `sbx` calls. Do not add in-template service health checks.
- [x] 4.3 Concise human-readable output plus an optional `--json` mode; reuse diagnostic codes/recovery hints where checks map to runtime failure modes.

## 5. Tests

- [x] 5.1 Fake-runner tests for `up`/`down`/`destroy`/`rebuild` command construction and sequencing (extending the `sbx-3` seam), including `dune destroy` → `sbx rm` and rebuild preserving persisted state.
- [x] 5.2 Diagnostics tests asserting codes and preserved stderr for representative failures (missing sbx, failed diagnose, below-min version, create/start/exec/rm failures, template unavailable, corrupt profiles).
- [x] 5.3 `dune doctor` tests for pass/warn/fail/skip cases and `--json` shape; assert it constructs only read-only `sbx` calls (`diagnose`/`version`/`ls`/`policy ls`) and never a sandbox-mutating command (`create`/`run`/`exec`/`rm`).

## 6. Build, smoke, docs

- [x] 6.1 Run `go build ./cmd/dune` and `go test ./...`.
- [ ] 6.2 Smoke-verify the finalised commands against a sandbox built from the Dune sbx template (acceptance gates, design Migration §"Smoke verification"): `dune destroy --force` removes the instance (absent from `sbx ls --json`) and a subsequent `dune up` recreates it with persisted state intact; `dune doctor` does not change `sbx ls --json` for an absent/stopped sandbox and reports `pass` on a healthy host; `dune logs` surfaces a Dune-owned line plus `sbx policy log` records and exposes no `pipelock` subcommand; `dune ports` lists via `sbx ports <sandbox>`. Remove temporary sandboxes (`sbx rm`).
- [ ] 6.3 Update docs for the finalised command surface, `dune logs` composition, diagnostics, and `dune doctor`.
