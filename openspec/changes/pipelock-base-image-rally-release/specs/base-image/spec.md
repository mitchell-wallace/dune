## ADDED Requirements

### Requirement: Image is based on debian:12-slim
The container base image SHALL be `debian:12-slim`. The image SHALL configure UTF-8 locale (`en_US.UTF-8`) since slim images omit locale setup.

#### Scenario: Container has correct base and locale
- **WHEN** the container starts
- **THEN** `cat /etc/os-release` shows Debian 12
- **THEN** `locale` shows `LANG=en_US.UTF-8`

### Requirement: Non-root agent user with passwordless sudo
The image SHALL create a non-root user named `agent` with a home directory at `/home/agent`, a bash shell, and membership in the `sudo` group. The user SHALL have passwordless sudo access. The container SHALL run as the `agent` user by default.

#### Scenario: Agent user exists with correct configuration
- **WHEN** the container starts
- **THEN** `whoami` returns `agent`
- **THEN** `sudo whoami` returns `root` without prompting for a password

### Requirement: System tools are pre-installed
The image SHALL include the following system tools: `curl`, `git`, `sudo`, `tmux`, `jq`, `ripgrep`, `ca-certificates`, `gnupg`, `postgresql-client`, `vim`, `nano`.

#### Scenario: Core system tools are available
- **WHEN** the container starts
- **THEN** `git --version`, `jq --version`, `tmux -V`, `rg --version`, `psql --version` all succeed

### Requirement: Boost CLI tools are pre-installed
The image SHALL include enhanced CLI tools: `fd-find`, `bat`, `tree`, `eza`, `micro`.

#### Scenario: Boost CLI tools are available
- **WHEN** the container starts
- **THEN** `fd --version`, `bat --version`, `tree --version`, `eza --version`, `micro --version` all succeed

### Requirement: Node.js is pre-installed
The image SHALL install Node.js 22.x via NodeSource. npm SHALL be available globally.

#### Scenario: Node.js is available
- **WHEN** the container starts
- **THEN** `node --version` shows v22.x
- **THEN** `npm --version` succeeds

### Requirement: pnpm is pre-installed
The image SHALL install pnpm globally.

#### Scenario: pnpm is available
- **WHEN** the container starts
- **THEN** `pnpm --version` succeeds

### Requirement: Turborepo is pre-installed
The image SHALL install the Turborepo CLI globally.

#### Scenario: turbo is available
- **WHEN** the container starts
- **THEN** `turbo --version` succeeds

### Requirement: mise is pre-installed
The image SHALL install mise for managing per-project runtimes. The mise shims directory SHALL be on the agent user's PATH.

#### Scenario: mise is available and on PATH
- **WHEN** the container starts
- **THEN** `mise --version` succeeds
- **THEN** `which mise` shows a path under the agent user's home

### Requirement: Python and uv are pre-installed via mise
The image SHALL install Python and the uv package manager via mise. Both SHALL be available on PATH via mise shims.

#### Scenario: Python and uv are available
- **WHEN** the container starts
- **THEN** `python --version` and `uv --version` both succeed

### Requirement: Go is pre-installed via mise
The image SHALL install Go via mise. The Go binary SHALL be available on PATH via mise shims.

#### Scenario: Go is available
- **WHEN** the container starts
- **THEN** `go version` succeeds

### Requirement: Rust toolchain is pre-installed via mise
The image SHALL install the Rust toolchain (rustc, cargo) via mise. Both SHALL be available on PATH via mise shims.

#### Scenario: Rust is available
- **WHEN** the container starts
- **THEN** `rustc --version` and `cargo --version` both succeed

### Requirement: PostgreSQL server is pre-installed
The image SHALL install PostgreSQL server (not just client). The entrypoint SHALL start the PostgreSQL service automatically.

#### Scenario: PostgreSQL server is running
- **WHEN** the container starts and the entrypoint completes
- **THEN** `pg_isready` reports the server is accepting connections

### Requirement: Redis server is pre-installed
The image SHALL install Redis server. The entrypoint SHALL start the Redis service automatically.

#### Scenario: Redis server is running
- **WHEN** the container starts and the entrypoint completes
- **THEN** `redis-cli ping` returns `PONG`

### Requirement: Playwright and Chromium are pre-installed
The image SHALL install Playwright globally with Chromium browser and its system dependencies.

#### Scenario: Playwright and Chromium are available
- **WHEN** the container starts
- **THEN** `npx playwright --version` succeeds
- **THEN** Chromium is available for Playwright tests without additional downloads

### Requirement: Mailpit is pre-installed
The image SHALL install Mailpit for local SMTP/email testing. The entrypoint SHALL start Mailpit automatically. Mailpit data SHALL NOT be persisted between container restarts.

#### Scenario: Mailpit is running
- **WHEN** the container starts and the entrypoint completes
- **THEN** Mailpit is listening on its default SMTP and HTTP ports

### Requirement: All AI agent CLIs are pre-installed
The image SHALL install Claude Code, Codex, Opencode, and Gemini CLI globally.

#### Scenario: All agent CLIs are available
- **WHEN** the container starts
- **THEN** `claude --version`, `codex --version`, `opencode version`, and `gemini --version` all succeed

### Requirement: Rally is installed from GitHub Releases
The image SHALL install Rally by downloading the latest release binary from the `mitchell-wallace/rally` GitHub repository. Rally SHALL be installed to `~/.local/bin/rally` and be on PATH.

#### Scenario: Rally is available
- **WHEN** the container starts
- **THEN** `rally --version` succeeds

### Requirement: git-delta is pre-installed
The image SHALL install git-delta for enhanced diff visualization.

#### Scenario: git-delta is available
- **WHEN** the container starts
- **THEN** `delta --version` succeeds

### Requirement: Entrypoint starts services unconditionally
The container entrypoint SHALL start PostgreSQL, Redis, and Mailpit services automatically on container start. No mode gating or conditional logic SHALL be applied — all services always start.

#### Scenario: All services start on container boot
- **WHEN** the container starts
- **THEN** PostgreSQL, Redis, and Mailpit are all running and accepting connections

### Requirement: Image is published to GHCR
The base image SHALL be published to `ghcr.io/mitchell-wallace/dune-base` via GitHub Actions. Images SHALL be tagged with both `latest` and the git commit SHA. The CI workflow SHALL trigger on pushes to main that modify the Dockerfile or related build files.

#### Scenario: Image is pullable from GHCR
- **WHEN** a user runs `docker pull ghcr.io/mitchell-wallace/dune-base:latest`
- **THEN** the image is downloaded successfully

### Requirement: Dockerfile.dune extends the base image
Users SHALL be able to create a `Dockerfile.dune` in their repo root that uses `FROM ghcr.io/mitchell-wallace/dune-base:latest` to add repo-specific tools. When present, `dune` SHALL build this file and use the resulting image instead of the base image.

#### Scenario: Repo with Dockerfile.dune
- **WHEN** a repo contains a `Dockerfile.dune` at its root
- **THEN** `dune up` builds the custom image with `--cache-from ghcr.io/mitchell-wallace/dune-base:latest` and starts the container using the custom image

#### Scenario: Repo without Dockerfile.dune
- **WHEN** a repo does not contain a `Dockerfile.dune`
- **THEN** `dune up` uses `ghcr.io/mitchell-wallace/dune-base:latest` directly
