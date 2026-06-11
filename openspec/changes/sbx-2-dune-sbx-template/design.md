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

### D1: Provide Docker via the Docker-enabled template family, with an install fallback
The template targets Docker-in-sandbox by building on (or replicating) Docker's Docker-enabled `-docker` shell template behavior, so `dockerd` is present and auto-starts on exec. Spike 2 confirmed the `-docker` variant works but noted the internal `shell-docker` image was **not** inspectable as a normal host Docker image. Decision: if the Dune template can be expressed as an extension of a Docker-enabled base, do that; otherwise install Docker Engine + the Compose plugin into the Dune image directly and reproduce the daemon-start-on-exec behavior. The acceptance tests (D8) gate either path.

### D2: A new template build, separate from the Compose base image
Add new template build inputs (e.g. a `Dockerfile.sbx` / `container/sbx/` tree) rather than mutating the root `Dockerfile` in place, so the existing Compose base image stays buildable as migration scaffolding until `sbx-3` cuts over. Reuse the existing build assets where possible: `container/base/scripts/{install-rally,install-laps,install-agy,configure-agents,setup-persist}.sh`, `container/base/tooling.yaml`, and `container/base/home-defaults/`. The `container/base/s6-overlay/s6-rc.d/` service tree and the Postgres/Redis/Mailpit installs are **not** carried into the sbx template (D2a/D3); only the `setup-persist` wiring is retained, re-homed off s6. The legacy base image and its workflow are retired later in `sbx-6`.

### D2a: Drop s6-overlay; the sbx Docker init owns PID 1; re-home `setup-persist`
The current base image uses **s6-overlay** as PID 1 (`ENTRYPOINT ["/init"]`) to supervise `postgres`, `redis`, and `mailpit` (`longrun`) plus a `setup-persist` `oneshot`. The Docker-enabled `sbx` template auto-starts `dockerd` from its own init. Rather than make two init systems coexist, the Dune sbx template **removes s6-overlay and the three app services** (D3) and lets the sbx template's native Docker init be PID 1. The only s6 unit that is not an app service — `setup-persist`, which seeds and symlinks agent credentials/config into `/persist/agent` (`~/.claude → /persist/agent/.claude`, etc.) and is mandatory for the D3 persistence story — is re-homed onto a one-time boot hook:
- **Preferred:** an `sbx`-native create/boot hook if one exists. The exact mechanism is unverified against the installed `sbx` and must be resolved in Spike 4 (`sbx create --help` / template-config inspection).
- **Fallback:** a login-shell hook guarded by a per-sandbox sentinel. `setup-persist.sh` is already idempotent (every seed/link is existence-guarded), so running it at first login is safe; the sentinel prevents re-running it on every `exec`.

This removes the highest-risk integration point in the original plan (two competing supervisors) and replaces it with a single idempotent boot step. The acceptance gate (D8) becomes "on a fresh sandbox, the persist symlinks resolve into `/persist/agent` and the agent home is wired," not an s6/dockerd coexistence check.

### D3: In-template Postgres/Redis/Mailpit are removed; app deps come from project Compose
Docker-in-sandbox (D1) makes project-owned `docker compose` the canonical path for app dependencies (spike-3 recommendations #4/#5). The Dune sbx template therefore **does not ship Postgres, Redis, or Mailpit** — their packages, s6 service scripts, and the build steps that install/initialise them (`postgresql`, `redis-server`, `mailpit`, the `initdb` step, the `/var/run/postgresql` runtime-dir handling) are dropped from the sbx template build. The previously planned Postgres runtime-directory fix is moot and removed with them.
- App-dependency lifecycle, readiness, and logs belong to project-owned `docker compose` inside the sandbox.
- No service-native health probes (`pg_isready`, `redis-cli ping`, Mailpit HTTP) exist in the template, and `sbx-5` doctor/logs depend on none.

### D4: `/workspace` is a compatibility symlink to the real mount, created at runtime
The template must not ship a static empty `/workspace` that masks the mount; the current root `Dockerfile`'s `WORKDIR /workspace` (line 218) and the `chown ... /workspace` are removed for the sbx template. The canonical working directory is the real mounted repo path. A first-run/login step creates/refreshes `/workspace` as a symlink to the resolved mount, reproducing the spike-3 manual fix (`ln -s <absolute mount path> /workspace`). Because only the runtime backend knows the absolute mount path, the backend (`sbx-3`) supplies it; the template consumes it to create/refresh the symlink. The template's responsibility here is to *support* the symlink mechanism and remove the misleading `WORKDIR /workspace` assumption.

The mount path is delivered to the template via the `DUNE_WORKSPACE` environment variable, which is the contract `sbx-3` D4 commits to. **The env-injection mechanism itself is not yet proven against the `sbx` binary** (the spikes mounted the repo but never exercised env injection at create time), so sbx-2 must verify, against the installed `sbx`, *how* an env var reaches the sandbox before depending on it:
- Determine the supported create-time env mechanism on the installed `sbx` (e.g. `sbx create --help` for an `--env`/`-e` flag; otherwise an in-sandbox login-shell that derives the mount path). Record the exact flag/mechanism in 1.1/3.2.
- If create-time env injection is supported, standalone verification uses it: `sbx create --name <name> -e DUNE_WORKSPACE=<absolute mount path> --template <ref> shell <absolute mount path>`.
- If it is **not** supported, the template falls back to deriving the mount path inside the sandbox at login (the repo is visible at its absolute host path), and the recorded outcome flags that `sbx-3` D4 must adopt the same in-sandbox derivation rather than an env-var contract. Either way, sbx-2 produces a *verified* contract for sbx-3 rather than an assumed one.

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
- **`-docker` template not derivable as a normal image.** If D1's derive path is impossible, fall back to installing Docker CE + compose plugin (more maintenance). Acceptance tests gate both.
- **Playwright/Chromium under microVM.** Browser launch must be verified in-sandbox, not assumed.
- **`/workspace` symlink timing.** The symlink must be created after the mount is present and before the user's shell relies on it; coordinate the create-time variable with `sbx-3`.
- **Hard cut of in-container app services.** Removing Postgres/Redis/Mailpit and s6-overlay is a breaking change: projects that relied on auto-started in-container services must now bring their own `docker compose` inside the sandbox. This is deliberate (spike-3 #4/#5) and eliminates the dual-init risk; the trade-off is a migration burden that must be documented for users.
- **`setup-persist` must run under the new init.** Persistence depends on the re-homed boot hook firing on every fresh sandbox. Mitigated by the idempotent script plus the D8 persist-wiring gate; the exact hook mechanism (native sbx hook vs login-shell sentinel) is a Spike 4 item (D2a).

## Migration Plan

1. Author/build the Dune sbx template alongside the existing Compose base image (no removal yet).
2. Validate against the D8 acceptance matrix in an ephemeral sandbox.
3. Publish the template image and wire the publish workflow.
4. `sbx-3` consumes the template ref; the legacy Compose base image and its workflow are retired in `sbx-6` once parity holds.

## Open Questions

- Derive from a Docker-enabled `-docker` template vs. install Docker Engine directly (resolve during 1.1).
- Where the re-homed `setup-persist` runs under the sbx init — a native `sbx` create/boot hook vs. a login-shell sentinel (Spike 4, D2a).
- Final template image name and version scheme (dedicated `dune-sbx` repo vs. shared versioning).
- Whether the installed `sbx` supports create-time env injection for `DUNE_WORKSPACE` (D4). This is no longer assumed: 1.1/3.2 verify it against the binary and record either the env-injection flag or the in-sandbox-derivation fallback as the contract handed to `sbx-3`.
