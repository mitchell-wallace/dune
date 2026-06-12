## 1. Runtime package and runner seam

- [ ] 1.1 Create `internal/dune/runtime/sbx` with a lifecycle-oriented `Backend` (`Validate`, `Ensure`, `Start`, `Shell`, `Stop`, `Status`) keeping `sbx` verbs private to the package (design D1).
- [ ] 1.2 Define a `Runner` seam (`Capture`, `Stream`) and a `defaultRunner` wrapping `os/exec` that preserves current capture/stream behavior (D5).
- [ ] 1.3 Add a `fakeRunner` test helper that records each invocation (dir, name, args) and returns preset output/errors per call.

## 2. sbx readiness validation

- [ ] 2.1 Implement `Validate`: `sbx` on `PATH`; `sbx diagnose --output json` all checks pass (covers daemon health + auth); installed `sbx version` meets the minimum (candidate `v0.32.0`) — replacing `validateDockerPrerequisites` (D6).
- [ ] 2.2 Return clear, actionable errors for each unmet requirement; ensure no sandbox operation runs when validation fails.

## 3. Semantic Spec and app.go restructure

- [ ] 3.1 Define a minimal backend-agnostic `Spec` (instance name, workspace host path, profile, template ref, working dir, shell, timezone); remove `project{}` (D2, D8).
- [ ] 3.2 Add `version.SbxTemplateRef()` (alongside `BaseImageRef`) reading `container/sbx/IMAGE_VERSION` (not top-level `VERSION`; lockstep with `VERSION` is a release convention, per D10).
- [ ] 3.3 Restructure `app.go` into resolve → build `Spec` → dispatch to the backend, reusing existing workspace resolution, profile store, profile-name validation, and timezone logic.

## 4. Lifecycle operations and command construction

- [x] 4.0 ~~Confirm the unverified `sbx` command shapes~~ Resolved by spike 4 against `sbx` v0.32.0: `sbx create` has **no env flag** but accepts **extra positional rw workspaces** (the D3 persist mechanism); `sbx exec` supports `-it`/`-w` (and `-e`/`--env-file`/`-u`/`-d`); the start shape is `sbx run <sandbox-name>` (and `sbx exec` auto-starts a stopped sandbox); `sbx rm` needs `--force` when non-TTY. Pin these shapes in fakeRunner tests (design D3/D4/D7).
- [ ] 4.1 Implement `Ensure` to create the sandbox from the Dune template with the workspace direct-mounted (verified shape: `sbx create --name <instance> --template <ref> shell <absolute host path>`), passing the profile-persist host dir as an extra positional workspace (design D3), and delivering the mount path via the contract `sbx-2` D4 records — a Dune-generated per-sandbox kit (`--kit`, env var `DUNE_WORKSPACE`; verified by spike 4 but experimental) or the template's in-sandbox derivation (design D4).
- [ ] 4.2 Implement `Status` via `sbx ls --json` (replacing `isAgentCreated`/`isAgentRunning`), distinguishing exists-but-stopped from running; implement `Stop` via `sbx stop <name>` and `Start` via `sbx run <instance>` (spike 4; `sbx exec` also auto-starts stopped sandboxes for non-interactive ops).
- [ ] 4.3 Implement `Shell` as `sbx exec -it -w <mounted repo path> <instance> zsh` (using the flags confirmed in 4.0) so the shell starts in the repository; if `-w` is unsupported, fall back to `sbx exec -it <instance> zsh -lc 'cd <mounted repo path> && exec zsh -l'`.
- [ ] 4.4 Back the template's `/persist/agent` path with a durable, profile-scoped location decoupled from the sandbox so profile state is shared across a profile's sandboxes and survives `rebuild` (D3). Prove the supported mechanism (host mount at `/persist/agent` or an sbx-native per-profile volume) before cutover via the recreate-survival check in 8.1; do not accept per-sandbox-only persistence as a fallback.

## 5. Wire commands to the backend

- [ ] 5.1 Map `up` (default) to Validate → ensure template available → Ensure → Start (if stopped) → Shell, reusing a running sandbox without recreation (D7).
- [ ] 5.2 Map `down` → Stop and `rebuild` → recreate from the template (no `Dockerfile.dune` build), preserving profile-scoped persisted state across the recreate via the durable persist location (D3).
- [ ] 5.3 Keep `logs` functional against the sbx runtime as an interim mapping; the finalized `dune logs` composition (service logs + `sbx policy log`) is `sbx-5`.
- [ ] 5.4 Leave `version`, `profile set`, and `profile list` unchanged (no runtime/backend needed).

## 6. Remove the Docker Compose path

- [ ] 6.1 Remove `compose.yaml.tmpl`, `renderComposeFile`/`ensureComposeFile`/`validateComposeFile`, `ensureVolume`, `prepareAgentImage`, `composeArgs`/`composeUp`/`localImageExists`, and `isAgentRunning`/`isAgentCreated` (D8).
- [ ] 6.2 Remove `Dockerfile.dune` detection and the `dune-local-<slug>:latest` build (`UseBuild`).
- [ ] 6.3 Stop calling `ensurePipelockConfig` and starting the Pipelock sidecar in the runtime path. Leave the `internal/dune/pipelock` package present-but-unused; full removal is `sbx-4` (D9).

## 7. Tests

- [ ] 7.1 Add fakeRunner command-construction/sequencing tests for `up` (create/start/attach and reuse), `down`, and `rebuild`, asserting exact `sbx` args, instance name, mount path, and order.
- [ ] 7.2 Add tests for `Validate` failure modes (missing sbx, failed diagnose, below-min version).
- [ ] 7.3 Retire or replace the Docker Compose golden/validation and shell-shim tests tied to the removed path.

## 8. Smoke verification and docs

- [ ] 8.1 Add/adjust a smoke test against the installed `sbx` that: (a) creates a sandbox from the Dune sbx template and confirms the attached shell's cwd is the mounted repo path (`git rev-parse --show-toplevel` succeeds); and (b) gates D3 by writing a sentinel under `/persist/agent`, running `sbx rm <name>` then recreating, and confirming the sentinel survives the recreate.
- [ ] 8.2 Run `go build ./cmd/dune` and `go test ./...`.
- [ ] 8.3 Update architecture/README docs: `sbx` replaces Docker as the prerequisite; the runtime launches the Dune sbx template; `Dockerfile.dune` and the compose path are gone; note the one-time transition from Docker Compose workspaces (full cleanup in `sbx-6`).
