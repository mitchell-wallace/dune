## 1. Rally Extraction

- [ ] 1.1 Create `mitchell-wallace/rally` GitHub repository
- [ ] 1.2 Move `cmd/rally` and `internal/rally` to the new repo (drop `internal/contracts/rally` entirely — no shared types needed)
- [ ] 1.3 Ensure `go build ./cmd/rally` succeeds independently in the new repo
- [ ] 1.4 Add `Version` variable with ldflags injection in `main.go`
- [ ] 1.5 Implement `rally.toml` config reading **and writing** from `/workspace/rally.toml` for model prefs and beads — migrate `rally init` to write beads here instead of `dune.toml`, delete `writeDuneToml()` and `loadModelDefaults()` helpers
- [ ] 1.6 Create `.goreleaser.yaml` with builds for linux/darwin amd64/arm64
- [ ] 1.7 Create `.github/workflows/release.yml` that runs `goreleaser release --clean` on `v*` tag push
- [ ] 1.8 Create `install.sh` that detects OS/arch, downloads latest release, installs to `~/.local/bin/rally`
- [ ] 1.9 Implement `rally update` subcommand that downloads latest release and replaces current binary
- [ ] 1.10 Implement background version check on startup with `RALLY_NO_UPDATE_CHECK=1` suppression
- [ ] 1.11 Tag and publish first release (`v0.1.0`), verify install script works on Linux and macOS
- [ ] 1.12 Remove Rally source code (`cmd/rally`, `internal/rally`, `internal/contracts/rally`) from the dune repo

## 2. Base Image

- [ ] 2.1 Write new `Dockerfile` based on `debian:12-slim` with locale setup (`en_US.UTF-8`), timezone support (`tzdata`), and `agent` user with passwordless sudo
- [ ] 2.2 Install s6-overlay from official release tarball
- [ ] 2.3 Install zsh and Powerlevel10k theme, set as default shell for `agent` user
- [ ] 2.4 Install system tools: curl, git, sudo, tmux, jq, ripgrep, ca-certificates, gnupg, postgresql-client, vim, nano
- [ ] 2.5 Install boost CLI tools: fd-find, bat, tree, eza, micro, git-delta
- [ ] 2.6 Install Node.js 22.x via NodeSource and npm
- [ ] 2.7 Install pnpm and Turborepo globally via npm
- [ ] 2.8 Install mise globally, configure PATH for shims in zsh/bash profiles
- [ ] 2.9 Create default mise config at `/home/agent/.config/mise/config.toml` with `latest` for node, go, python, rust, and uv
- [ ] 2.10 Run `mise install` as `agent` user during build to pre-populate all runtimes into image layer
- [ ] 2.11 Install PostgreSQL server and configure for agent user
- [ ] 2.12 Install Redis server
- [ ] 2.13 Install Playwright with Chromium and system dependencies
- [ ] 2.14 Install Mailpit
- [ ] 2.15 Install Claude Code, Codex, Opencode, and Gemini CLIs
- [ ] 2.16 Install Rally from GitHub Releases via install script (depends on 1.11)
- [ ] 2.17 Define s6 `longrun` service directories for PostgreSQL, Redis, and Mailpit under `/etc/s6-overlay/s6-rc.d/` (with `run` scripts)
- [ ] 2.18 Write s6 `oneshot` `setup-persist` service: seed defaults into `/persist/agent` if empty (copy image defaults without overwriting existing files), then create symlinks from home dir paths (`.claude/`, `.codex/`, `.gemini/`, `.config/opencode/`, `.local/share/opencode/`, `.config/gh/`, `.gitconfig`, `.git-credentials`, `.zshrc`, `.p10k.zsh`) into `/persist/agent`
- [ ] 2.19 Store default `.zshrc` and `.p10k.zsh` in `/opt/home-defaults/` during image build for seeding
- [ ] 2.20 Build image locally and verify all tools, services, persistence symlinks, and s6 auto-restart work
- [ ] 2.21 Create `.github/workflows/image.yml` to build and push to `ghcr.io/mitchell-wallace/dune-base` on main pushes (with `--build-arg BUILDKIT_INLINE_CACHE=1`)

## 3. Pipelock Configuration

