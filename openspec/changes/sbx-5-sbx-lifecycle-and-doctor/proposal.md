## Why

With execution on the sbx backend (`sbx-3`) and the egress/secrets posture defined (`sbx-4`), Dune's user-facing commands and failure handling need to be expressed in sbx terms rather than Docker Compose terms. `sbx-3` intentionally left the `up/down/rebuild/logs` mapping as interim backend operations; this change finalises that command surface, adds structured runtime diagnostics for sbx failures, and adds a read-only `dune doctor` readiness command.

The spikes sketched the shape. Spike 2's command mapping covers `dune up/down/rebuild/logs`, an optional `dune destroy`/`rm`, and an optional `dune ports` (with a loopback-vs-all-interfaces bind caveat for nested services). Spike 2 also found there is no single top-level `sbx logs` command, so Dune must define its own logs story, and noted `s6-svstat` was unavailable, so service status relies on service-native checks. Spike 3 recommended keeping `dune logs` focused on `sbx` lifecycle/policy logs plus Dune-owned setup/runtime logs, with project app-service logs increasingly belonging to project-owned Compose. This change carries that direction (consistent with the `sbx-2` service de-emphasis).

This proposal scopes the **finalised lifecycle UX, diagnostics, and doctor**. It inherits the reusable primitives of the superseded `ref-3-improve-runtime-diagnostics` (a small diagnostic-error type, stable codes, preserved stderr, recovery hints) and `ref-4-add-dune-doctor` (a structured `Check` model, pass/warn/fail/skip, concise output + optional `--json`), retargeted from Docker/Compose onto sbx.

## What Changes

- **NEW** Finalise the `dune` command mapping onto the sbx lifecycle, building on the `sbx-3` backend operations:
  - `dune` / `dune up`: validate sbx readiness, ensure the Dune sbx template is available, create the sandbox if missing, start it if stopped, attach a shell in the mounted repo path.
  - `dune down`: stop the sandbox (state retained).
  - `dune destroy` (**new first-class command**, resolving the `sbx-3` deferred question): remove the sandbox (`sbx rm`); profile-scoped persisted state survives via the durable persist location (`sbx-3` D3).
  - `dune rebuild`: recreate the sandbox from the Dune sbx template (no `Dockerfile.dune` path), preserving profile-scoped persisted state.
  - `dune logs`: compose Dune-owned setup/runtime logs with `sbx policy log` (egress), replacing Docker Compose/Pipelock logs. In-template/app service logs are **not** the focus — app dependencies are project-owned Compose, so their logs come from `docker compose logs` inside the sandbox.
  - `dune ports` (**optional, first-class if included**): wrap `sbx ports` list/publish/unpublish, with guidance on the loopback-vs-all-interfaces bind caveat (services bound to sandbox loopback may not be publishable).
- **NEW** Structured runtime diagnostics for sbx failures: a small diagnostic-error type with stable error codes, preserved command stderr, and actionable recovery hints; failure-mode tests assert codes rather than brittle strings. The taxonomy is intentionally small and sbx-scoped.
- **NEW** `dune doctor`: a read-only readiness command that does **not** start or enter the environment. It reports `sbx diagnose` (readiness/auth/daemon/version), Dune sbx template availability, sandbox status, the egress policy baseline (from `sbx-4`), and — optionally and non-fatally — in-template service health. Output is concise and human-readable with an optional machine-readable (`--json`) mode.

### Non-goals (explicitly deferred)

- The Dune sbx template and its service/log conventions (`sbx-2`); the sbx runtime backend (`sbx-3`); the egress baseline, domain-opening, and `sbx policy log` access plumbing (`sbx-4`). This change consumes those.
- `sbx` kits and removal of remaining Compose scaffolding / legacy base image (`sbx-6`).
- A repair command (`dune doctor --fix`) — doctor is read-only.
- Deep/expensive checks (e.g. forcing a full template pull) by default.

## Capabilities

### New Capabilities

- `sbx-lifecycle`: The finalised `dune` command surface over the sbx backend — `up`/`down`/`destroy`/`rebuild`/`logs` (and optional `ports`) — with `dune logs` composed from Dune-owned logs plus `sbx policy log`.
- `runtime-diagnostics`: A small, sbx-scoped diagnostic-error type with stable codes, preserved stderr, and recovery hints, reusable by lifecycle commands and `dune doctor`.
- `dune-doctor`: A read-only readiness command that inspects sbx/template/sandbox/policy (and optional service) readiness without starting the environment, with concise and optional JSON output.

## Impact

- **User-facing commands**: The `dune` verbs are finalised on sbx; `dune destroy` is added; `dune logs` is recomposed (no `dune logs pipelock`); `dune ports` optionally added.
- **Diagnostics**: Runtime failures return structured, coded errors with preserved stderr and recovery hints; tests assert codes.
- **New command**: `dune doctor` (read-only). Reuses backend readiness (`sbx-3`) and the egress baseline (`sbx-4`) without starting a sandbox.
- **Dependencies**: Requires `sbx-3` (backend ops) and `sbx-4` (egress baseline + `sbx policy log` access). Service-health checks are best-effort, consistent with the `sbx-2` de-emphasis.

## Depends On

`sbx-3-sbx-runtime-backend` (command mapping and diagnostics target the sbx runtime) and `sbx-4-sbx-network-and-secrets` (`dune logs` policy-log integration and the `dune doctor` policy-baseline check). Service health/log conventions come from `sbx-2-dune-sbx-template`.
