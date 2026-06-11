## Context

Dune's current runtime artifact is the Compose-oriented base image built by the root `Dockerfile` and published as `ghcr.io/mitchell-wallace/dune-base` (`container/base/IMAGE_VERSION` = `0.4.2`). It is consumed by `internal/dune/app.go`, which starts an `agent` container plus a `pipelock` sidecar via Docker Compose.

The `sbx-1-runtime-spike` reports validated that this image can be launched as an `sbx` `--template`, retaining Dune's tooling (Rally, Laps, agent CLIs, Playwright, Redis, Mailpit, language toolchains) and its persistence symlinks. But three concrete gaps make it unsuitable as the final sbx artifact:

- **No Docker Engine / Compose inside the sandbox.** The generic `sbx` Docker-enabled `shell` template (`docker/sandbox-templates:shell-docker`) proves Docker-in-sbx works — `dockerd` starts automatically on `sbx exec`, `docker compose version` reports `v5.1.4`, nested `docker compose up` works, and `sbx ports` can publish a nested service. The Dune base image lacks all of this.
- **Misleading `/workspace`.** The image sets `WORKDIR /workspace`, but under direct-mount `sbx` the repository is visible at its absolute host path, and `/workspace` exists yet is *not* the repo. Spike 3 confirmed a `/workspace` symlink to the mounted path resolves correctly.
- **Postgres fails to start.** `/var/run/postgresql` is absent at sandbox runtime (`FATAL: could not create lock file ".../.s.PGSQL.5432.lock"`). Creating and chowning it at service start fixes it; image-build-time creation is not reliable under `sbx`.

This change defines a dedicated Dune sbx template image that closes these gaps. It is the foundation the runtime backend (`sbx-3-sbx-runtime-backend`) launches. Scope is the **template image only** — no CLI, network/secrets, diagnostics, or kit work.

## Goals / Non-Goals

**Goals**
- Produce a Docker-enabled Dune sbx template: Docker Engine + `docker compose` usable inside the sandbox.
- Reach toolchain parity with the current base image, verified from inside a sandbox.
- Keep the in-template service tools (Postgres, Redis, Mailpit) installed and apply the low-cost Postgres runtime-directory fix so they are *usable when started* — without making service auto-start, health probes, or log aggregation part of template readiness (in-sandbox `docker compose` makes project-owned Compose the canonical path for app dependencies).
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
Add new template build inputs (e.g. a `Dockerfile.sbx` / `container/sbx/` tree) rather than mutating the root `Dockerfile` in place, so the existing Compose base image stays buildable as migration scaffolding until `sbx-3` cuts over. Reuse the existing build assets where possible: `container/base/scripts/{install-rally,install-laps,install-agy,configure-agents,setup-persist}.sh`, `container/base/tooling.yaml`, the `container/base/s6-overlay/s6-rc.d/` service tree, and `container/base/home-defaults/`. The legacy base image and its workflow are retired later in `sbx-6`.

### D2a: s6-overlay and the Docker daemon must coexist as PID 1 / init
The current base image uses **s6-overlay** (`container/base/s6-overlay/s6-rc.d/`) as its init system and runs `postgres`, `redis`, and `mailpit` as s6 `longrun` services plus a `setup-persist` `oneshot`. The generic Docker-enabled `-docker` template auto-starts `dockerd` on the first `sbx exec` through its own entrypoint/init. These two init mechanisms must not conflict in the merged Dune template, and exactly one of the two integration shapes below must be chosen and recorded during 1.1:
- **Extend the `-docker` base (D1 preferred path):** the `-docker` template's daemon-start mechanism owns dockerd; the Dune s6 service tree is layered on top and must be launched by whatever init the `-docker` base runs (or s6-overlay is installed as a subordinate supervisor invoked from that init). Verify both `dockerd` and the s6 services come up in one `sbx exec`.
- **Install Docker into the s6 image (D1 fallback path):** s6-overlay remains PID 1 and gains a `docker` (`dockerd`) `longrun` service alongside the existing services, reproducing daemon-start-on-exec via s6 rather than the `-docker` entrypoint.

