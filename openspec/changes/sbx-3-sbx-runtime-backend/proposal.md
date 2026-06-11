## Why

Dune's host-side Go CLI currently realises a workspace with local Docker Compose. `internal/dune/app.go` computes a Docker-shaped `project{}` struct (`internal/dune/app.go:69`), renders `compose.yaml.tmpl`, validates and writes a compose file, creates a `dune-persist-<profile>` volume, pulls/builds the agent image, and drives `docker compose up/down/logs/exec` through `composeArgs` (`internal/dune/app.go:588`). Docker prerequisite checks live in `validateDockerPrerequisites` (`internal/dune/app.go:575`), and a Pipelock sidecar config is generated at `~/.config/dune/pipelock.yaml` (`internal/dune/app.go:420`).

The `sbx-1-runtime-spike` reports establish the product decision to hard-cut this execution path to the standalone `sbx` CLI. With the dedicated Dune sbx template from `sbx-2-dune-sbx-template` in place, this change replaces Docker Compose execution in the CLI with an `sbx`-backed runtime: instead of composing two containers, `dune` creates/enters an `sbx` sandbox built from the Dune template.

This is the central change of the migration. It supersedes the dual-backend direction of `ref-2-docker-backend-abstraction` and reshapes the pure-plan boundary idea from `ref-1-introduce-environment-boundary` (which was proposed but never implemented — there is no `internal/dune/plan` package today) into an sbx-oriented `resolve → plan → execute` flow. Per the spikes, Dune does not maintain a long-lived dual-backend matrix; the Docker Compose code remains only as migration scaffolding until cutover, and `Dockerfile.dune` is dropped rather than carried forward.

This proposal scopes the **CLI runtime backend**. It assumes the Dune sbx template exists (`sbx-2`). It defers the network/secrets posture and Pipelock removal to `sbx-4-sbx-network-and-secrets`, and the diagnostics/`dune doctor` work to `sbx-5-sbx-lifecycle-and-doctor`.

## What Changes

- **NEW** `internal/dune/runtime/sbx` package that owns sbx execution: it validates the host sbx install (present on PATH, daemon healthy, authenticated, meets a minimum sbx version — the spikes used `v0.32.0`), maps a Dune instance to a sandbox, and manages the sandbox lifecycle (`create`, `run`/start, `exec`/attach, `stop`, `rm`).
- **NEW** A lightweight semantic environment description and a `resolve → plan → execute` structure in `app.go`: resolve workspace/profile/config/template inputs, build a backend-agnostic environment description, then hand it to the sbx runtime. This is the reshaped, sbx-oriented successor to `ref-1`'s `EnvironmentPlan` — kept deliberately minimal rather than over-generalised for hypothetical backends.
- **NEW** A command-runner seam inside the sbx runtime so command construction and lifecycle sequencing are unit-testable without a running sbx daemon (mirroring the testing intent behind the existing `capture`/`runStreaming` helpers), plus command-construction tests.
- Instance/sandbox naming and persistence: a Dune instance maps to a sandbox named `dune-<workspace-slug>-<profile>` (per workspace+profile), preserving today's mental model. Profile-scoped persisted state (agent credentials/config — today's `dune-persist-<profile>` volume contents exposed under `/persist/agent`) is backed by a **durable, profile-scoped location decoupled from any single sandbox**, so it is shared across that profile's sandboxes and survives `rebuild` (which recreates the sandbox). The rest of the sandbox state persists via the sandbox lifecycle (until `rm`). The durable-persist mechanism must be proven before this change is considered complete; regressing to per-sandbox-only credential persistence is not acceptable for cutover.
- Workspace model (v1): **direct mount** of the resolved workspace root, with the attached shell starting in the real mounted repository path. The Dune sbx template's `/workspace` symlink (from `sbx-2`) provides legacy-path compatibility; the CLI does not assume `/workspace` is the repo.
- Replace the Docker prerequisite checks with sbx readiness validation (`sbx diagnose --output json` + `sbx version` minimum), and replace `docker compose exec ... zsh` shell attach with an `sbx exec` interactive shell whose working directory is the mounted repo path (intended `sbx exec -it -w <mounted repo path> <sandbox> zsh`; the `-it`/`-w` flags are confirmed against `sbx exec --help` before use, with a `cd <repo> && exec zsh -l` fallback if `-w` is unsupported — see design D4).
- **REMOVE (from the active runtime path)** the Docker-shaped `project{}` struct, `compose.yaml.tmpl`/compose rendering + validation, the Docker volume creation step, the base-image pull/`docker compose build` path, and the `Dockerfile.dune` detection/build behavior (`UseBuild`, `dune-local-<slug>:latest`). `Dockerfile.dune` is dropped, not migrated.
- Stop starting Pipelock for the sbx path. This change removes the Pipelock *startup/wiring* from the runtime flow; full removal of the `internal/dune/pipelock` package, the generated `pipelock.yaml`, and the proxy env model is completed in `sbx-4` alongside the sbx network-policy posture so egress is not left unmediated in between.
- Preserve the user-facing command surface and profile semantics that are not Docker-specific: `dune` / `dune up`, `dune down`, `dune rebuild`, `dune logs`, `dune version`, and `dune profile set/list`, plus workspace resolution (`git rev-parse --show-toplevel` with cwd fallback), the profile store at `~/.config/dune/profiles.json`, profile-name validation, and timezone resolution. The mapping of `up/down/rebuild/logs` onto sbx lifecycle is finalised in `sbx-5`; this change provides the backend operations they call.

