## ADDED Requirements

### Requirement: dune up starts the two-container topology
The `dune` or `dune up` command SHALL start an agent container and a Pipelock sidecar container using `docker compose up -d`. The compose file SHALL be generated from an embedded Go template and written to `~/.local/share/dune/projects/<slug>/compose.yaml`, where slug is `<folderName>-<2hexhash>` (2 hex chars derived from a hash of the absolute workspace root path, e.g., `myapp-a3`). The compose project name SHALL be set explicitly to `dune-<slug>-<profile>` to avoid collisions.

The **workspace root** is defined as the git repository root (`git rev-parse --show-toplevel`). If the current directory is not inside a git repo, the workspace root falls back to the current working directory. Running `dune` from any subdirectory within a repo always resolves to the same workspace root.

#### Scenario: Starting containers in a repo for the first time
- **WHEN** a user runs `dune` in a repo directory with no running containers
- **THEN** dune generates the compose file, runs `docker compose up -d`, and attaches to the agent container shell

#### Scenario: Attaching to already-running containers
- **WHEN** a user runs `dune` in a repo directory with containers already running
- **THEN** dune attaches to the existing agent container shell without restarting

### Requirement: dune down stops containers
The `dune down` command SHALL run `docker compose down` to stop and remove the containers for the current directory's project.

#### Scenario: Stopping running containers
- **WHEN** a user runs `dune down` in a repo directory with running containers
- **THEN** both the agent and Pipelock containers are stopped and removed
- **THEN** the home directory volume is preserved

### Requirement: dune rebuild forces image rebuild
The `dune rebuild` command SHALL force a rebuild of the agent image (running `docker compose build --no-cache` for the agent service) and recreate the containers. The home directory volume SHALL be preserved.

#### Scenario: Rebuilding after Dockerfile.dune change
- **WHEN** a user modifies `Dockerfile.dune` and runs `dune rebuild`
- **THEN** the agent image is rebuilt from scratch and the containers are recreated
- **THEN** credentials and config in the home directory volume persist

### Requirement: dune logs tails compose logs
The `dune logs [service]` command SHALL run `docker compose logs -f [service]`. When no service is specified, it SHALL show logs for all services. This is primarily useful for `dune logs pipelock` to view proxy activity.

#### Scenario: Viewing Pipelock logs
- **WHEN** a user runs `dune logs pipelock`
- **THEN** they see a live tail of the Pipelock container's JSON log output

### Requirement: Compose template produces correct topology
The generated compose file SHALL define two services: `agent` and `pipelock`. The `agent` service SHALL connect only to the `internal` network. The `pipelock` service SHALL connect to both `internal` and `external` networks. The `internal` network SHALL be marked as `internal: true` (no external access). The `agent` service SHALL have `depends_on: pipelock`.

#### Scenario: Network isolation is enforced
- **WHEN** containers are running
- **THEN** the agent container cannot reach the internet directly
- **THEN** the agent container can reach the Pipelock container on the internal network

### Requirement: Compose template configures the agent service correctly
The agent service in the generated compose file SHALL:
- Use the appropriate image (base or Dockerfile.dune-built)
- Set proxy env vars in both cases: `http_proxy=http://pipelock:8888`, `HTTP_PROXY=http://pipelock:8888`, `https_proxy=http://pipelock:8888`, `HTTPS_PROXY=http://pipelock:8888`, `no_proxy=localhost,127.0.0.1`, `NO_PROXY=localhost,127.0.0.1`
- Forward the host's `TZ` environment variable for timezone support
- Mount the workspace root to `/workspace`
- Mount the profile-specific persist volume (`dune-persist-<profile>`) to `/persist/agent`
- Set `working_dir: /workspace`
- No API keys SHALL be forwarded — agent CLIs authenticate via OAuth tokens persisted in the persist volume

#### Scenario: Agent container has correct environment
- **WHEN** the agent container starts
- **THEN** `echo $http_proxy` outputs `http://pipelock:8888`
- **THEN** `echo $HTTP_PROXY` outputs `http://pipelock:8888`
- **THEN** `/workspace` contains the repo files
- **THEN** `date +%Z` matches the host timezone

### Requirement: Compose template configures the Pipelock service correctly
The Pipelock service in the generated compose file SHALL:
- Use a pinned Pipelock image from GHCR (`ghcr.io/luckypipewrench/pipelock:<pinned-tag>` — pin to specific version tag at implementation time)
- Mount `~/.config/dune/pipelock.yaml` read-only to `/config/pipelock.yaml`
- Run with `command: run --config /config/pipelock.yaml --listen 0.0.0.0:8888`
- Have `restart: unless-stopped` policy

#### Scenario: Pipelock container is correctly configured
- **WHEN** the containers start
- **THEN** Pipelock is listening on port 8888 on the internal network
- **THEN** Pipelock is using the config from `~/.config/dune/pipelock.yaml`

### Requirement: Dockerfile.dune detection and build
When `dune up` or `dune` is run, dune SHALL check for `Dockerfile.dune` in the workspace root (as resolved by the git-root rule above). If present, dune SHALL pull the configured published base-image tag first (to ensure cache layers are available), then build it tagged as `dune-local-<slug>:latest` with `--cache-from` against that same published base-image tag using the workspace root as the Docker build context. `COPY` commands in `Dockerfile.dune` are relative to the workspace root. If absent, dune SHALL use the configured published base-image tag directly.

#### Scenario: Repo has Dockerfile.dune
- **WHEN** `Dockerfile.dune` exists at the workspace root
- **THEN** dune pulls the base image, builds the custom image with cache-from, and uses it for the agent container

#### Scenario: Repo has no Dockerfile.dune
- **WHEN** no `Dockerfile.dune` exists at the workspace root
- **THEN** dune uses the configured published base-image tag for the agent container

### Requirement: Persist volume is created per profile
The dune CLI SHALL create a named Docker volume `dune-persist-<profile>` for each profile. This volume SHALL be mounted at `/persist/agent` in the agent container. An s6 oneshot service in the container creates symlinks from the agent's home directory into the persist volume for credential and config paths, persisting auth tokens and shell configuration across container restarts and rebuilds.

#### Scenario: Volume persists across container lifecycle
- **WHEN** a user runs `dune down` then `dune up`
- **THEN** the agent container has the same credentials and shell config as before
- **THEN** OAuth tokens for Claude Code, GitHub CLI, Codex, etc. are still present via symlinks into `/persist/agent`
