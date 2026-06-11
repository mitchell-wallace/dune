## 1. Docker-enabled template base

- [ ] 1.1 Spike whether the Dune template can extend a Docker-enabled `-docker` sandbox template, or whether Docker Engine + the Compose plugin must be installed into the Dune image directly (decision D1), and record which D2a s6/dockerd coexistence shape the chosen path implies. Also record, from `sbx create --help` on the installed binary, whether create-time env injection (`--env`/`-e`) is available for the `DUNE_WORKSPACE` contract (D4). Record all outcomes.
- [ ] 1.2 Create the template build inputs (e.g. `Dockerfile.sbx` and/or a `container/sbx/` tree) separate from the root `Dockerfile`, reusing `container/base/scripts/*`, `container/base/tooling.yaml`, the `container/base/s6-overlay/s6-rc.d/` service tree, and `container/base/home-defaults/` where possible (D2).
- [ ] 1.3 Ensure the Docker daemon auto-starts on the first `sbx exec` (no manual `dockerd` start), matching the generic `-docker` template behavior (D5), and that s6-overlay and `dockerd` coexist without either init crash-looping (D2a).
- [ ] 1.4 Verify inside an ephemeral sandbox, in a single `sbx exec`: `docker version` (engine reported), `docker compose version`, `docker run --rm hello-world`, a nested `docker compose up -d` reachable from inside the sandbox, **and** that the s6 service tree is present alongside the running daemon (D2a coexistence check).

## 2. In-template services (convenience) and project-owned Compose

- [ ] 2.1 Update the Postgres s6 `run` script (`container/base/s6-overlay/s6-rc.d/postgres/run`) to `install -d -o agent -g agent -m 0755 /var/run/postgresql` before the `exec "${pg_bin}" ...` line (D3), so an installed-but-broken Postgres does not ship. Verify inside a sandbox that the runtime directory exists and is owned by `agent` and that startup does not fail with the known lock-file error; do not add a health-probe gate.
- [ ] 2.2 Confirm the Redis and Mailpit tools remain installed with their existing scripts, but do not add service auto-start, health probes, or log aggregation to the template scope.
- [ ] 2.3 Verify that project-owned `docker compose` inside the sandbox can host app dependencies (e.g. a Postgres/Mailpit service via a nested Compose project) — this is the canonical path for reliable app dependency lifecycle.

## 3. Workspace path compatibility

- [ ] 3.1 Remove reliance on a static `WORKDIR /workspace` (root `Dockerfile` line 218) and the `/workspace` entry in the build-time `chown`; do not ship a `/workspace` directory that masks the mount (D4).
- [ ] 3.2 Add a first-run/login step that creates/refreshes `/workspace` as a symlink to the mounted repo path. Use the create-time env mechanism confirmed in 1.1 (e.g. `DUNE_WORKSPACE`); if create-time env injection is not supported by the installed `sbx`, fall back to deriving the mount path in-sandbox at login and record that as the contract handed to `sbx-3` D4.
- [ ] 3.3 Verify inside a sandbox created with the chosen mount-path mechanism (e.g. `sbx create --name <name> -e DUNE_WORKSPACE=<mount path> --template <ref> shell <mount path>`): `readlink -f /workspace` resolves to the mounted repo and `git -C /workspace rev-parse --show-toplevel` succeeds.

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

- [ ] 6.1 Run the gating acceptance matrix in an ephemeral sandbox (`sbx create --name <name> --template <ref> shell <mount path>`): sandbox starts and appears in `sbx ls --json`; `/workspace` resolves to the mounted repo; s6 services and `dockerd` coexist in one `sbx exec` (D2a); Docker + Compose work and `docker run --rm hello-world` succeeds; nested `docker compose up -d` (service bound to `0.0.0.0`) works; Playwright launches Chromium; `sbx ports <name> --publish <port> --json` exposes the nested service and the host can reach the published port. Separately confirm (non-gating) that the in-template service tools are present and Postgres starts cleanly when invoked.
- [ ] 6.2 Verify network mediation is intact: `sbx policy deny network --sandbox <name> example.com` blocks both shell (`forward`) and nested-Docker (`transparent`) traffic, with blocked records in `sbx policy log <name> --limit <n>`.
- [ ] 6.3 Remove all temporary sandboxes created during verification.

## 7. Documentation

- [ ] 7.1 Document the Dune sbx template: what it contains, how it is built/published/versioned, the `/workspace` compatibility behavior, and the explicit exclusion of in-template service health probes/log aggregation. Note that the CLI runtime that launches it lands in `sbx-3-sbx-runtime-backend`.