This coexistence is the highest-risk integration point in this change and is gated by a dedicated acceptance check (D8): a single `sbx exec` into a fresh sandbox must show `dockerd` responding to `docker version` **and** the s6 service tree present (e.g. the supervised services are visible to the active supervisor), with neither init mechanism crash-looping.

### D3: In-template services are convenience tools only; no health-probe scope
Docker-in-sandbox (D1) makes project-owned `docker compose` the canonical path for app dependencies, so the in-template Postgres/Redis/Mailpit services are convenience tools, not readiness gates. The template still ships them and fixes the known broken Postgres startup path:
- The Postgres s6 `run` script (`container/base/s6-overlay/s6-rc.d/postgres/run`, currently `exec "${pg_bin}" -D /var/lib/postgresql/data -k /var/run/postgresql`) is updated to create and chown the runtime directory at service start — e.g. `install -d -o agent -g agent -m 0755 /var/run/postgresql` immediately before the `exec` — rather than relying on the image-build-time `install -d ... /var/run/postgresql` in the root `Dockerfile`, which the spike showed does not survive into the `sbx` runtime. This is the exact fix validated in spike-2 Experiment 4 and is worth doing so an installed-but-broken Postgres does not ship.
- Redis and Mailpit remain installed with their existing scripts, but their supervision, auto-start, health probes, and log aggregation are outside this change.
- No service-native health probes (`pg_isready`, `redis-cli ping`, Mailpit HTTP) are added to the template acceptance gate. `sbx-5` doctor/logs should not depend on in-template service health; app-dependency readiness belongs to project-owned Compose inside the sandbox.

### D4: `/workspace` is a compatibility symlink to the real mount, created at runtime
The template must not ship a static empty `/workspace` that masks the mount; the current root `Dockerfile`'s `WORKDIR /workspace` (line 218) and the `chown ... /workspace` are removed for the sbx template. The canonical working directory is the real mounted repo path. A first-run/login step creates/refreshes `/workspace` as a symlink to the resolved mount, reproducing the spike-3 manual fix (`ln -s <absolute mount path> /workspace`). Because only the runtime backend knows the absolute mount path, the backend (`sbx-3`) supplies it; the template consumes it to create/refresh the symlink. The template's responsibility here is to *support* the symlink mechanism and remove the misleading `WORKDIR /workspace` assumption.

The mount path is delivered to the template via the `DUNE_WORKSPACE` environment variable, which is the contract `sbx-3` D4 commits to. **The env-injection mechanism itself is not yet proven against the `sbx` binary** (the spikes mounted the repo but never exercised env injection at create time), so sbx-2 must verify, against the installed `sbx`, *how* an env var reaches the sandbox before depending on it:
- Determine the supported create-time env mechanism on the installed `sbx` (e.g. `sbx create --help` for an `--env`/`-e` flag; otherwise an in-sandbox login-shell that derives the mount path). Record the exact flag/mechanism in 1.1/3.2.
- If create-time env injection is supported, standalone verification uses it: `sbx create --name <name> -e DUNE_WORKSPACE=<absolute mount path> --template <ref> shell <absolute mount path>`.
- If it is **not** supported, the template falls back to deriving the mount path inside the sandbox at login (the repo is visible at its absolute host path), and the recorded outcome flags that `sbx-3` D4 must adopt the same in-sandbox derivation rather than an env-var contract. Either way, sbx-2 produces a *verified* contract for sbx-3 rather than an assumed one.

### D5: Daemon-start-on-exec preserved
Whatever Docker provisioning path D1 takes, the template must retain automatic `dockerd` startup so `docker`/`docker compose` work in the first `sbx exec` without a manual daemon start, matching the generic `-docker` template behavior.

### D6: Versioning, build, and publish
The template is published as its own image reference (proposed `ghcr.io/mitchell-wallace/dune-sbx:<version>`) and is pullable directly by `sbx create --template <ref>` (spike 1/2 used a GHCR ref as a custom template successfully). For private/non-Docker-Hub registries, document `sbx secret set --registry`. `sbx template load` is supported for local/offline use. Keep the CLI version and template image version in lockstep, extending the existing `VERSION` + `container/base/IMAGE_VERSION` convention (a `container/sbx/IMAGE_VERSION` or reuse of the shared version). The image-publish workflow (`image.yml`) gains a build-and-push job for the template.

