## 1. Finalise lifecycle commands

- [ ] 1.1 Finalise `dune` / `dune up`: Validate → ensure template available → Ensure → Start (if stopped) → Shell at the mounted repo path; reuse a running sandbox without recreation (D1).
- [ ] 1.2 Finalise `dune down` → Stop (state retained) and `dune rebuild` → recreate from the template preserving profile-scoped persisted state (D1).
- [ ] 1.3 Add `dune destroy` → `sbx rm <instance>` with confirmation (or `--force`); profile-scoped persisted state survives (D2).
- [ ] 1.4 (Optional) Add `dune ports` wrapping `sbx ports` list/publish/unpublish, surfacing the loopback-vs-all-interfaces caveat (D1).

## 2. dune logs composition

- [ ] 2.1 Compose `dune logs` from Dune-owned setup/runtime logs + `sbx policy log <instance>`; remove the `dune logs pipelock` surface (D3, `sbx-4`).
- [ ] 2.2 Document that app-dependency service logs come from `docker compose logs` inside the sandbox (project-owned Compose), not `dune logs`.

## 3. Structured diagnostics

- [ ] 3.1 Add a small `DiagnosticError` type (code/summary/detail/cause/command/stderr/recovery) with `IsDiagnostic`/`AsDiagnostic`/`WrapCommandError` (D4).
- [ ] 3.2 Map sbx backend failures to the initial code set (`sbx.not_installed`, `sbx.diagnose_failed`, `sbx.version_below_min`, `sbx.create_failed`, `sbx.start_failed`, `sbx.stop_failed`, `sbx.exec_failed`, `sbx.rm_failed`, `template.unavailable`, `policy.apply_failed`, `workspace.invalid`, `profile.config_corrupt`), preserving stderr and attaching recovery hints (D4).
- [ ] 3.3 Keep default CLI output concise; show command/stderr under verbose mode.

## 4. dune doctor

- [ ] 4.1 Add a `Check` model (id/group/severity/status/summary/detail/recovery; status pass/warn/fail/skip) and a read-only `dune doctor` that does not start or enter the environment (D5).
- [ ] 4.2 Implement checks: host/sbx (PATH, `sbx diagnose`, min version), template availability (lightweight; deep pull opt-in), sandbox status (`sbx ls --json`), workspace/profile/config/persist dirs, egress baseline (from `sbx-4`; `warn` if unconfirmable), and optional non-fatal in-template service health.
- [ ] 4.3 Concise human-readable output plus an optional `--json` mode; reuse diagnostic codes/recovery hints where checks map to runtime failure modes.

## 5. Tests

- [ ] 5.1 Fake-runner tests for `up`/`down`/`destroy`/`rebuild` command construction and sequencing (extending the `sbx-3` seam), including `dune destroy` → `sbx rm` and rebuild preserving persisted state.
- [ ] 5.2 Diagnostics tests asserting codes and preserved stderr for representative failures (missing sbx, failed diagnose, below-min version, create/start/exec/rm failures, template unavailable, corrupt profiles).
- [ ] 5.3 `dune doctor` tests for pass/warn/fail/skip cases and `--json` shape; assert it never starts a sandbox.

## 6. Build, smoke, docs

- [ ] 6.1 Run `go build ./cmd/dune` and `go test ./...`.
- [ ] 6.2 Smoke-verify the finalised commands against a sandbox built from the Dune sbx template; remove temporary sandboxes.
- [ ] 6.3 Update docs for the finalised command surface, `dune logs` composition, diagnostics, and `dune doctor`.
