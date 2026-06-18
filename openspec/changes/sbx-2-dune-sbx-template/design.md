## Context

Dune's current runtime artifact is the Compose-oriented base image built by the root `Dockerfile` and published as `ghcr.io/mitchell-wallace/dune-base` (`container/base/IMAGE_VERSION` = `0.4.2`). It is consumed by `internal/dune/app.go`, which starts an `agent` container plus a `pipelock` sidecar via Docker Compose.

The `sbx-1-runtime-spike` reports validated that this image can be launched as an `sbx` `--template`, retaining Dune's tooling (Rally, Laps, agent CLIs, Playwright, language toolchains) and its persistence symlinks. But two concrete gaps make it unsuitable as the final sbx artifact:

- **No Docker Engine / Compose inside the sandbox.** The generic `sbx` Docker-enabled `shell` template (`docker/sandbox-templates:shell-docker`) proves Docker-in-sbx works — `dockerd` starts automatically on `sbx exec`, `docker compose version` reports `v5.1.4`, nested `docker compose up` works, and `sbx ports` can publish a nested service. The Dune base image lacks all of this.
- **Misleading `/workspace`.** The image sets `WORKDIR /workspace`, but under direct-mount `sbx` the repository is visible at its absolute host path, and `/workspace` exists yet is *not* the repo. Spike 3 confirmed a `/workspace` symlink to the mounted path resolves correctly.

A third, structural change rides along. The base image runs **s6-overlay** as PID 1 (`ENTRYPOINT ["/init"]`) to supervise in-container Postgres/Redis/Mailpit (`longrun`) plus the mandatory `setup-persist` `oneshot`, while the Docker-enabled `sbx` template brings its own init that auto-starts `dockerd`. Rather than make two init systems coexist, this change **removes the in-container app services and s6-overlay entirely** — project-owned `docker compose` inside the sandbox is the path for app dependencies (spike-3 recommendations #4/#5) — and re-homes the credential-wiring `setup-persist` oneshot onto a boot hook under the sbx template's init (D2a/D3).

This change defines a dedicated Dune sbx template image that closes these gaps. It is the foundation the runtime backend (`sbx-3-sbx-runtime-backend`) launches. Scope is the **template image only** — no CLI, network/secrets, diagnostics, or kit work.

## Goals / Non-Goals

**Goals**
- Produce a Docker-enabled Dune sbx template: Docker Engine + `docker compose` usable inside the sandbox.
- Reach toolchain parity with the current base image (agent CLIs, Rally, Laps, Playwright, language toolchains), verified from inside a sandbox.
- Remove the in-container Postgres/Redis/Mailpit services and the s6-overlay supervisor, and re-home the `setup-persist` credential-wiring oneshot onto a boot hook so persistence still initialises under the sbx template's native init.
- Define a canonical in-container Dune setup/runtime log path that `dune logs` (`sbx-5`) can read.
- Support a deliberate `/workspace` symlink to the real mounted repo rather than a misleading static directory.
- Define a build, version, and publish path for the template, kept in lockstep with the CLI version.
- Meet the spike-2 acceptance criteria.

**Non-Goals**
- Go CLI runtime / command mapping (`sbx-3`).
- Network policy baseline, domain-opening UX, `sbx secret` wiring (`sbx-4`).
- `dune doctor`, diagnostics taxonomy, `dune logs` reimplementation (`sbx-5`).
- `sbx` kits and removal of the legacy Compose base image (`sbx-6`).
- Clone-mode / copy-in-out workspace models.

## Decisions

### D1: Provide Docker via the Docker-enabled template family — derive path confirmed
The template builds on Docker's Docker-enabled shell template. **Spike 4 resolved the derive question**: `docker/sandbox-templates:shell-docker` **is pullable from Docker Hub as a normal image** (contrary to the spike-2 probe note) and works as a `FROM` base. It is Ubuntu 26.04 LTS + `docker-ce`/`containerd.io`, `ENTRYPOINT ["tini","--"]`, `USER agent` (uid 1000, passwordless sudo, `docker` group — matching Dune's user), and the OCI label **`com.docker.sandboxes.start-docker=true`** is what makes the sbx runtime auto-start `dockerd`; the label (and flavor detection — `sbx template ls` shows the derived image as flavor `shell-docker`) is inherited by `FROM`. The spike-4 merged template (`spike-reports/spike-4-artifacts/Dockerfile.sbx-spike`) built on this base passed the full D8 matrix. The install-Docker-directly fallback is retained only as a contingency for base-image drift.