### Non-goals (explicitly deferred)

- The Dune sbx template image itself (`sbx-2-dune-sbx-template`).
- The default network policy baseline, domain-opening affordance, `sbx policy log` surfacing, secrets posture, and final Pipelock package removal (`sbx-4-sbx-network-and-secrets`).
- The finalized `dune up/down/rebuild/logs` (+ optional `dune ports`) command mapping, structured runtime diagnostics, and `dune doctor` (`sbx-5-sbx-lifecycle-and-doctor`).
- `sbx` kits and removal of remaining Docker Compose scaffolding once parity is proven (`sbx-6-sbx-kits-and-cleanup`).
- Clone-mode and copy-in/copy-out workspace models (deferred; v1 is direct mount).

## Capabilities

### New Capabilities

- `sbx-runtime`: An sbx-backed runtime that validates the host sbx install, maps Dune instances to sandboxes named `dune-<slug>-<profile>`, and manages sandbox lifecycle and shell attach from a semantic environment description.

### Modified Capabilities

- `compose-lifecycle`: The Docker Compose lifecycle is retired as the realisation backend. Its user-facing intent (start/attach, stop, rebuild, logs) is re-expressed against the sbx runtime; generated compose files, the persist volume, and the `Dockerfile.dune` build path are removed from the active runtime. (There is no promoted `compose-lifecycle` main spec under `openspec/specs/` to emit a `MODIFIED`/`REMOVED` delta against, so this retirement is captured as `ADDED` requirements in the new `sbx-runtime` spec — see design D11.)

## Impact

- **Architecture**: Execution moves out of `app.go` into `internal/dune/runtime/sbx`. `app.go` becomes a thin `resolve → plan → execute` entry point. The Docker-shaped `project{}` and compose template are removed from the active path.
- **Behavior change (intended)**: Dune workspaces run inside an `sbx` sandbox rather than a Docker Compose pair. The shell attaches at the real mounted repository path. `Dockerfile.dune` is no longer detected or built.
- **Compatibility surfaces**: The instance name keeps the `dune-<slug>-<profile>` shape; the `dune-persist-<profile>` Docker volume is replaced by a durable, profile-scoped persist location (its semantics preserved) plus sandbox lifecycle persistence for the rest of the sandbox. Existing Docker Compose workspaces are not migrated automatically; users re-enter via the sbx backend. Migration/cleanup of stale Docker artifacts is addressed in `sbx-6`.
- **Egress**: Pipelock startup is removed from the sbx flow; the sbx network-policy posture that replaces it lands in `sbx-4`. The two changes are sequenced so egress is governed by sbx policy by the time Pipelock is fully removed.
- **Tests**: New command-construction/sequencing tests via the runner seam; existing Docker Compose golden/validation tests for the removed path are retired or replaced. Real end-to-end behavior is validated by smoke tests against the Dune sbx template.
- **Host requirements**: Docker is no longer required on the host for `dune`; `sbx` (installed, authenticated, daemon healthy, minimum version) becomes the prerequisite.
- **Dependencies**: Requires the Dune sbx template from `sbx-2`. Tightly sequenced with `sbx-4` (egress) and `sbx-5` (command mapping, diagnostics, doctor).
