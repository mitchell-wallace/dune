## ADDED Requirements

### Requirement: Image is based on debian:12-slim
The container base image SHALL be `debian:12-slim`. The image SHALL configure UTF-8 locale (`en_US.UTF-8`) since slim images omit locale setup. The `tzdata` package SHALL be installed for timezone support. The container SHALL respect the `TZ` environment variable forwarded from the host (defaulting to `UTC` if not set).

#### Scenario: Container has correct base, locale, and timezone
- **WHEN** the container starts with `TZ=Australia/Sydney`
- **THEN** `cat /etc/os-release` shows Debian 12
- **THEN** `locale` shows `LANG=en_US.UTF-8`
- **THEN** `date +%Z` shows `AEDT` or `AEST` (depending on DST)

### Requirement: Non-root agent user with passwordless sudo and zsh
The image SHALL create a non-root user named `agent` with a home directory at `/home/agent`, zsh as the default shell with Powerlevel10k theme, and membership in the `sudo` group. The user SHALL have passwordless sudo access. The container SHALL run as the `agent` user by default.

#### Scenario: Agent user exists with correct configuration
- **WHEN** the container starts
- **THEN** `whoami` returns `agent`
- **THEN** `echo $SHELL` shows `/bin/zsh`
- **THEN** `sudo whoami` returns `root` without prompting for a password

### Requirement: zsh with Powerlevel10k is the default shell
The image SHALL install zsh and the Powerlevel10k theme. The `agent` user's default shell SHALL be `/bin/zsh`. A default `.zshrc` with Powerlevel10k configuration SHALL be provided.

#### Scenario: zsh and Powerlevel10k are configured
- **WHEN** the container starts and the user opens a shell
- **THEN** zsh loads with the Powerlevel10k prompt theme

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

### Requirement: mise is pre-installed with default runtime config
The image SHALL install mise globally during the Docker build. A default mise config SHALL be placed at `/home/agent/.config/mise/config.toml` pinning `latest` for Node.js, Go, Python, and Rust. The mise shims directory (`/home/agent/.local/share/mise/shims`) SHALL be on the agent user's PATH via the shell profile. `mise install` SHALL be run as the `agent` user during the Docker build so runtimes are pre-populated in the image layer with no first-run install delay.

#### Scenario: mise is available and runtimes are pre-installed
- **WHEN** the container starts
- **THEN** `mise --version` succeeds
- **THEN** `mise list` shows installed versions of node, go, python, and rust
- **THEN** `which mise` shows a path under the agent user's home

### Requirement: Python, uv, Go, and Rust are pre-installed via mise
The default mise config SHALL include latest stable versions of Python, Go, and Rust. The uv package manager SHALL also be installed via mise. All SHALL be available on PATH via mise shims, pre-installed during the Docker build.

#### Scenario: Language runtimes are available
- **WHEN** the container starts
- **THEN** `python --version`, `uv --version`, `go version`, `rustc --version`, and `cargo --version` all succeed

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

### Requirement: s6-overlay manages services with automatic restart
The image SHALL install s6-overlay from the official release tarball. PostgreSQL, Redis, and Mailpit SHALL be defined as s6 `longrun` services under `/etc/s6-overlay/s6-rc.d/`. The container entrypoint SHALL be s6-overlay's `/init`, which handles PID 1 responsibilities (signal forwarding, zombie reaping), starts all services, and then drops to the `agent` user's zsh shell. s6 SHALL automatically restart any service that crashes.

#### Scenario: All services start on container boot
- **WHEN** the container starts
- **THEN** PostgreSQL, Redis, and Mailpit are all running and accepting connections

#### Scenario: Crashed service is automatically restarted
- **WHEN** the PostgreSQL process crashes during an agent session
- **THEN** s6-overlay detects the crash and restarts PostgreSQL automatically
- **THEN** `pg_isready` succeeds again within seconds

### Requirement: Image is published to GHCR with BuildKit inline cache
The base image SHALL be published to `ghcr.io/mitchell-wallace/dune-base` via GitHub Actions. Images SHALL be tagged with both `latest` and the git commit SHA. The CI build SHALL include `--build-arg BUILDKIT_INLINE_CACHE=1` so that `Dockerfile.dune` builds can use `--cache-from` with BuildKit. The CI workflow SHALL trigger on pushes to main that modify the Dockerfile or related build files.

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