One toolchain caveat from the Ubuntu 26.04 base: **no stable Playwright supports `ubuntu26.04` yet** (1.58.2 and 1.60.0 both hard-fail). Spike 4 installed Playwright 1.60.0 with the documented `PLAYWRIGHT_HOST_PLATFORM_OVERRIDE=ubuntu24.04-<arch>` escape hatch, and Chromium 148 launched fine in-sandbox; the 1.61 alphas add native ubuntu26.04 mappings, so the override can be dropped when 1.61 is stable.

**Ubuntu-24 base as a transition path — investigated and rejected.** A mini-spike (2026-06-15) checked whether deriving from an older, Ubuntu-24-based Docker template would avoid the Playwright override entirely until 1.61 ships. It does not exist as a usable option: every current `docker/sandbox-templates` flavor — including `shell-docker` and its `shell`/`claude-code`/`codex`/… siblings — was rebuilt 2026-06-04 on the same Ubuntu 25.10/26.04 base ([Docker's docs](https://docs.docker.com/ai/sandboxes/customize/templates/) state Ubuntu 25.10; the image's apt suite resolves to 26.04), and there is **no OS-version-pinned tag** — OS version tracks flavor freshness, not a separate axis. The only pre-26 tags (`latest`, `0.1.0` from 2025-09; `nightly`, `ubuntu-python` from 2025-11) predate the `-docker` flavor split, so they ship **no Docker Engine and no `com.docker.sandboxes.start-docker` label** — the exact capabilities D1 depends on. Pinning to one would be an ~8-month tooling regression that breaks the nested-Docker requirement, not a smoother bridge. Decision stands: derive from `shell-docker` (26.04) and carry the `PLAYWRIGHT_HOST_PLATFORM_OVERRIDE=ubuntu24.04-<arch>` override until Playwright 1.61 is stable.

### D2: A new template build, separate from the Compose base image
Add new template build inputs (e.g. a `Dockerfile.sbx` / `container/sbx/` tree) rather than mutating the root `Dockerfile` in place, so the existing Compose base image stays buildable as migration scaffolding until `sbx-3` cuts over. Reuse the existing build assets where possible: `container/base/scripts/{install-rally,install-laps,install-agy,configure-agents,setup-persist}.sh`, `container/base/tooling.yaml`, and `container/base/home-defaults/`. The `container/base/s6-overlay/s6-rc.d/` service tree and the Postgres/Redis/Mailpit installs are **not** carried into the sbx template (D2a/D3); only the `setup-persist` wiring is retained, re-homed off s6. The legacy base image and its workflow are retired later in `sbx-6`.

### D2a: Drop s6-overlay; the sbx Docker init owns PID 1; re-home `setup-persist`
The current base image uses **s6-overlay** as PID 1 (`ENTRYPOINT ["/init"]`) to supervise `postgres`, `redis`, and `mailpit` (`longrun`) plus a `setup-persist` `oneshot`. The Docker-enabled `sbx` template auto-starts `dockerd` from its own init. Rather than make two init systems coexist, the Dune sbx template **removes s6-overlay and the three app services** (D3) and lets the sbx template's native Docker init be PID 1. The only s6 unit that is not an app service — `setup-persist`, which seeds and symlinks agent credentials/config into `/persist/agent` (`~/.claude → /persist/agent/.claude`, etc.) and is mandatory for the D3 persistence story — is re-homed onto a one-time boot hook. **Spike 4 resolved the mechanism**:
- A native create/boot hook **does exist, but only via kits** (experimental): a kit's `commands.install` runs exactly once at sandbox creation and `commands.startup` runs once per boot (both verified empirically). Because `sbx kit` is marked experimental and only fires when the creator passes `--kit`, persist wiring must not *depend* on it.
- **Chosen: the login-shell sentinel, baked into the template** (self-contained; verified in spike 4). Hook body at `/etc/dune/setup-persist-hook.sh`, sourced from `/etc/profile.d/dune-setup-persist.sh` (bash/sh login shells) and `/etc/zsh/zprofile` (zsh login shells); guarded by a per-sandbox sentinel (`~/.dune-setup-persist-done`, in the non-persisted home so recreation re-runs it); writes timestamped start/done lines plus script output to `/var/log/dune/setup-persist.log` (D5a) — the explicit log lines matter because `setup-persist.sh` is silent on success.
- **Build-time pitfall (hit in spike 4):** the image build's own `bash -lc` steps are login shells and will fire the hook during `docker build`, baking the sentinel into the image so fresh sandboxes never run it. The template build must guard against this (spike 4 used a `DUNE_SETUP_PERSIST_SKIP` env guard on build `RUN` steps plus a final `rm -f` of sentinel and log).
- The backend can force the hook deterministically post-create with one non-interactive `sbx exec <name> bash -lc true`; plain `bash -c` execs do not source profile files and do not fire it.

This removes the highest-risk integration point in the original plan (two competing supervisors) and replaces it with a single idempotent boot step. The acceptance gate (D8) becomes "on a fresh sandbox, the persist symlinks resolve into `/persist/agent` and the agent home is wired," not an s6/dockerd coexistence check.

### D3: In-template Postgres/Redis/Mailpit are removed; app deps come from project Compose
Docker-in-sandbox (D1) makes project-owned `docker compose` the canonical path for app dependencies (spike-3 recommendations #4/#5). The Dune sbx template therefore **does not ship Postgres, Redis, or Mailpit** — their packages, s6 service scripts, and the build steps that install/initialise them (`postgresql`, `redis-server`, `mailpit`, the `initdb` step, the `/var/run/postgresql` runtime-dir handling) are dropped from the sbx template build. The previously planned Postgres runtime-directory fix is moot and removed with them.
- App-dependency lifecycle, readiness, and logs belong to project-owned `docker compose` inside the sandbox.
- No service-native health probes (`pg_isready`, `redis-cli ping`, Mailpit HTTP) exist in the template, and `sbx-5` doctor/logs depend on none.

### D4: `/workspace` is a compatibility symlink to the real mount, created at runtime
The template must not ship a static empty `/workspace` that masks the mount; the current root `Dockerfile`'s `WORKDIR /workspace` (line 218) and the `chown ... /workspace` are removed for the sbx template. The canonical working directory is the real mounted repo path. A first-run/login step creates/refreshes `/workspace` as a symlink to the resolved mount, reproducing the spike-3 manual fix (`ln -s <absolute mount path> /workspace`). Because only the runtime backend knows the absolute mount path, the backend (`sbx-3`) supplies it; the template consumes it to create/refresh the symlink. The template's responsibility here is to *support* the symlink mechanism and remove the misleading `WORKDIR /workspace` assumption.

The mount path is delivered to the template via the `DUNE_WORKSPACE` environment variable, which is the contract `sbx-3` D4 commits to. **Landed sbx-2 contract:** after `sbx create --name <name> --template <ref> shell <absolute mount path>`, the backend runs the deterministic hook-firing login shell as `sbx exec -e DUNE_WORKSPACE=<absolute mount path> <name> bash -lc true`. The login-shell hook consumes `DUNE_WORKSPACE`, resolves it to a physical path, creates or refreshes `/workspace` as a symlink to that target, then runs the sentinel-guarded `setup-persist` step. Later execs/user shells do not need to carry `DUNE_WORKSPACE`; the symlink remains in the sandbox filesystem until refreshed.

Spike 4 verified why this is the contract: `sbx create` has **no `--env`/`-e` flag** (neither does `sbx run`), while `sbx exec -e` / `--env-file` are available and match the post-create hook trigger the backend already needs for D2a. A Dune-generated per-sandbox kit passed at create (`sbx create --kit <dir> …`) whose `environment.variables` carries `DUNE_WORKSPACE` remains the create-time option if the experimental kit surface is accepted later; spike 4 verified kit env vars reach non-login execs and survive stop/start. In-sandbox derivation is intentionally not the sbx-3 contract because it must disambiguate the primary rw virtiofs workspace from the profile-persist workspace.

### D5: Daemon-start-on-exec preserved
Whatever Docker provisioning path D1 takes, the template must retain automatic `dockerd` startup so `docker`/`docker compose` work in the first `sbx exec` without a manual daemon start, matching the generic `-docker` template behavior.

### D5a: Canonical in-container Dune log path
With no top-level `sbx logs` command and no in-container app services, `dune logs` (`sbx-5`) composes Dune-owned setup/runtime output plus `sbx policy log`. The template owns where the Dune-owned output lands: the re-homed `setup-persist` boot hook (D2a) and any template entrypoint work write to a fixed in-container path — `/var/log/dune/` (e.g. `/var/log/dune/setup-persist.log`). `sbx-5`'s `dune logs` reads this path via `sbx exec`. This is the **verified contract** sbx-2 hands to sbx-5, replacing the previously dangling "a path `sbx-2` records" reference.

### D6: Versioning, build, and publish
The template is published as its own image reference (proposed `ghcr.io/mitchell-wallace/dune-sbx:<version>`) and is pullable directly by `sbx create --template <ref>` (spike 1/2 used a GHCR ref as a custom template successfully). For private/non-Docker-Hub registries, document `sbx secret set --registry`. `sbx template load` is supported for local/offline use. Keep the CLI version and template image version in lockstep, extending the existing `VERSION` + `container/base/IMAGE_VERSION` convention (a `container/sbx/IMAGE_VERSION` or reuse of the shared version). The image-publish workflow (`image.yml`) gains a build-and-push job for the template.

### D7: The template is not a secret boundary
Document that `sbx template save` of a configured sandbox captures the whole filesystem (including any manually stored secrets). The Dune template is always built from source, never from a sandbox snapshot, and no secrets are baked in.

### D8: Acceptance is the spike-2 success matrix, with in-container app services removed
Parity is defined by reproducing the spike-2 criteria that actually gate the template, run as smoke checks against the installed `sbx` binary using the exact command forms the spikes proved (sandbox name is **`--name`** on create and **positional** on `sbx ports`; sandbox scope is **`--sandbox`** on policy):

1. Sandbox starts from the Dune template: `sbx create --name <name> --template <ref> shell <absolute mount path>` succeeds and appears in `sbx ls --json`.
2. Attached shell starts in the mounted repo (D4): `readlink -f /workspace` resolves to the mount and `git -C /workspace rev-parse --show-toplevel` succeeds.
3. **Persistence wiring (D2a/D3):** on a fresh sandbox the re-homed `setup-persist` boot hook has run — the agent-home symlinks resolve into `/persist/agent` (e.g. `readlink ~/.claude` → `/persist/agent/.claude`) and its output is present at `/var/log/dune/setup-persist.log` (D5a) — and `dockerd` responds to `docker version` (spike baseline `29.5.3`), with the sbx init not crash-looping.
4. Docker Engine + `docker compose version` work (spike baseline Compose `v5.1.4`); `docker run --rm hello-world` succeeds.
5. Nested `docker compose up -d` works and the service is reachable from inside the sandbox (the canonical path for app dependencies, now that in-container PG/Redis/Mailpit are removed).
6. Playwright launches Chromium.
7. `sbx ports <name> --publish <port> --json` exposes a nested service bound to all sandbox interfaces and the host can reach the published port (spike-2 Experiment 2 showed loopback-only binds are **not** reachable through publish; the nested service used for this check must bind `0.0.0.0`).
8. `sbx policy deny network --sandbox <name> example.com` blocks both shell (`forward`) and nested-Docker (`transparent`) traffic, with the blocked records visible in `sbx policy log <name> --limit <n>`.

The template ships no in-container app services; app-dependency readiness is out of scope and is validated only via the nested `docker compose` smoke check (#5).

## Risks / Trade-offs

- **Docker-in-sandbox cost and build time.** Nested Docker adds resource overhead; the full image build is large (~12–15 min per the project notes). Mitigation: validate with an ephemeral build; keep layers cache-friendly.
- ~~`-docker` template not derivable as a normal image~~ — **retired by spike 4**: `docker/sandbox-templates:shell-docker` pulls and works as a `FROM` base; the spike-4 merged template built on it passed the D8 matrix. Residual risk is base-image drift (tracked by pinning a digest or re-verifying on bumps). A new risk takes its place: **the base is Ubuntu 26.04, which no stable Playwright supports yet** — mitigated via `PLAYWRIGHT_HOST_PLATFORM_OVERRIDE=ubuntu24.04-<arch>` (Chromium launch verified in-sandbox) until Playwright 1.61 is stable. A 2026-06-15 mini-spike confirmed there is **no maintained Ubuntu-24 `-docker` template** to derive from as a stopgap (all current flavors are rebuilt on 25.10/26.04; pre-26 tags predate the Docker-enabled flavor), so the override is the bridge, not a base swap — see D1.
- ~~Playwright/Chromium under microVM~~ — **verified by spike 4**: Chromium 148 (ubuntu24.04 fallback build) launched and rendered a page inside a sandbox from the merged template.
- **`/workspace` symlink timing.** The symlink must be created after the mount is present and before the user's shell relies on it; `sbx-3` does this with the post-create hook exec carrying `DUNE_WORKSPACE`.
- **Hard cut of in-container app services.** Removing Postgres/Redis/Mailpit and s6-overlay is a breaking change: projects that relied on auto-started in-container services must now bring their own `docker compose` inside the sandbox. This is deliberate (spike-3 #4/#5) and eliminates the dual-init risk; the trade-off is a migration burden that must be documented for users.
- **`setup-persist` must run under the new init.** Persistence depends on the re-homed boot hook firing on every fresh sandbox. Spike 4 settled the mechanism (login-shell sentinel, D2a) and surfaced the build-time pitfall (login shells during `docker build` firing the hook — guard required). Residual: the hook only fires on a *login* shell, so the backend should force it post-create with one `bash -lc` exec rather than relying on the user's first attach.

## Migration Plan

1. Author/build the Dune sbx template alongside the existing Compose base image (no removal yet).
2. Validate against the D8 acceptance matrix in an ephemeral sandbox.
3. Publish the template image and wire the publish workflow.
4. `sbx-3` consumes the template ref; the legacy Compose base image and its workflow are retired in `sbx-6` once parity holds.

## Open Questions

- ~~Derive from a Docker-enabled `-docker` template vs. install Docker Engine directly~~ — **resolved by spike 4**: derive (`FROM docker/sandbox-templates:shell-docker`), see D1.
- ~~Where the re-homed `setup-persist` runs under the sbx init~~ — **resolved by spike 4**: login-shell sentinel baked into the template; kits offer a native create hook but are experimental and opt-in (D2a).
- Final template image name and version scheme (dedicated `dune-sbx` repo vs. shared versioning).
- ~~Whether the installed `sbx` supports create-time env injection for `DUNE_WORKSPACE`~~ — **resolved by spike 4**: no `--env` on create; delivery is exec-time `-e`, a Dune-generated kit's `environment.variables` (experimental), or in-sandbox derivation (D4). 1.1/3.2 record which one lands as the sbx-3 contract.
