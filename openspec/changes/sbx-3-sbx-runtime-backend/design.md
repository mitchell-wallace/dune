## Context

`internal/dune/app.go` realises a Dune workspace with local Docker Compose. `Run` (`app.go:85`) resolves the workspace and profile, builds a Docker-shaped `project{}` (`app.go:69`), and for `up/down/rebuild/logs` it ensures a Pipelock config (`app.go:420`), renders/validates a compose file (`app.go:453`), creates a `dune-persist-<profile>` volume (`app.go:509`), pulls/builds the agent image (`app.go:516`), and drives `docker compose` through `composeArgs` (`app.go:588`). Docker prerequisites are checked in `validateDockerPrerequisites` (`app.go:575`). Two low-level execution helpers exist: `capture` (combined output) and `runStreaming` (interactive/PTY).

The `sbx-1-runtime-spike` reports establish the decision to hard-cut this execution to the standalone `sbx` CLI, launching a sandbox built from the Dune sbx template (`sbx-2-dune-sbx-template`). This change replaces the Docker Compose execution path with an `sbx` runtime layer. It supersedes the dual-backend direction of `ref-2` and reshapes `ref-1`'s (never-implemented) `EnvironmentPlan` boundary into a minimal sbx-oriented `resolve → plan → execute` flow — there is no `internal/dune/plan` package today.

Scope is the **CLI runtime backend**. The Dune sbx template is assumed to exist (`sbx-2`). Network/secrets posture and full Pipelock removal are `sbx-4`; final command-mapping detail, diagnostics, and `dune doctor` are `sbx-5`.

## Goals / Non-Goals

**Goals**
- Add an `internal/dune/runtime/sbx` package that owns sbx execution behind a lifecycle-oriented interface (not Docker/sbx primitives leaking into `app.go`).
- Validate host sbx readiness (present, daemon healthy, authenticated, minimum version) before any runtime operation.
- Introduce a minimal semantic environment description and a `resolve → plan → execute` structure in `app.go`.
- Map a Dune instance to a sandbox named `dune-<slug>-<profile>` and manage create/start/stop/attach.
- Direct-mount the workspace and attach the shell at the real mounted repo path.
- Add a command-runner seam so sbx command construction/sequencing is unit-testable without an sbx daemon.
- Remove the Docker Compose execution path, the `Dockerfile.dune` build, and Pipelock startup from the runtime flow.
- Preserve the non-Docker user surface: command names, workspace resolution, profile store, profile-name validation, timezone, and self-update.

**Non-Goals**
- The template image (`sbx-2`).
- Network policy baseline, domain-opening UX, `sbx policy log` surfacing, secrets posture, and full `internal/dune/pipelock` removal (`sbx-4`).
- Finalised `dune up/down/rebuild/logs` (+ optional `dune ports`) UX, structured diagnostics, and `dune doctor` (`sbx-5`).
- Kits and removal of remaining Compose scaffolding (`sbx-6`).
- Clone / copy-in-out workspace models.

## Decisions

### D1: A lifecycle-oriented sbx backend in `internal/dune/runtime/sbx`
Introduce a backend whose surface is Dune lifecycle operations, not sbx primitives. Indicative shape:

```go
type Backend interface {
    Validate(ctx context.Context) error
    Ensure(ctx context.Context, env Spec) error            // create sandbox if missing, from the Dune template
    Start(ctx context.Context, env Spec) error             // run/start if stopped
    Shell(ctx context.Context, env Spec, io StdIO) error   // attach interactive shell at mounted repo path
    Stop(ctx context.Context, env Spec) error
    Status(ctx context.Context, env Spec) (State, error)   // exists / running, via `sbx ls --json`
}
```

sbx-specific verbs (`sbx create/run/exec/stop/rm/ls`) are private to the package. This mirrors the lifecycle-vs-primitive boundary argued in the `ref-2` reference draft, retargeted to sbx.

