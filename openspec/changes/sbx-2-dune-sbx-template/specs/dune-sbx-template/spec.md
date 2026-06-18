## ADDED Requirements

### Requirement: Docker Engine and Docker Compose are available inside the sandbox

The Dune sbx template SHALL provide a working Docker Engine and the `docker compose` plugin inside the sandbox, and the Docker daemon SHALL be available without a manual start step on the first `sbx exec` into a started sandbox. The sbx template's native Docker init SHALL own PID 1; the template SHALL NOT install s6-overlay.

#### Scenario: Docker and Compose respond in a fresh sandbox
- **GIVEN** a sandbox created from the Dune sbx template via `sbx create --name <name> --template <ref> shell <mount path>`
- **WHEN** `docker version` and `docker compose version` are run in the first `sbx exec` into the sandbox
- **THEN** both succeed and report a Docker Engine and a Docker Compose version
- **AND** `docker run --rm hello-world` succeeds
- **AND** the sbx init does not crash-loop

#### Scenario: Nested Compose project runs and is reachable
- **GIVEN** a sandbox created from the Dune sbx template
- **WHEN** a nested `docker compose up -d` starts a service bound to all sandbox interfaces (`0.0.0.0`)
- **THEN** the service reports running via `docker compose ps` and is reachable from inside the sandbox
- **AND** `sbx ports <name> --publish <port>` can expose it to the host

---

### Requirement: The attached shell starts in the mounted repository with a /workspace compatibility symlink

The Dune sbx template SHALL treat the real mounted repository path as the canonical working directory and SHALL provide `/workspace` as a symlink to that mounted path. The template SHALL NOT ship a static `/workspace` directory that masks the mount.

#### Scenario: /workspace resolves to the mounted repository
- **GIVEN** a sandbox created from the Dune sbx template with the workspace mounted and its absolute path supplied to the hook-firing login shell via `sbx exec -e DUNE_WORKSPACE=<absolute mount path> <name> bash -lc true`
- **WHEN** `readlink -f /workspace` and `git -C /workspace rev-parse --show-toplevel` are run inside the sandbox
- **THEN** `/workspace` resolves to the mounted repository path
- **AND** the repository top-level is reported
- **AND** `/workspace` is a symlink, not a static directory that masks the mount

---

### Requirement: In-container app services are removed; persistence wiring is re-homed onto a boot hook

The Dune sbx template SHALL NOT ship the in-container Postgres, Redis, or Mailpit services. App dependencies SHALL be provided by a project-owned `docker compose` project inside the sandbox. The credential-wiring `setup-persist` step SHALL be re-homed off s6-overlay onto a one-time boot hook (an `sbx`-native boot hook where available, otherwise an idempotent login-shell hook guarded by a per-sandbox sentinel) so that, on a fresh sandbox, the agent-home persistence symlinks into `/persist/agent` are established, and its output SHALL be written to a canonical in-container log path under `/var/log/dune/`.

#### Scenario: Persistence symlinks are wired on a fresh sandbox
- **GIVEN** a sandbox created from the Dune sbx template
- **WHEN** the agent home is inspected after first login
- **THEN** the persistence symlinks resolve into `/persist/agent` (for example `readlink ~/.claude` resolves to `/persist/agent/.claude`)
- **AND** the `setup-persist` output is present at `/var/log/dune/setup-persist.log`

#### Scenario: No in-container app services ship
- **GIVEN** a sandbox created from the Dune sbx template
- **WHEN** the template's installed services are inspected
- **THEN** no in-container Postgres, Redis, or Mailpit service is present or auto-started

#### Scenario: App dependencies are provided by a project-owned Compose project
- **GIVEN** a sandbox created from the Dune sbx template
- **WHEN** a project-owned `docker compose` project defines an app-dependency service (for example Postgres or Mailpit)
- **THEN** the service runs inside the sandbox via `docker compose`

---

### Requirement: The template reaches Dune toolchain parity

The Dune sbx template SHALL include Dune's batteries-included toolchain, verifiable from inside the sandbox: zsh with the Dune shell defaults, Rally, Laps, the supported agent CLIs (`claude`, `codex`, `opencode`, `gemini`), `openspec`, `git`/`gh`, the language toolchains (Go, Node, Python, `uv`, `pnpm`), and Playwright with a launchable Chromium.

#### Scenario: Bundled tools are present and runnable
- **GIVEN** a sandbox created from the Dune sbx template
- **WHEN** `rally --version`, `laps --version`, and `playwright --version` are run inside the sandbox
- **THEN** each command reports a version without error

#### Scenario: Playwright can launch Chromium
- **GIVEN** a sandbox created from the Dune sbx template
- **WHEN** a minimal Playwright Chromium launch is run inside the sandbox
- **THEN** Chromium launches successfully

---

### Requirement: The template has a defined build, version, and publish path and is not a secret boundary

The Dune sbx template SHALL be built from source and published to a container registry as a versioned image reference that `sbx create --template <ref>` can consume, and SHALL be loadable via `sbx template load`. The template version SHALL be kept in lockstep with the Dune CLI version. The template SHALL NOT be produced from an `sbx template save` snapshot, and no secrets SHALL be baked into it.

#### Scenario: A sandbox can be created from the published template reference
- **GIVEN** the Dune sbx template published at its versioned registry reference
- **WHEN** `sbx create --name <name> --template <ref> shell <mount path>` is run
- **THEN** the sandbox is created from the Dune sbx template and appears in `sbx ls --json`

#### Scenario: Template version tracks the CLI version
- **GIVEN** a Dune CLI release version
- **WHEN** the corresponding template image is built and published
- **THEN** the template image version matches the CLI version per the lockstep convention

---

### Requirement: The template does not bypass sbx network mediation

The Dune sbx template SHALL NOT disable or circumvent the `sbx` network policy layer. Outbound traffic from the sandbox shell and from nested Docker containers SHALL remain subject to `sbx` policy and observable through `sbx policy log`.

#### Scenario: A scoped deny blocks both shell and nested Docker traffic
- **GIVEN** a sandbox created from the Dune sbx template
- **AND** a scoped deny rule `sbx policy deny network --sandbox <name> example.com`
- **WHEN** the shell and a nested Docker container each request `example.com`
- **THEN** both requests are blocked (the shell request as `forward`, the nested-Docker request as `transparent`)
- **AND** `sbx policy log <name>` shows the blocked records
