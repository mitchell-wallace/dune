## Why

Dune's runtime substrate is moving from local Docker Compose to the standalone `sbx` microVM sandbox CLI (see `openspec/changes/sbx-1-runtime-spike/spike-reports/`). The three spikes established a decisive direction: hard-cut to an `sbx` backend, drop `Dockerfile.dune` and Pipelock, and make a dedicated Dune **sbx template image** the durable, batteries-included runtime artifact that the new backend launches.

The current base image (root `Dockerfile`, published as `ghcr.io/mitchell-wallace/dune-base`, `container/base/IMAGE_VERSION` = `0.4.2`) was shaped for the Docker Compose `agent` + `pipelock` topology. The spikes confirmed it can be launched as an `sbx` `--template` and still carries Dune's tooling and persistence symlinks, but it is **not** the right final sbx artifact:

- It has **no Docker Engine** and therefore no `docker compose` inside the sandbox, while the generic Docker-enabled `shell` template proves Docker-in-sbx (including nested Compose, `docker run`, image builds, and volumes) works.
- Its `WORKDIR /workspace` is actively misleading under direct-mount sbx: `/workspace` exists but is not the mounted repository, which lives at the absolute host path inside the sandbox.
- Postgres fails to start under sbx because `/var/run/postgresql` is not present at runtime (`FATAL: could not create lock file ".../.s.PGSQL.5432.lock"`); the runtime directory must be created and chowned at service start, not only at image-build time.

Crucially, because the generic Docker-enabled template proves `docker compose` works **inside** the sandbox, the in-template Postgres/Redis/Mailpit services are no longer the parity-critical path they were under the Compose topology: a project can own its app dependencies via project-owned `docker compose` inside the sandbox. The Dune template keeps these tools installed as a convenience and applies the cheap Postgres runtime-directory fix, but service auto-start, health probes, and log aggregation are explicitly **out of scope** for this change.

This change defines and builds that dedicated Dune sbx template so the runtime backend work (`sbx-3-sbx-runtime-backend`) has a concrete, parity-tested image to target. It is the foundation of the sbx migration: a heavy template now, with thin `sbx` kits as the later customization layer.

This proposal scopes the **template image only**. It does not change the Go CLI, the runtime command mapping, the network/secrets posture, or remove Pipelock — those are `sbx-3`, `sbx-4`, and `sbx-5`. It supersedes the base-image assumptions baked into the `ref-*` drafts, which were written for a Docker Compose future.

## What Changes

- **NEW** A dedicated Dune sbx template image (the durable runtime artifact) that is Docker-enabled and reproduces Dune's batteries-included environment inside an `sbx` sandbox. It derives from, or replicates the important behavior of, Docker's Docker-enabled (`-docker`) shell sandbox template so Docker Engine and `docker compose` are available inside the sandbox. Because today's base image already uses s6-overlay as init for its services, this requires reconciling s6-overlay with the `-docker` daemon-start mechanism so both come up cleanly (see design D2a).
- **NEW** Keep the in-template app-style service tools (Postgres, Redis, Mailpit) installed as a convenience, with a low-cost runtime-safe Postgres fix so it is usable when started: Postgres startup creates and chowns `/var/run/postgresql` (and any other ephemeral runtime dirs) at service start instead of relying on image-build-time directory creation. With `docker compose` available inside the sandbox, project-owned Compose is the canonical path for app dependencies, so in-template service auto-start, health probes, and service-log aggregation are not part of this change.
- **NEW** Deliberate `/workspace` compatibility handling: the template provides `/workspace` as a symlink to the actual mounted repository path rather than the current static `WORKDIR /workspace`, so legacy `/workspace` assumptions resolve to the real repo. The canonical working directory is the real mounted repo path; `/workspace` is a compatibility bridge only. (The backend in `sbx-3` is responsible for ensuring the symlink targets the resolved mount; this change makes the template support it rather than contradict it.)
- Toolchain parity with today's base image, verified inside the sandbox: zsh + Powerlevel10k defaults, Rally, Laps, the supported agent CLIs (`claude`, `codex`, `opencode`, `gemini`), `openspec`, `gh`/`git` + common CLI tooling, Go/Node/Python/`uv`/`pnpm` via `mise`, and Playwright with a working Chromium launch.
- **NEW** Template versioning, build, and publish path: how the Dune sbx template is built, tagged, published (GHCR), and loaded for local use (`sbx template load`), including registry auth for non-Docker Hub pulls where required (`sbx secret set --registry`). The existing convention of keeping the CLI version and image version in lockstep (`VERSION` + `container/base/IMAGE_VERSION`) is preserved or explicitly evolved for the template.
- Document that an `sbx template save` of a configured sandbox captures the whole filesystem (including any manually stored secrets); the Dune template must be built from source, never from a sandbox snapshot, and is **not** a secret boundary.