### D2: A minimal semantic environment description, resolved in `app.go`
Replace `project{}` with a small backend-agnostic `Spec` carrying only semantic fields: instance name, workspace host path, profile, template reference, working directory (the mounted repo path), shell, and timezone. `app.go` performs a synchronous resolution phase (workspace resolve, profile resolve, config/data dirs as still needed, timezone, template ref) and then hands the `Spec` to the backend. This is the reshaped successor to `ref-1`'s `EnvironmentPlan`, deliberately minimal rather than generalised for hypothetical backends. Compose-specific fields (`ComposeProject`, `ComposeDir`, `ComposePath`, `PersistVolume`, `AgentImage`, `UseBuild`, `PipelockImage`, `PipelockConfigPath`) are dropped.

### D3: Instance/sandbox naming and persistence
The instance name keeps the `dune-<slug>-<profile>` shape (today's Compose project name) and is used as the sandbox name, scoped per workspace+profile. Persistence needs more care than "the sandbox persists until removed":
- Today's `dune-persist-<profile>` Docker volume is **profile-scoped** (shared across every workspace using that profile) and holds agent credentials/config that the template exposes via symlinks (e.g. `~/.claude -> /persist/agent/.claude`). Note the actual compose mount target is **`/persist/agent`** (the volume `agent_persist` → `target: /persist/agent`), not bare `/persist`; the sbx-backed durable mapping must reproduce the same in-sandbox path the template's symlinks expect (`/persist/agent`).
- A sandbox is **per workspace+profile**, and `rebuild` recreates it (D7). If profile state lived only inside the sandbox, it would not be shared across a profile's workspaces and would be lost on `rebuild`.
- Decision: back `/persist/agent` with a **durable, profile-scoped location decoupled from any single sandbox**, mirroring the old volume's semantics, so profile state is shared across that profile's sandboxes and survives `rebuild`. The rest of the sandbox filesystem persists via the sandbox lifecycle (until `rm`).
- The concrete mechanism is **confirmed (spike 4, Part 3)**: `sbx create AGENT PATH [PATH...]` accepts **extra positional workspace paths**, each mounted read-write (`:ro` opts out) as virtiofs **at its absolute host path** inside the sandbox; there is no named-volume flag and no `--env` on create. Dune therefore owns a host-side profile-scoped persist dir (e.g. `~/.local/share/dune/persist/<profile>`) and passes it as an extra workspace on every create: `sbx create --name <instance> --template <ref> shell <workspace> <persist-dir>`. Spike 4 proved a sentinel written in that mount **survives `sbx rm --force` + recreate** — strictly stronger than the old volume semantics, since the data lives on the host. Because the in-sandbox mount path equals the *host* path, the template's `/persist/agent` symlink target is wired by the re-homed `setup-persist` step (`sbx-2` D2a), which must be pointed at the mounted persist dir (its `PERSIST_DIR` env override, or an in-sandbox `/persist/agent` symlink to the mount) rather than assuming a fixed image path.
- **Verification point (gating cutover, mechanism now proven standalone):** spike 4 already proved recreate-survival with the exact shape above (`sbx rm --force` + recreate; sentinel intact). The remaining gate is reproducing it through `Ensure` in the smoke test (task 8.1), not re-discovering the mechanism. Note `sbx rm` requires `--force` when stdin is not a TTY (spike 4), which is how the backend always invokes it.

`down` maps to `sbx stop` (sandbox retained); whether a `dune destroy`/`rm` command is added is deferred to `sbx-5`. The Docker `dune-persist-<profile>` volume itself is removed from the active path; its *semantics* are preserved by the durable persist location above.

### D4: Direct mount and shell working directory
`Ensure` creates the sandbox from the Dune template with the workspace direct-mounted. The spikes verified the create shape `sbx create --name <name> --template <ref> shell <absolute host path>` (the agent `shell` and the host path are **positional**; there is no `--mount` flag). After create, `Ensure` passes the absolute mount path to the template with the sbx-2 D4 contract so `/workspace` resolves correctly.

The mount-path delivery and the shell working-directory flags were **both resolved by spike 4** (the earlier plans gated them on `--help`); each still carries a smoke-test assertion for its runtime outcome:

- **Mount-path delivery (resolved by sbx-2 D4): `sbx create` has no `--env` flag.** The backend creates the sandbox with `sbx create --name <instance> --template <ref> shell <absolute host path> <persist-dir>`, then runs the deterministic post-create hook exec as `sbx exec -e DUNE_WORKSPACE=<absolute host path> <instance> bash -lc true`. The login shell fires the template hook, which refreshes `/workspace` before the sentinel-guarded `setup-persist` step. A Dune-generated per-sandbox kit `environment.variables` entry remains the experimental create-time option for later kit work, but it is not the sbx-3 contract; in-sandbox derivation is intentionally avoided because the profile-persist dir is also a rw virtiofs workspace. Assert this create/exec argument construction in fakeRunner tests and confirm `/workspace` resolves in the smoke test.
- **Interactive attach (resolved by spike 4): `sbx exec` supports `-it` and `-w`** (plus `-e/--env`, `--env-file`, `-u/--user`, `-d/--detach`; "flags match the behavior of `docker exec`"). The intended shape `sbx exec -it -w <workspaceRoot> <instance> zsh` is supported as spelled and is the shape to pin in fakeRunner tests. The working-directory outcome is asserted by the smoke test scenario (`git rev-parse --show-toplevel` succeeds from the shell's cwd), since the PTY attach itself cannot be faked.

### D5: Command-runner seam
Generalise today's `capture`/`runStreaming` into a small `Runner` used by the backend:

```go
type Runner interface {
    Capture(ctx context.Context, dir, name string, args ...string) ([]byte, error)
    Stream(ctx context.Context, dir string, io StdIO, name string, args ...string) error
}
```

A `defaultRunner` wraps `os/exec` (preserving current capture/stream behavior). A `fakeRunner` records each invocation (dir, name, args) and returns preset output, enabling command-construction and sequencing tests without an sbx daemon. The interactive `Stream` path (PTY attach) cannot be meaningfully faked and is covered by smoke tests, as `runStreaming` is today.

### D6: sbx readiness validation replaces Docker prerequisites
`Validate` replaces `validateDockerPrerequisites`. It checks: `sbx` is on `PATH`; `sbx diagnose --output json` reports all checks passing (this also covers daemon health and authentication, per the spikes — the spikes saw 8 checks pass); and the installed version (from `sbx version`, which the spikes observed returns `v0.32.0`) meets a minimum. Each failure returns a clear, actionable error. Docker is no longer a host requirement for `dune`.

Flag note (verified): the JSON flag is **not uniform across sbx subcommands** — `sbx diagnose` uses `--output json` while `sbx ls` (D1 `Status`) uses `--json`. Use each command's verified form; do not assume `sbx diagnose --json` works. The exact parse shapes are pinned by fakeRunner tests so a future sbx flag change surfaces as a test failure.

### D7: Lifecycle sequencing for `up` (backend ops; final UX in sbx-5)
`up` becomes: `Validate` → ensure the Dune template is available (sbx pulls the template ref directly on `create`; `sbx template load` / `sbx secret set --registry` only where needed) → `Ensure` (create if missing) → `Start` (if stopped) → `Shell`. `Status` (via `sbx ls --json`, verified) replaces `isAgentCreated`/`isAgentRunning`. `down` → `Stop` (verified shape `sbx stop <name>`). `rebuild` → recreate the sandbox from the (possibly updated) template via `sbx rm <name>` (verified, accepts multiple names) followed by `Ensure`/`Start` — there is no `Dockerfile.dune` build.

`Start` shape is **confirmed (spike 4)**: `sbx run <sandbox-name>` starts/attaches an existing sandbox (positional name, no flags; `sbx run [flags] SANDBOX | AGENT [PATH...] [-- AGENT_ARGS...]` also creates-if-missing when given an agent form). Additionally `sbx exec` **auto-starts a stopped sandbox** ("If the sandbox is stopped, it is started first" — empirically confirmed), so non-interactive backend ops need no separate start step. Pin the chosen shape in fakeRunner tests. `Status` must distinguish *exists-but-stopped* from *running* from the `sbx ls --json` fields the spike confirmed (sandbox name + status) so `up` only calls `Start` when stopped and reuses a running sandbox without recreation. Because profile-scoped persisted state lives in the durable persist location (D3) rather than inside the sandbox image, recreating the sandbox on `rebuild` preserves agent credentials/config. The exact user-facing wiring of `up/down/rebuild/logs` is finalised in `sbx-5`; this change provides the backend operations they call.

### D8: Removals from the active runtime path
Remove `project{}`; the compose template `compose.yaml.tmpl`; `renderComposeFile`/`ensureComposeFile`/`validateComposeFile`; `ensureVolume`; `prepareAgentImage`; `composeArgs`/`composeUp`/`localImageExists`; `isAgentRunning`/`isAgentCreated`; `validateDockerPrerequisites`; and the `Dockerfile.dune` detection + `dune-local-<slug>:latest` build (`UseBuild`). Preserve `selfUpdate`, the profile store (`loadProfileStore`/`saveProfileStore`, `~/.config/dune/profiles.json`), `resolveProfile`/`validateProfileName`, workspace resolution, `effectiveTimezone`, and the `cli` option parsing.

### D9: Pipelock not started in the sbx path; package removed in sbx-4
`up`/`rebuild` no longer call `ensurePipelockConfig` or start a Pipelock sidecar. Egress is governed by the `sbx` network policy layer (the sandbox does not bypass it — `sbx-2`). The `internal/dune/pipelock` package, the generated `~/.config/dune/pipelock.yaml`, and the proxy-env model remain physically present but unused after this change, and are fully removed in `sbx-4` alongside the explicit sbx network-policy baseline. Sequencing this way ensures egress is mediated by sbx policy at all times.

### D10: Template reference in `internal/version`
Add a template reference accessor alongside `version.BaseImageRef()` (e.g. `version.SbxTemplateRef()`) returning the Dune sbx template image ref from `sbx-2` D6 (proposed `ghcr.io/mitchell-wallace/dune-sbx:<version>`). It mirrors the existing `BaseImageRef()` mechanism: an ldflags-injectable repo + version, with the version falling back to a source file read (`BaseImageRef()` reads `container/base/IMAGE_VERSION`, so the template accessor reads the template's own version file — `container/sbx/IMAGE_VERSION` per `sbx-2` D6 — not the top-level `VERSION`). "Lockstep with `VERSION`" is a release convention (the version-bump checklist), not the literal source this accessor reads. The backend reads `version.SbxTemplateRef()` to populate `Spec.TemplateRef`. A unit test asserts the accessor returns the expected repo and a non-empty version (mirroring any existing `BaseImageRef` test).

### D11: compose-lifecycle retirement is captured in the sbx-runtime spec
The proposal lists `compose-lifecycle` as a modified capability, but there is no promoted main spec under `openspec/specs/` to emit a `MODIFIED`/`REMOVED` delta against. Its retirement is therefore captured as `ADDED` requirements in the new `sbx-runtime` capability spec (Docker is no longer required; Compose execution and the `Dockerfile.dune` build are removed). If/when main specs are promoted, a `REMOVED` delta can be added.

## Risks / Trade-offs

- **User-visible behavior change.** Docker is no longer required; users must install and authenticate `sbx`. Communicated via `Validate` errors and docs.
- **Orphaned Docker workspaces.** Existing Compose containers/volumes are not migrated; cleanup is addressed in `sbx-6`. Document the one-time transition.
- **sbx CLI surface drift.** `sbx create/exec/ls/run/stop/rm` flags may vary across versions. The previously-unverified shapes are now confirmed against v0.32.0 by spike 4 (interactive attach `-it`/`-w`, the `sbx run <name>` start shape and `sbx exec` auto-start, extra positional workspace mounts, `sbx ports --unpublish`, `sbx rm --force` for non-TTY). Mitigation against future drift: pin a minimum version (D6); assert exact argument construction in fakeRunner tests so a drift surfaces as a failing test; and cover the real-attach/persistence outcomes (which fakeRunner cannot observe) in the smoke test.
- **PTY attach fidelity.** Interactive `Stream` cannot be faked; rely on smoke tests against the template, as today.
- **Egress window.** Between removing Pipelock startup (this change) and setting the sbx policy baseline (`sbx-4`), egress relies on the host's current sbx default policy. Mitigation: sequence `sbx-3`→`sbx-4` closely; document that the sandbox never bypasses sbx mediation.
- **Working-directory correctness.** The shell must start at the mounted repo path (`-w`); verify, since the sandbox default `pwd` differs.
- **Persistence mechanism (verified, with a path caveat).** Spike 4 proved the durable profile-persist mapping: an extra positional workspace (host dir) survives `sbx rm --force` + recreate. The residual risk is path wiring, not durability: the extra workspace mounts at its **absolute host path** (not `/persist/agent`), so the `/persist/agent` symlink-target wiring inside the sandbox (via the re-homed `setup-persist`, `sbx-2` D2a) must point at the mounted host path. Mitigation: the smoke test asserts the wired symlinks resolve and the sentinel survives recreate via `Ensure`.

## Migration Plan

1. Land after `sbx-2` provides the template ref.
2. Add `internal/dune/runtime/sbx` (+ `Runner`), `Spec`, and readiness validation; restructure `app.go` to resolve → build `Spec` → dispatch to the backend.
3. Remove the Docker Compose code, `Dockerfile.dune` build, and Pipelock startup (package removed in `sbx-4`).
4. Replace Compose golden/validation/shell-shim tests with fakeRunner command-construction tests; keep/Add a smoke test that enters a sandbox built from the Dune template.
5. `sbx-4`/`sbx-5` build on the resulting backend.

## Open Questions

Each open question below has a designated resolution point against the installed `sbx` binary so it is closed by verification, not assumption.

- Minimum `sbx` version to pin (candidate: `v0.32.0`, the version the spikes observed via `sbx version`).
- Sandbox agent type: use the `shell` agent (the spikes' verified `sbx create ... shell <path>` form) with Dune's own agent CLIs run as normal binaries (assumed) vs. any `sbx` built-in agent.
- Whether template availability ever needs an explicit `sbx template load` step, or `sbx create --template <registry-ref>` always suffices (with `sbx secret set --registry` for private pulls). Spike 4 partially resolved this: a **host-local Docker image name is not usable** as `--template` (the sbx runtime has its own image store; create fails with `pull failed`), while `docker save` + `sbx template load <tar>` works and the loaded ref is then usable on create. Registry refs remain the normal path (spikes 1/2); `template load` is the verified offline/local-build path.
- Whether `dune destroy`/`dune rm` is introduced for sandbox removal (deferred to `sbx-5`).
- ~~The durable-persist mechanism for profile-scoped state~~ — **resolved by spike 4 (Part 3)**: extra positional workspace (host dir per profile), mounted rw at its host path; sentinel survived `sbx rm --force`/recreate (D3).
- ~~The interactive-attach flags (`-it`/`-w`) for `sbx exec`~~ — **resolved by spike 4 (Part 1)**: both exist; `sbx exec -it -w <dir> <instance> zsh` is supported as spelled (D4).
- ~~The `sbx run` (start/restart) invocation shape~~ — **resolved by spike 4 (Part 1)**: `sbx run <sandbox-name>` (positional) starts/attaches an existing sandbox; `sbx exec` auto-starts a stopped sandbox (D7).
- ~~The mount-path delivery contract~~ — **resolved by sbx-2 D4**: `sbx create` has no `--env`, so `Ensure` creates the sandbox normally and then runs `sbx exec -e DUNE_WORKSPACE=<absolute host path> <instance> bash -lc true` to fire the template hook that refreshes `/workspace`.
