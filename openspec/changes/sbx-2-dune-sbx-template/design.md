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
- Keep the in-template service tools (Postgres, Redis, Mailpit) installed and apply the low-cost Postgres runtime-directory fix so they are *usable when started* — without making their auto-start/health a parity gate (in-sandbox `docker compose` makes project-owned Compose the canonical path for app dependencies).
- Support a deliberate `/workspace` symlink to the real mounted repo rather than a misleading static directory.
- Define lightweight, optional service health and log conventions later consumable by `dune logs` / `dune doctor`.
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
Add new template build inputs (e.g. a `Dockerfile.sbx` / `container/sbx/` tree) rather than mutating the root `Dockerfile` in place, so the existing Compose base image stays buildable as migration scaffolding until `sbx-3` cuts over. Reuse the existing build assets where possible: `container/base/scripts/{install-rally,install-laps,install-agy,configure-agents,setup-persist}.sh`, `container/base/tooling.yaml`, and `container/base/home-defaults/`. The legacy base image and its workflow are retired later in `sbx-6`.

### D3: In-template services are a convenience, with a low-cost Postgres fix
Docker-in-sandbox (D1) makes project-owned `docker compose` the canonical path for app dependencies, so the in-template Postgres/Redis/Mailpit services are demoted from parity-critical to convenience. The template still ships them and keeps them usable:
- The Postgres service creates and chowns `/var/run/postgresql` (owner `agent`) before launching `postgres -D /var/lib/postgresql/data -k /var/run/postgresql`. This is a cheap, already-diagnosed fix and is worth doing so an installed-but-broken Postgres does not ship.
- Redis and Mailpit remain installed and start with their existing scripts.
- Service-native health probes (`pg_isready`, `redis-cli ping`, Mailpit HTTP) are provided but **optional** (spike 2 noted `s6-svstat` was unavailable, so probes do not assume an s6 status tool). In-template service auto-start/health is best-effort and is explicitly **not** part of this change's acceptance gate.
- Whether these services remain supervised in-template long-term, or are dropped in favor of project-owned Compose, is tracked for `sbx-6`.

### D4: `/workspace` is a compatibility symlink to the real mount, created at runtime
The template must not ship a static empty `/workspace` that masks the mount. The canonical working directory is the real mounted repo path. A first-run/login step creates `/workspace` as a symlink to the resolved mount. Because only the runtime backend knows the absolute mount path, the backend (`sbx-3`) supplies it (e.g. via an environment variable such as `DUNE_WORKSPACE` at sandbox create time); the template consumes it to create/refresh the symlink. The template's responsibility here is to *support* the symlink mechanism and remove the misleading `WORKDIR /workspace` assumption.

Because there is no Dune backend yet in this change, sbx-2's standalone verification supplies the mount path manually (e.g. `sbx create ... -e DUNE_WORKSPACE=<absolute mount path>`) so the symlink behavior can be tested without `sbx-3`. The env var name is the contract `sbx-3` D4 commits to.

### D5: Daemon-start-on-exec preserved
Whatever Docker provisioning path D1 takes, the template must retain automatic `dockerd` startup so `docker`/`docker compose` work in the first `sbx exec` without a manual daemon start, matching the generic `-docker` template behavior.

### D6: Versioning, build, and publish
The template is published as its own image reference (proposed `ghcr.io/mitchell-wallace/dune-sbx:<version>`) and is pullable directly by `sbx create --template <ref>` (spike 1/2 used a GHCR ref as a custom template successfully). For private/non-Docker-Hub registries, document `sbx secret set --registry`. `sbx template load` is supported for local/offline use. Keep the CLI version and template image version in lockstep, extending the existing `VERSION` + `container/base/IMAGE_VERSION` convention (a `container/sbx/IMAGE_VERSION` or reuse of the shared version). The image-publish workflow (`image.yml`) gains a build-and-push job for the template.

### D7: The template is not a secret boundary
Document that `sbx template save` of a configured sandbox captures the whole filesystem (including any manually stored secrets). The Dune template is always built from source, never from a sandbox snapshot, and no secrets are baked in.

### D8: Acceptance is the spike-2 success matrix (minus the service-health gate)
Parity is defined by reproducing the spike-2 criteria that actually gate the template, as smoke checks: sandbox starts from the Dune template; attached shell starts in the mounted repo; Docker Engine + `docker compose version` work; nested `docker compose up` works (the canonical path for app dependencies); Playwright launches Chromium; `sbx ports` exposes a nested service to the host; `sbx policy deny network --sandbox <name> example.com` blocks both shell and nested-Docker traffic with useful `sbx policy log` records. The in-template service tools are present and Postgres starts cleanly when invoked, but their health is a convenience check rather than a blocking acceptance criterion.

## Risks / Trade-offs

- **Docker-in-sandbox cost and build time.** Nested Docker adds resource overhead; the full image build is large (~12–15 min per the project notes). Mitigation: validate with an ephemeral build; keep layers cache-friendly.
- **`-docker` template not derivable as a normal image.** If D1's derive path is impossible, fall back to installing Docker CE + compose plugin (more maintenance). Acceptance tests gate both.
- **Supervision under sbx.** s6-overlay must run as the template's init under `sbx`, or services must move to an sbx-compatible start mechanism. Mitigation: verify s6 supervises correctly inside a sandbox; if not, adopt a sandbox-native start path for the three services.
- **Playwright/Chromium under microVM.** Browser launch must be verified in-sandbox, not assumed.
- **`/workspace` symlink timing.** The symlink must be created after the mount is present and before the user's shell relies on it; coordinate the create-time variable with `sbx-3`.
- **Service ownership drift.** Spike 3 recommended that app dependencies (PG/Redis/Mailpit) move toward project-owned Compose now that `docker compose` runs inside the sandbox. This change therefore keeps them installed as a convenience but does not gate on their health; the longer-term decision (keep supervised in-template vs. drop in favor of project Compose) is tracked in `sbx-6`. The trade-off is that a user relying on the in-template services gets a best-effort experience until that decision lands.

## Migration Plan

1. Author/build the Dune sbx template alongside the existing Compose base image (no removal yet).
2. Validate against the D8 acceptance matrix in an ephemeral sandbox.
3. Publish the template image and wire the publish workflow.
4. `sbx-3` consumes the template ref; the legacy Compose base image and its workflow are retired in `sbx-6` once parity holds.

## Open Questions

- Derive from a Docker-enabled `-docker` template vs. install Docker Engine directly (resolve during build spike).
- Does s6-overlay run cleanly as the sandbox init under `sbx`, or should the three services adopt an sbx-native start mechanism?
- Final template image name and version scheme (dedicated `dune-sbx` repo vs. shared versioning).
- Exact contract for how the backend passes the mount path for the `/workspace` symlink (env var name vs. in-sandbox detection).
- Whether Postgres/Redis/Mailpit remain supervised in-template long-term or move toward project-owned Compose (tracked for `sbx-6`).