### Non-goals (explicitly deferred)

- No changes to the Go CLI runtime, command mapping, or `app.go` (that is `sbx-3-sbx-runtime-backend`).
- No network policy baseline, domain-opening affordance, or `sbx secret` wiring (that is `sbx-4-sbx-network-and-secrets`).
- No removal of Pipelock or the existing Docker Compose code paths (Pipelock removal lands with the sbx backend in `sbx-3`/`sbx-4`).
- No `dune doctor`, diagnostics taxonomy, or `dune logs` reimplementation (that is `sbx-5-sbx-lifecycle-and-doctor`).
- No `sbx` kits (that is `sbx-6-sbx-kits-and-cleanup`).
- No clone-mode or copy-in/copy-out workspace model; v1 assumes direct mount.

## Capabilities

### New Capabilities

- `dune-sbx-template`: A dedicated, Docker-enabled Dune sbx template image that reproduces Dune's batteries-included environment inside an `sbx` sandbox, with a runtime-safe Postgres fix for the (now convenience) in-template services, deliberate `/workspace` compatibility, and a defined build/version/publish path.

## Impact

- **New runtime artifact**: The Dune sbx template becomes the durable equivalent of today's `dune-base` image for the sbx world. The Compose-oriented base image remains only as migration scaffolding until the sbx backend lands and parity is proven.
- **Container build**: New/changed build inputs under `container/` (and likely the root `Dockerfile` or a new template Dockerfile) to add Docker Engine + Compose inside the sandbox and to make service startup runtime-safe. Build time remains large (the spike notes a full base build can take ~12-15 minutes); contributors should validate with an ephemeral build.
- **Services**: The in-template Postgres/Redis/Mailpit tools remain installed; Postgres gains a runtime-directory fix so it is usable when started. Auto-start, health probes, and log aggregation are out of scope: with in-sandbox `docker compose`, app dependencies are expected to move toward project-owned Compose, so in-template service lifecycle is a convenience rather than a template-readiness requirement.
- **Workspace path**: `/workspace` becomes a compatibility symlink to the real mounted repo rather than a static, misleading directory.
- **Versioning/CI**: Template build and publish flow (GHCR) and version lockstep are defined; image-publish automation may need a new or adjusted workflow.
- **User-facing behavior**: None yet. No `dune` command behavior changes in this proposal; the template is consumed by the backend introduced in `sbx-3`.
- **Acceptance**: Centers on the spike-2 success criteria that actually gate the template, run against the installed `sbx` binary with the exact command forms the spikes proved — a sandbox starts from the Dune sbx template (`sbx create --name <name> --template <ref> shell <mount path>`); the attached shell resolves `/workspace` to the mounted repository; the s6 service tree and `dockerd` coexist in a single `sbx exec` without either init crash-looping; Docker Engine and `docker compose version` work; a nested `docker compose up` works (the canonical path for app dependencies); Playwright launches Chromium; `sbx ports <name> --publish <port>` can expose a nested service (bound to all sandbox interfaces) to the host; and `sbx policy deny network --sandbox <name> example.com` blocks both shell and nested-Docker traffic with useful `sbx policy log <name>` records. The in-template service tools (Postgres/Redis/Mailpit) are present and the Postgres runtime-directory fix is verified, but no health-probe acceptance gate is introduced.