- [ ] 3.1 Determine current Pipelock release tag and pin version (do not use `:latest`)
- [ ] 3.2 Generate baseline config via `docker run --rm ghcr.io/luckypipewrench/pipelock:<pinned-tag> generate config --preset balanced`
- [ ] 3.3 Customise: set `enforce: true`, `response_scanning.enabled: true` with `action: warn`, `dlp.include_defaults: true`
- [ ] 3.4 Seed `api_allowlist` with core domains using wildcards: `*.anthropic.com`, `*.openai.com`, `*.googleapis.com`, etc.
- [ ] 3.5 Add `fetch_proxy.monitoring.blocklist` for exfiltration targets: `*.pastebin.com`, `*.hastebin.com`, `*.transfer.sh`, `file.io`, `requestbin.net`
- [ ] 3.6 Set `fetch_proxy.monitoring.max_requests_per_minute` to reasonable default (e.g., 60)
- [ ] 3.7 Configure `logging.format: json`, `logging.output: stdout`
- [ ] 3.8 Embed the customised config as a Go template in the dune CLI source

## 4. Dune CLI Rewrite

- [ ] 4.1 Create new `cmd/dune/main.go` and `internal/dune` package structure from scratch
- [ ] 4.2 Implement workspace root resolution: `git rev-parse --show-toplevel` with cwd fallback; derive slug as `<folderName>-<2hexhash>` from the resolved workspace root path
- [ ] 4.3 Implement profile resolution: `--profile`/`-p` flag → stored mapping → `default` fallback
- [ ] 4.4 Implement `~/.config/dune/profiles.json` read/write for directory-to-profile mapping
- [ ] 4.5 Implement `dune profile set <name>` command (validates: lowercase alphanumeric + hyphens only)
- [ ] 4.6 Implement `dune profile list` command
- [ ] 4.7 Implement compose.yaml generation from embedded Go template
- [ ] 4.8 Implement `Dockerfile.dune` detection: pull base image first, then `docker build --cache-from` with workspace root as build context
- [ ] 4.9 Implement `dune` / `dune up` command: generate compose, `docker compose up -d`, `docker compose exec agent zsh`
- [ ] 4.10 Implement `dune down` command: `docker compose down`
- [ ] 4.11 Implement `dune rebuild` command: `docker compose build --no-cache` for agent service, recreate containers
- [ ] 4.12 Implement `dune logs [service]` command: `docker compose logs -f [service]`
- [ ] 4.13 Implement first-run Pipelock config generation: run `docker run --rm ghcr.io/luckypipewrench/pipelock:<pinned-tag> generate config --preset balanced`, apply customisations, write to `~/.config/dune/pipelock.yaml`
- [ ] 4.14 Implement persist volume creation (`dune-persist-<profile>`) if it doesn't exist
- [ ] 4.15 Forward host `TZ` environment variable to agent container
- [ ] 4.16 Update `dune.sh` entry point to build and run the new CLI
- [ ] 4.17 Implement clear, actionable error messages: validate prerequisites early (Docker running? Compose available?), surface relevant log tail on `docker compose up` failures

## 5. Compose Template

- [ ] 5.1 Write Go template for `compose.yaml` with agent and pipelock services
- [ ] 5.2 Configure agent service: image, proxy env vars (both `http_proxy`/`HTTP_PROXY` and `https_proxy`/`HTTPS_PROXY`, `no_proxy`/`NO_PROXY=localhost,127.0.0.1`), `TZ` forwarding, workspace mount, persist volume at `/persist/agent`, internal network only, working_dir, depends_on pipelock
- [ ] 5.3 Configure pipelock service: pinned image, config mount (read-only), internal + external networks, command (`run --config /config/pipelock.yaml --listen 0.0.0.0:8888`), restart policy (`unless-stopped`)
- [ ] 5.4 Define internal network (`internal: true`) and external network
- [ ] 5.5 Test generated compose file with `docker compose config` validation

## 6. Cleanup

- [ ] 6.1 Delete `container/devcontainer.json`
- [ ] 6.2 Delete `container/runtime/init-firewall.sh` and `container/runtime/firewall-domains.tsv`
- [ ] 6.3 Delete gear system: `container/gear/manifest.tsv`, `container/gear/*.sh`, `container/runtime/gear-cli.sh`
- [ ] 6.4 Delete security mode logic: `container/runtime/dune-privileged.sh` and related mode scripts in `container/runtime/dune-privileged/`
- [ ] 6.5 Delete `dune.toml` and `internal/dune/config/` (config parser)
- [ ] 6.6 Delete devcontainer orchestration code in `internal/dune/devcontainer/`
- [ ] 6.7 Delete rally binary sync code in `internal/dune/` (rally build/update commands)
- [ ] 6.8 Delete old `container/Dockerfile` (replaced by new Dockerfile in step 2)
- [ ] 6.9 Delete runtime scripts no longer needed: `dune-poststart.sh`, `dune-entrypoint.sh`, `setup-agent-persist.sh` (replaced by s6 `setup-persist` oneshot)
- [ ] 6.10 Delete `internal/dune/tui/` (config wizard)
- [ ] 6.11 Update CLAUDE.md and AGENTS.md to reflect new architecture (explicitly removing the rule that Rally cannot self-update)
- [ ] 6.12 Document in README.md that Git via SSH is not natively supported due to container network isolation constraints