### D7: The template is not a secret boundary
Document that `sbx template save` of a configured sandbox captures the whole filesystem (including any manually stored secrets). The Dune template is always built from source, never from a sandbox snapshot, and no secrets are baked in.

### D8: Acceptance is the spike-2 success matrix, with health probes explicitly excluded
Parity is defined by reproducing the spike-2 criteria that actually gate the template, run as smoke checks against the installed `sbx` binary using the exact command forms the spikes proved (sandbox name is **`--name`** on create and **positional** on `sbx ports`; sandbox scope is **`--sandbox`** on policy):

1. Sandbox starts from the Dune template: `sbx create --name <name> --template <ref> shell <absolute mount path>` succeeds and appears in `sbx ls --json`.
2. Attached shell starts in the mounted repo (D4): `readlink -f /workspace` resolves to the mount and `git -C /workspace rev-parse --show-toplevel` succeeds.
3. **s6 + dockerd coexistence (D2a):** a single `sbx exec <name> bash -lc '...'` shows `docker version` succeeding (engine reported, spike baseline `29.5.3`) **and** the s6 service tree present, with neither init mechanism crash-looping.
4. Docker Engine + `docker compose version` work (spike baseline Compose `v5.1.4`); `docker run --rm hello-world` succeeds.
5. Nested `docker compose up -d` works and the service is reachable from inside the sandbox (the canonical path for app dependencies).
6. Playwright launches Chromium.
7. `sbx ports <name> --publish <port> --json` exposes a nested service bound to all sandbox interfaces and the host can reach the published port (spike-2 Experiment 2 showed loopback-only binds are **not** reachable through publish; the nested service used for this check must bind `0.0.0.0`).
8. `sbx policy deny network --sandbox <name> example.com` blocks both shell (`forward`) and nested-Docker (`transparent`) traffic, with the blocked records visible in `sbx policy log <name> --limit <n>`.

The in-template service tools are present and the Postgres runtime-directory fix is verified (non-gating), but service health probes are deliberately excluded from acceptance.

## Risks / Trade-offs

- **Docker-in-sandbox cost and build time.** Nested Docker adds resource overhead; the full image build is large (~12–15 min per the project notes). Mitigation: validate with an ephemeral build; keep layers cache-friendly.
- **`-docker` template not derivable as a normal image.** If D1's derive path is impossible, fall back to installing Docker CE + compose plugin (more maintenance). Acceptance tests gate both.
- **Playwright/Chromium under microVM.** Browser launch must be verified in-sandbox, not assumed.
- **`/workspace` symlink timing.** The symlink must be created after the mount is present and before the user's shell relies on it; coordinate the create-time variable with `sbx-3`.
- **Service ownership drift.** Spike 3 recommended that app dependencies (PG/Redis/Mailpit) move toward project-owned Compose now that `docker compose` runs inside the sandbox. This change therefore keeps the tools installed and fixes known-broken Postgres startup, but does not create a second service-management surface in the template. Users needing reliable app dependency lifecycle should use project-owned Compose inside the sandbox.

## Migration Plan

1. Author/build the Dune sbx template alongside the existing Compose base image (no removal yet).
2. Validate against the D8 acceptance matrix in an ephemeral sandbox.
3. Publish the template image and wire the publish workflow.
4. `sbx-3` consumes the template ref; the legacy Compose base image and its workflow are retired in `sbx-6` once parity holds.

## Open Questions

- Derive from a Docker-enabled `-docker` template vs. install Docker Engine directly (resolve during 1.1; the chosen path also fixes the s6/dockerd coexistence shape per D2a).
- Final template image name and version scheme (dedicated `dune-sbx` repo vs. shared versioning).
- Whether the installed `sbx` supports create-time env injection for `DUNE_WORKSPACE` (D4). This is no longer assumed: 1.1/3.2 verify it against the binary and record either the env-injection flag or the in-sandbox-derivation fallback as the contract handed to `sbx-3`.
