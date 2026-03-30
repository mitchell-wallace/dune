# Dune

> Because sometimes, your agents need something a little bigger than a sandbox

`dune` is a host-side Go CLI that starts a two-container workspace:

- `agent`: the interactive development container
- `pipelock`: the outbound HTTP(S) proxy sidecar

The `agent` container comes with a batteries-included base image for AI-assisted development, including [Rally](https://github.com/mitchell-wallace/rally), a Ralph-loop based agent runner for orchestrating work inside the container.

## Usage

For local development from this repo, install the alias with `./install-dune-alias.sh`.

For release builds, download the standalone `dune` binary for your platform from GitHub Releases and place it on your `PATH`.

Then run:

```sh
dune
dune up
dune down
dune rebuild
dune logs
dune logs pipelock
dune version
dune profile set work
dune profile list
```

`dune` resolves the workspace root from `git rev-parse --show-toplevel` and falls back to the current directory outside a git repo. It stores generated compose files under `~/.local/share/dune/projects/<slug>/compose.yaml`.

## Commands

- `dune` starts the workspace for the current repo if needed, then opens an interactive `zsh` shell inside the `agent` container
- `dune up` does the same thing explicitly
- `dune down` stops the `agent` and `pipelock` containers for the current workspace/profile
- `dune rebuild` rebuilds the `agent` image for the current workspace, then recreates the workspace containers
- `dune logs` tails logs for the current workspace
- `dune logs pipelock` tails just the proxy logs, which is useful when checking outbound requests or policy decisions
- `dune version` prints the dune version, commit, and release build metadata
- `dune profile set <name>` stores a profile mapping for the current workspace root
- `dune profile list` shows the effective profile for the current workspace and any stored mappings

## What Dune Sets Up

When you run `dune`, it:

- resolves the workspace root from the current directory
- selects a profile, defaulting to `default`
- ensures the Pipelock config exists at `~/.config/dune/pipelock.yaml`
- creates or reuses the profile's persist volume
- uses the published base image, or builds `Dockerfile.dune` if the repo defines one
- starts the `agent` and `pipelock` services with Docker Compose
- attaches you to the `agent` container in `/workspace`

## Profiles and persistence

Profiles are string names such as `default`, `work`, or `personal`.

- `dune profile set <name>` stores a directory-to-profile mapping in `~/.config/dune/profiles.json`
- `--profile` / `-p` overrides the stored mapping for a given command
- Each profile gets its own Docker volume: `dune-persist-<profile>`

The base image seeds and persists these home-directory paths through `/persist/agent`:

- `~/.claude/`
- `~/.codex/`
- `~/.gemini/`
- `~/.config/opencode/`
- `~/.local/share/opencode/`
- `~/.config/gh/`
- `~/.gitconfig`
- `~/.git-credentials`
- `~/.zshrc`
- `~/.p10k.zsh`

## Repo-specific customization

If a repo contains `Dockerfile.dune` at the workspace root, `dune` builds it and uses the resulting image for the `agent` service. The build context is the workspace root, so `COPY` paths are relative to the repo root.

If no `Dockerfile.dune` is present, `dune` uses the published base image directly.

## Included Tools

The base image is meant to be ready to use without a separate bootstrap step. It includes:

- `claude`: Anthropic's Claude Code CLI for agentic coding workflows
- `codex`: OpenAI Codex CLI for coding and automation tasks
- `gemini`: Google's Gemini CLI for model-assisted development work
- `opencode`: Opencode CLI for agent-driven coding workflows
- `rally`: Ralph-loop based agent runner that ships with dune
- `gh`: GitHub CLI for repository, auth, PR, and release workflows
- `git`: source control inside the container
- `delta`: syntax-highlighted Git pager for diffs
- `tmux`: terminal multiplexer for long-lived sessions
- `zsh` with Powerlevel10k: the default interactive shell and prompt
- `vim`, `nano`, `micro`: terminal editors with different levels of complexity
- `ripgrep`: fast recursive code search
- `fd`: friendly file finder
- `fzf`: fuzzy finder for shell navigation and filtering
- `bat`: `cat` with syntax highlighting and paging
- `eza`: modern replacement for `ls`
- `tree`: directory tree viewer
- `jq`: JSON query and formatting tool
- `curl`: HTTP client for APIs and downloads
- `Node.js` and `npm`: JavaScript runtime and package manager used by several CLIs and builds
- `pnpm`: fast JavaScript package manager
- `turbo`: Turborepo build orchestration CLI
- `mise`: runtime manager used to provide current language toolchains in the shell
- `go`: Go toolchain installed through `mise`
- `python`: Python runtime installed through `mise`
- `rust` and `cargo`: Rust toolchain installed through `mise`
- `uv`: fast Python package and environment tool installed through `mise`
- `playwright` with Chromium: browser automation and web testing stack
- `postgresql`: local PostgreSQL server and client tools
- `redis-server`: local Redis server for app development and caching tests
- `mailpit`: local SMTP capture server and web UI for email testing
- `sudo`: passwordless sudo for the `agent` user when elevated commands are needed

## Rally

[Rally](https://github.com/mitchell-wallace/rally) ships in the base image and is available inside every dune workspace.

- Rally is a Ralph-loop based agent runner that comes with dune
- Rally configuration lives in `rally.toml` at the workspace root
- The base image installs `rally` from GitHub Releases
- `rally` can update itself independently inside the container

## Networking

The `agent` container is attached only to an internal Docker network and reaches external HTTP(S) services through the `pipelock` sidecar via proxy environment variables.

[Pipelock](https://github.com/luckypipewrench/pipelock) is the outbound policy and monitoring layer for dune workspaces.

- the `agent` container sends HTTP(S) traffic through Pipelock using `http_proxy` and `https_proxy`
- Pipelock runs with dune's generated config at `~/.config/dune/pipelock.yaml`
- the default config enables balanced-mode enforcement, JSON logging, response scanning, rate limiting, and an allowlist/blocklist baseline for common AI tooling traffic
- `dune logs pipelock` is the main way to inspect proxy activity while you work

Git via SSH is not natively supported in this architecture because SSH traffic does not flow through the HTTP proxy sidecar. Prefer HTTPS remotes inside dune workspaces.

## Development

Useful checks:

```sh
go test ./...
go build ./cmd/dune
./dune.sh profile list
```
