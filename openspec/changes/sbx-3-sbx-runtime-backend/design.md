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
- Today's `dune-persist-<profile>` Docker volume is **profile-scoped** (shared across every workspace using that profile) and holds agent credentials/config that the template exposes via `/persist` symlinks (e.g. `~/.claude -> /persist/agent/.claude`).
- A sandbox is **per workspace+profile**, and `rebuild` recreates it (D7). If profile state lived only inside the sandbox, it would not be shared across a profile's workspaces and would be lost on `rebuild`.
- Decision: back `/persist` with a **durable, profile-scoped location decoupled from any single sandbox**, mirroring the old volume's semantics, so profile state is shared across that profile's sandboxes and survives `rebuild`. The rest of the sandbox filesystem persists via the sandbox lifecycle (until `rm`).
- The concrete mechanism depends on what `sbx` supports. The spikes verified that workspace direct-mount persists across `stop`/`run` but did **not** test an additional persist mount or behavior across recreate. Implementation must prove one supported mechanism before cutover: preferably mount a host-side profile-scoped persist dir (e.g. under `~/.local/share/dune/persist/<profile>`) into the sandbox at `/persist`; otherwise use an `sbx`-native per-profile volume if one exists. If neither mechanism is available, the runtime backend is blocked until Dune can provide an equivalent durable `/persist` mapping; accepting `rebuild` credential loss is not an allowed fallback.

`down` maps to `sbx stop` (sandbox retained); whether a `dune destroy`/`rm` command is added is deferred to `sbx-5`. The Docker `dune-persist-<profile>` volume itself is removed from the active path; its *semantics* are preserved by the durable persist location above.

### D4: Direct mount and shell working directory
`Ensure` creates the sandbox from the Dune template with the workspace direct-mounted (e.g. `sbx create --name <instance> --template <ref> shell <workspaceRoot>`) and passes the absolute mount path to the template (the `DUNE_WORKSPACE` contract from `sbx-2` D4) so `/workspace` resolves correctly. `Shell` attaches with the working directory set explicitly to the mounted repo path (`sbx exec -it -w <workspaceRoot> <instance> zsh`), because the spike showed the default `pwd` is `/home/agent/workspace`, not the repo.

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
`Validate` replaces `validateDockerPrerequisites`. It checks: `sbx` is on `PATH`; `sbx diagnose --output json` reports all checks passing (this also covers daemon health and authentication, per the spikes); and the installed `sbx version` meets a minimum (the spikes used `v0.32.0`). Each failure returns a clear, actionable error. Docker is no longer a host requirement for `dune`.

### D7: Lifecycle sequencing for `up` (backend ops; final UX in sbx-5)
`up` becomes: `Validate` → ensure the Dune template is available (sbx pulls the template ref directly on `create`; `sbx template load` / `sbx secret set --registry` only where needed) → `Ensure` (create if missing) → `Start` (if stopped) → `Shell`. `Status` (via `sbx ls --json`) replaces `isAgentCreated`/`isAgentRunning`. `down` → `Stop`. `rebuild` → recreate the sandbox from the (possibly updated) template — there is no `Dockerfile.dune` build. Because profile-scoped persisted state lives in the durable persist location (D3) rather than inside the sandbox image, recreating the sandbox on `rebuild` preserves agent credentials/config. The exact user-facing wiring of `up/down/rebuild/logs` is finalised in `sbx-5`; this change provides the backend operations they call.

### D8: Removals from the active runtime path
Remove `project{}`; the compose template `compose.yaml.tmpl`; `renderComposeFile`/`ensureComposeFile`/`validateComposeFile`; `ensureVolume`; `prepareAgentImage`; `composeArgs`/`composeUp`/`localImageExists`; `isAgentRunning`/`isAgentCreated`; `validateDockerPrerequisites`; and the `Dockerfile.dune` detection + `dune-local-<slug>:latest` build (`UseBuild`). Preserve `selfUpdate`, the profile store (`loadProfileStore`/`saveProfileStore`, `~/.config/dune/profiles.json`), `resolveProfile`/`validateProfileName`, workspace resolution, `effectiveTimezone`, and the `cli` option parsing.

