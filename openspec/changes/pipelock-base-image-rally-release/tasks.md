## 1. Rally Extraction

- [ ] 1.1 Create `mitchell-wallace/rally` GitHub repository
- [ ] 1.2 Move `cmd/rally` and `internal/rally` to the new repo, resolving shared `internal/contracts` types
- [ ] 1.3 Ensure `go build ./cmd/rally` succeeds independently in the new repo
- [ ] 1.4 Add `Version` variable with ldflags injection in `main.go`
- [ ] 1.5 Create `.goreleaser.yaml` with builds for linux/darwin amd64/arm64
- [ ] 1.6 Create `.github/workflows/release.yml` that runs `goreleaser release --clean` on `v*` tag push
- [ ] 1.7 Create `install.sh` that detects OS/arch, downloads latest release, installs to `~/.local/bin/rally`
- [ ] 1.8 Implement `rally update` subcommand that downloads latest release and replaces current binary
- [ ] 1.9 Implement background version check on startup with `RALLY_NO_UPDATE_CHECK=1` suppression
- [ ] 1.10 Tag and publish first release (`v0.1.0`), verify install script works on Linux and macOS
- [ ] 1.11 Remove Rally source code (`cmd/rally`, `internal/rally`, `internal/contracts/rally`) from the dune repo

## 2. Base Image

- [ ] 2.1 Write new `Dockerfile` based on `debian:12-slim` with locale setup and `agent` user
- [ ] 2.2 Install system tools: curl, git, sudo, tmux, jq, ripgrep, ca-certificates, gnupg, postgresql-client, vim, nano
- [ ] 2.3 Install boost CLI tools: fd-find, bat, tree, eza, micro, git-delta
- [ ] 2.4 Install Node.js 22.x via NodeSource and npm
- [ ] 2.5 Install pnpm and Turborepo globally via npm
- [ ] 2.6 Install mise and configure PATH for shims
- [ ] 2.7 Install Python + uv via mise
- [ ] 2.8 Install Go via mise
- [ ] 2.9 Install Rust toolchain via mise
- [ ] 2.10 Install PostgreSQL server and configure for agent user
- [ ] 2.11 Install Redis server
- [ ] 2.12 Install Playwright with Chromium and system dependencies
- [ ] 2.13 Install Mailpit
- [ ] 2.14 Install Claude Code, Codex, Opencode, and Gemini CLIs
- [ ] 2.15 Install Rally from GitHub Releases via install script (depends on 1.10)
- [ ] 2.16 Write entrypoint script that starts PostgreSQL, Redis, and Mailpit unconditionally
- [ ] 2.17 Build image locally and verify all tools and services work
- [ ] 2.18 Create `.github/workflows/image.yml` to build and push to `ghcr.io/mitchell-wallace/dune-base` on main pushes

## 3. Pipelock Configuration

- [ ] 3.1 Generate baseline `pipelock.yaml` using `pipelock generate config --preset balanced` as starting point
- [ ] 3.2 Configure `mode: balanced` with `enforce: true` and response scanning with action `warn`
- [ ] 3.3 Add DLP patterns for Anthropic API keys (`sk-ant-`), AWS access keys, GitHub tokens
- [ ] 3.4 Seed `api_allowlist` with core domains from current firewall allowlist
- [ ] 3.5 Add blocklist for exfiltration targets: pastebin.com, transfer.sh, hastebin.com
- [ ] 3.6 Enable rate limiting with reasonable defaults for AI agent workloads
- [ ] 3.7 Configure JSON logging to stdout
- [ ] 3.8 Embed the default config as a Go template in the dune CLI source
- [ ] 3.9 Test Pipelock proxy routing end-to-end: agent → pipelock → external API

## 4. Dune CLI Rewrite

- [ ] 4.1 Create new `cmd/dune/main.go` and `internal/dune` package structure from scratch
- [ ] 4.2 Implement workspace slug derivation (from git root or directory path)
- [ ] 4.3 Implement profile resolution: `--profile`/`-p` flag → stored mapping → `default` fallback
- [ ] 4.4 Implement `~/.config/dune/profiles.json` read/write for directory-to-profile mapping
- [ ] 4.5 Implement `dune profile set <name>` command
- [ ] 4.6 Implement `dune profile list` command
- [ ] 4.7 Implement compose.yaml generation from embedded Go template
- [ ] 4.8 Implement `Dockerfile.dune` detection and `docker build --cache-from` logic
- [ ] 4.9 Implement `dune` / `dune up` command: generate compose, `docker compose up -d`, `docker compose exec agent bash`
- [ ] 4.10 Implement `dune down` command: `docker compose down`
- [ ] 4.11 Implement `dune rebuild` command: `docker compose build --no-cache`, recreate containers
- [ ] 4.12 Implement `dune logs [service]` command: `docker compose logs -f [service]`
- [ ] 4.13 Implement first-run Pipelock config generation (write embedded default to `~/.config/dune/pipelock.yaml`)
- [ ] 4.14 Implement home volume creation (`dune-home-<profile>`) if it doesn't exist
- [ ] 4.15 Update `dune.sh` entry point to build and run the new CLI

## 5. Compose Template

- [ ] 5.1 Write Go template for `compose.yaml` with agent and pipelock services
- [ ] 5.2 Configure agent service: image, proxy env vars, API key forwarding, workspace mount, home volume, internal network only, working_dir
- [ ] 5.3 Configure pipelock service: image, config mount (read-only), internal + external networks, command, restart policy
- [ ] 5.4 Define internal network (`internal: true`) and external network
- [ ] 5.5 Test generated compose file with `docker compose config` validation

## 6. Cleanup

- [ ] 6.1 Delete `container/devcontainer.json`
- [ ] 6.2 Delete `container/runtime/init-firewall.sh` and `container/runtime/firewall-domains.tsv`
- [ ] 6.3 Delete gear system: `container/gear/manifest.tsv`, `container/gear/*.sh`, `container/runtime/gear-cli.sh`
- [ ] 6.4 Delete security mode logic: `container/runtime/dune-privileged.sh` and related mode scripts
- [ ] 6.5 Delete `dune.toml` and `internal/dune/config/` (config parser)
- [ ] 6.6 Delete devcontainer orchestration code in `internal/dune/devcontainer/`
- [ ] 6.7 Delete rally binary sync code in `internal/dune/` (rally build/update commands)
- [ ] 6.8 Delete old `container/Dockerfile` (replaced by new Dockerfile in step 2)
- [ ] 6.9 Delete runtime scripts no longer needed: `dune-poststart.sh`, `dune-entrypoint.sh`, `setup-agent-persist.sh`
- [ ] 6.10 Update CLAUDE.md and AGENTS.md to reflect new architecture

## 7. Integration Testing

- [ ] 7.1 Run `dune` end-to-end in a test repo with no `Dockerfile.dune` — verify containers start, agent shell works
- [ ] 7.2 Verify agent can reach `api.anthropic.com` through Pipelock proxy
- [ ] 7.3 Verify agent cannot reach the internet directly (bypassing proxy)
- [ ] 7.4 Verify `dune logs pipelock` shows request logs
- [ ] 7.5 Verify credentials persist across `dune down` / `dune up` cycle
- [ ] 7.6 Test `Dockerfile.dune` flow: create one, run `dune`, verify custom image is used
- [ ] 7.7 Test profile switching: `dune --profile work` vs `dune --profile personal` produce isolated containers
- [ ] 7.8 Verify `rally --version` works inside the container