## 7. Code Quality & Linting

- [ ] 7.1 Add `.golangci.yml` to dune repo with standard linter set; verify `golangci-lint run` passes
- [ ] 7.2 Add `shellcheck` validation for all `.sh` files (s6 run scripts, install.sh, dune.sh)
- [ ] 7.3 Add `hadolint` validation for the base Dockerfile
- [ ] 7.4 Create `justfile` with `just test` target that runs: golangci-lint, shellcheck, hadolint, Go unit tests

## 8. Unit Tests (Dune CLI)

- [ ] 8.1 Workspace root resolution: git repo → toplevel; non-git → cwd; subdirectory → same root
- [ ] 8.2 Slug generation: known path → expected `<name>-<2hex>`; verify different paths produce different slugs
- [ ] 8.3 Profile resolution: flag > stored mapping > default; validation rejects invalid names
- [ ] 8.4 Compose template rendering: golden-file test — render with known inputs, compare to expected YAML
- [ ] 8.5 Pipelock config generation: verify customisations are applied to baseline; validate YAML structure

## 9. Test Fixtures

- [ ] 9.1 Create `test/fixtures/sample-project/` with a minimal project directory (e.g. a README, a simple source file, a `Dockerfile.dune` that `FROM`s the base image and `COPY`s a file from the repo)
- [ ] 9.2 Use this fixture for integration tests involving `Dockerfile.dune` COPY context, workspace resolution, and end-to-end flows

## 10. Integration Testing

Tests are automatable unless marked `[local-only]` (requires Docker socket / real container runtime).

### CI-automatable

- [ ] 10.1 `docker compose config` validates generated compose.yaml (no Docker daemon needed)
- [ ] 10.2 Image build: build base image in CI, run smoke test container verifying all tools exist
- [ ] 10.3 Image smoke test: `node --version`, `go version`, `python --version`, `rustc --version`, `uv --version`, `pnpm --version`, `turbo --version`, `mise --version`, `rally --version`, `jq --version`, `rg --version`, `tmux -V`, `fd --version`, `bat --version`, `eza --version`, `delta --version`
- [ ] 10.4 Agent CLI smoke test: `claude --version`, `codex --version`, `gemini --version`, `opencode version`
- [ ] 10.5 Service smoke test: `pg_isready`, `redis-cli ping`, verify Mailpit ports
- [ ] 10.6 s6 auto-restart: kill postgres in smoke container, verify `pg_isready` recovers within seconds
- [ ] 10.7 Persist seeding: start container with empty volume, verify default `.zshrc` and `.p10k.zsh` are seeded, verify symlinks point to `/persist/agent`
- [ ] 10.8 Persist preservation: start container with pre-populated volume, verify existing files are NOT overwritten
- [ ] 10.9 `Dockerfile.dune` COPY context: use `test/fixtures/sample-project/` fixture, verify the COPY'd file exists in the built image

### Local-only (requires Docker socket)

- [ ] 10.10 `[local-only]` End-to-end `dune up` in test repo — verify containers start, agent gets zsh shell
- [ ] 10.11 `[local-only]` Verify agent can reach `api.anthropic.com` through Pipelock proxy
- [ ] 10.12 `[local-only]` Verify agent cannot reach the internet directly (bypassing proxy)
- [ ] 10.13 `[local-only]` Verify `dune logs pipelock` shows JSON request logs
- [ ] 10.14 `[local-only]` Verify credentials persist across `dune down` / `dune up` cycle and across `dune rebuild`
- [ ] 10.15 `[local-only]` Test profile switching: `dune --profile work` vs `dune --profile personal` produce isolated containers with separate persist volumes
- [ ] 10.16 `[local-only]` Verify timezone matches host (`TZ` forwarded correctly)
- [ ] 10.17 `[local-only]` Verify mise-managed runtimes are available: `node`, `go`, `python`, `rustc`, `uv`