### D9: Pipelock not started in the sbx path; package removed in sbx-4
`up`/`rebuild` no longer call `ensurePipelockConfig` or start a Pipelock sidecar. Egress is governed by the `sbx` network policy layer (the sandbox does not bypass it — `sbx-2`). The `internal/dune/pipelock` package, the generated `~/.config/dune/pipelock.yaml`, and the proxy-env model remain physically present but unused after this change, and are fully removed in `sbx-4` alongside the explicit sbx network-policy baseline. Sequencing this way ensures egress is mediated by sbx policy at all times.

### D10: Template reference in `internal/version`
Add a template reference accessor alongside `version.BaseImageRef()` (e.g. `version.SbxTemplateRef()`) sourced from the template version defined in `sbx-2`, kept in lockstep with `VERSION`. The backend reads it to populate `Spec.TemplateRef`.

### D11: compose-lifecycle retirement is captured in the sbx-runtime spec
The proposal lists `compose-lifecycle` as a modified capability, but there is no promoted main spec under `openspec/specs/` to emit a `MODIFIED`/`REMOVED` delta against. Its retirement is therefore captured as `ADDED` requirements in the new `sbx-runtime` capability spec (Docker is no longer required; Compose execution and the `Dockerfile.dune` build are removed). If/when main specs are promoted, a `REMOVED` delta can be added.

## Risks / Trade-offs

- **User-visible behavior change.** Docker is no longer required; users must install and authenticate `sbx`. Communicated via `Validate` errors and docs.
- **Orphaned Docker workspaces.** Existing Compose containers/volumes are not migrated; cleanup is addressed in `sbx-6`. Document the one-time transition.
- **sbx CLI surface drift.** `sbx create/exec/ls` flags may vary across versions. Mitigation: pin a minimum version (D6) and assert exact argument construction in fakeRunner tests.
- **PTY attach fidelity.** Interactive `Stream` cannot be faked; rely on smoke tests against the template, as today.
- **Egress window.** Between removing Pipelock startup (this change) and setting the sbx policy baseline (`sbx-4`), egress relies on the host's current sbx default policy. Mitigation: sequence `sbx-3`→`sbx-4` closely; document that the sandbox never bypasses sbx mediation.
- **Working-directory correctness.** The shell must start at the mounted repo path (`-w`); verify, since the sandbox default `pwd` differs.
- **Persistence mechanism is unverified.** The durable, profile-scoped persist location (D3) assumes `sbx` can either mount an extra host directory at `/persist` or provide a per-profile volume; the spikes only verified workspace direct-mount across `stop`/`run`. If neither is available, `rebuild` (and any recreate) would reset in-sandbox agent state, regressing the `dune-persist-<profile>` semantics. Mitigation: confirm the supported mechanism first and block runtime cutover until durable `/persist` semantics are proven.

## Migration Plan

1. Land after `sbx-2` provides the template ref.
2. Add `internal/dune/runtime/sbx` (+ `Runner`), `Spec`, and readiness validation; restructure `app.go` to resolve → build `Spec` → dispatch to the backend.
3. Remove the Docker Compose code, `Dockerfile.dune` build, and Pipelock startup (package removed in `sbx-4`).
4. Replace Compose golden/validation/shell-shim tests with fakeRunner command-construction tests; keep/Add a smoke test that enters a sandbox built from the Dune template.
5. `sbx-4`/`sbx-5` build on the resulting backend.

## Open Questions

- Minimum `sbx` version to pin (candidate: `v0.32.0`).
- Sandbox agent type: use the `shell` agent with Dune's own agent CLIs run as normal binaries (assumed) vs. any `sbx` built-in agent.
- Whether template availability ever needs an explicit `sbx template load` step, or `sbx create --template <registry-ref>` always suffices (with `sbx secret set --registry` for private pulls).
- Whether `dune destroy`/`dune rm` is introduced for sandbox removal (deferred to `sbx-5`).
- The durable-persist mechanism for profile-scoped state (extra host mount at `/persist` vs. an `sbx`-native per-profile volume), and the exact commands/flags needed to prove it preserves `/persist` across `rebuild` — gated on what `sbx` supports for mounts/volumes (D3).
