## 1. Docker-enabled template base

- [ ] 1.1 Spike whether the Dune template can extend a Docker-enabled `-docker` sandbox template, or whether Docker Engine + the Compose plugin must be installed into the Dune image directly (decision D1). Record the outcome.
- [ ] 1.2 Create the template build inputs (e.g. `Dockerfile.sbx` and/or a `container/sbx/` tree) separate from the root `Dockerfile`, reusing `container/base/scripts/*`, `container/base/tooling.yaml`, and `container/base/home-defaults/` where possible (D2).
- [ ] 1.3 Ensure the Docker daemon auto-starts on the first `sbx exec` (no manual `dockerd` start), matching the generic `-docker` template behavior (D5).
- [ ] 1.4 Verify inside an ephemeral sandbox: `docker version`, `docker compose version`, `docker run --rm hello-world`, and a nested `docker compose up -d` reachable from inside the sandbox.

## 2. In-template services (convenience) and project-owned Compose

- [ ] 2.1 Update the Postgres service startup to create and chown `/var/run/postgresql` (owner `agent`) before launching `postgres` (D3), so an installed-but-broken Postgres does not ship. Confirm `pg_isready` reports accepting connections when Postgres is started.
- [ ] 2.2 Confirm the Redis and Mailpit tools remain installed and start with their existing scripts (best-effort; not an acceptance gate).
- [ ] 2.3 Verify that project-owned `docker compose` inside the sandbox can host app dependencies (e.g. a Postgres/Mailpit service via a nested Compose project) — this is the canonical path that lets in-template service health stay best-effort.
- [ ] 2.4 Add optional service-native health probes (`pg_isready`, `redis-cli ping`, Mailpit HTTP) and a documented service-log location, for later optional use by `dune logs` / `dune doctor`. Do not assume an s6 status tool (`s6-svstat` was unavailable in the spike).

## 3. Workspace path compatibility

- [ ] 3.1 Remove reliance on a static `WORKDIR /workspace`; do not ship a `/workspace` directory that masks the mount (D4).
- [ ] 3.2 Add a first-run/login step that creates/refreshes `/workspace` as a symlink to the mounted repo path supplied by the backend (e.g. via a `DUNE_WORKSPACE` env var at create time).
- [ ] 3.3 Verify inside a sandbox: `readlink -f /workspace` resolves to the mounted repo and `git -C /workspace rev-parse --show-toplevel` succeeds.

## 4. Toolchain parity

- [ ] 4.1 Ensure the template installs zsh + Dune shell defaults, Rally, Laps, agent CLIs (`claude`, `codex`, `opencode`, `gemini`), `openspec`, `git`/`gh`, and language toolchains (Go, Node, Python, `uv`, `pnpm`).
- [ ] 4.2 Ensure Playwright + Chromium are installed and verify a minimal Chromium launch inside the sandbox.
- [ ] 4.3 Add a parity smoke check that runs `rally --version`, `laps --version`, `playwright --version` (and a representative agent CLI) inside a sandbox.

## 5. Versioning, build, and publish

- [ ] 5.1 Choose the template image reference and version scheme (e.g. `ghcr.io/mitchell-wallace/dune-sbx:<version>`), kept in lockstep with the CLI `VERSION` (D6).
- [ ] 5.2 Add a build-and-push job for the template image (extend `image.yml` or add a workflow), publishing to GHCR.
- [ ] 5.3 Document `sbx create --template <ref>` usage, `sbx template load` for offline use, and `sbx secret set --registry` for private registries.
- [ ] 5.4 Document that the template is built from source only, is never produced via `sbx template save`, and is not a secret boundary (D7).

## 6. Acceptance verification (spike-2 matrix)

- [ ] 6.1 Run the gating acceptance matrix in an ephemeral sandbox: sandbox starts; shell starts in the mounted repo; Docker + Compose work; nested `docker compose up` works; Playwright launches Chromium; `sbx ports` exposes a nested service to the host. Separately confirm (non-gating) that the in-template service tools are present and Postgres starts cleanly when invoked.
- [ ] 6.2 Verify network mediation is intact: `sbx policy deny network --sandbox <name> example.com` blocks both shell and nested-Docker traffic, with records in `sbx policy log <name>`.
- [ ] 6.3 Remove all temporary sandboxes created during verification.

## 7. Documentation

- [ ] 7.1 Document the Dune sbx template: what it contains, how it is built/published/versioned, the `/workspace` compatibility behavior, and the service health/log conventions. Note that the CLI runtime that launches it lands in `sbx-3-sbx-runtime-backend`.
