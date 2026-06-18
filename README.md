# Dune

> Because sometimes, your agents need something a little bigger than a sandbox

Dune is a single-command, profile-aware, persistent, isolated development environment for AI-assisted coding work. Run `dune` for the host-side Go CLI that launches a dedicated **Dune sbx template** image inside an `sbx` microVM sandbox, then attaches you to an interactive shell at your repository root.

The sandbox runs a batteries-included base image for AI-assisted development, including [Rally](https://github.com/mitchell-wallace/rally), a failure-tolerant Ralph-loop based meta-harness with task-model routing for orchestrating work inside the sandbox, and [Laps](), the extensible agent-first sequential task manager CLI for organising units of work.

## What Dune Offers

1. Memorable entrypoint

dune gets you from a host repo into a ready-to-use isolated workspace. It resolves the workspace root, selects a profile, ensures the profile's persisted state exists, validates the host `sbx` install, creates/starts the sandbox from the Dune sbx template, and attaches a shell at the repository root. The current command flow shows this is all concentrated in the default/up path.

2. Batteries-included agent coding base image

The sbx template is not incidental; it is central product surface. It includes agent CLIs, Git/GitHub tooling, shells/editors/search tools, language toolchains, Playwright, and DB clients. That makes Dune closer to a reproducible agent workstation than a thin sandbox wrapper.

3. Preserved auth/config/tooling state

Profiles and persistence are a core feature, not an implementation detail. Dune preserves things like Claude, Codex, OpenCode, GitHub CLI, git config/credentials, shell config, etc., under a durable, profile-scoped persist directory, and each profile gets its own directory. That gives you separation between work/hobby/client contexts without re-authing everything constantly. The persist directory is decoupled from any single sandbox, so it is shared across a profile's sandboxes and survives `rebuild`.

4. Isolated microVM sandbox

The workspace runs inside an `sbx` microVM sandbox rather than as loose host processes. The sandbox owns its own kernel, filesystem, and Docker engine, and its outbound network egress is mediated by the `sbx` sandbox policy layer.

5. Bundled workflow harnesses

Rally and Laps make the environment more than a generic sandbox. Rally belongs inside the environment as the agent-in-loop orchestration layer. Laps belongs as the lightweight task-state substrate that agents can use without a service dependency. Dune should install and preserve the substrate; it should not absorb their responsibilities.

## Prerequisites

`dune` runs workspaces inside an `sbx` microVM sandbox, so the host needs `sbx` rather than Docker:

- `sbx` installed and on `PATH`
- `sbx` authenticated and its daemon healthy (`sbx diagnose` reports every check passing)
- a supported `sbx` version — currently `v0.32.0` or newer

`dune` validates all of this before any sandbox operation and errors clearly if `sbx` is missing, unhealthy, unauthenticated, or too old. Docker is no longer a host requirement for `dune`.

## Installation

Install the latest release with one command:

```bash
curl -fsSL https://raw.githubusercontent.com/mitchell-wallace/dune/main/install.sh | bash
```

Or install a specific version:

```bash
curl -fsSL https://raw.githubusercontent.com/mitchell-wallace/dune/main/install.sh | bash -s 0.3.2
```

The installer downloads the correct binary for your platform, places it in `~/.local/bin`, and updates your shell configuration to include it on `PATH`.

For local development from this repo, build the binary with `./scripts/build-dune.sh --force --print-path` or install the alias with `./scripts/install-dune-alias.sh`.

If `dune` seems to be running the wrong thing on your machine, check `type -a dune`. A shell alias created by `./scripts/install-dune-alias.sh` will override a standalone binary on `PATH`.

Then run:

```sh
dune
dune up
dune down
dune rebuild
dune logs
dune version
dune profile set work
dune profile list
```

`dune` resolves the workspace root from `git rev-parse --show-toplevel` and falls back to the current directory outside a git repo. It names each workspace's sandbox `dune-<workspace-slug>-<profile>` and stores profile-scoped persisted state under `~/.local/share/dune/persist/<profile>` (`$XDG_DATA_HOME/dune/persist/<profile>` if set).

## Commands

- `dune` starts the sandbox for the current repo if needed, then opens an interactive `zsh` shell at the repository root inside the sandbox
- `dune up` does the same thing explicitly
- `dune down` stops the sandbox for the current workspace/profile (the sandbox is retained; it is not removed)
- `dune rebuild` recreates and starts the sandbox from the Dune sbx template, preserving the profile's persisted state
- `dune logs` streams Dune runtime logs for the current sandbox
- `dune logs <service>` streams a single named log under the sandbox's `/var/log/dune/` path
- `dune version` prints the dune version, commit, and release build metadata
- `dune -v` / `dune --version` is a shorthand for `dune version`
- `dune -h` / `dune --help` shows usage information
- `dune -u` / `dune --update` updates the dune CLI to the latest release
- `dune profile set <name>` stores a profile mapping for the current workspace root
- `dune profile list` shows the effective profile for the current workspace and any stored mappings

## What Dune Sets Up

When you run `dune`, it:

- resolves the workspace root from the current directory
- selects a profile, defaulting to `default`
- validates that `sbx` is installed, authenticated, daemon-healthy, and meets the minimum version
- ensures the profile's persist directory exists
- creates the sandbox from the Dune sbx template (if it does not already exist), direct-mounting the workspace root and the profile persist directory
- starts the sandbox if it is stopped
- attaches an interactive `zsh` shell with its working directory set to the repository root

## Profiles and persistence

Profiles are string names such as `default`, `work`, or `personal`.

- `dune profile set <name>` stores a directory-to-profile mapping in `~/.config/dune/profiles.json`
- `--profile` / `-p` overrides the stored mapping for a given command
- Each profile gets its own durable persist directory: `~/.local/share/dune/persist/<profile>` (`$XDG_DATA_HOME/dune/persist/<profile>` if set)

The persist directory replaces the old `dune-persist-<profile>` Docker volume with the same semantics: it is profile-scoped (shared across every workspace using that profile), decoupled from any single sandbox, and survives `rebuild`. The sbx template seeds and persists these home-directory paths through `/persist/agent`:

- `~/.claude/`
- `~/.codex/`
- `~/.config/opencode/`
- `~/.local/share/opencode/`
- `~/.config/gh/`
- `~/.gitconfig`
- `~/.git-credentials`
- `~/.zshrc`
- `~/.p10k.zsh`

## Customization

There is no `Dockerfile.dune`. The repo-specific image build path that detected a `Dockerfile.dune` at the workspace root has been dropped with the Docker Compose backend; `dune` always launches the published Dune sbx template (or a locally loaded template ref).

The Dune sbx template is built from source under [`container/sbx/`](./container/sbx); see [Dune sbx Template — Contents, Persistence, and Workspace](./docs/architecture/sbx-template.md) and [Distribution, Versioning, and Registry Access](./docs/architecture/sbx-template-distribution.md) for how it is built, tagged, and consumed. `sbx` kits as the per-workspace customization layer land in `sbx-6`.

## Included Tools

The sbx template is meant to be ready to use without a separate bootstrap step. It includes (the canonical, maintained list lives in [the sbx template doc](./docs/architecture/sbx-template.md)):

- `claude`: Anthropic's Claude Code CLI
- `codex`: OpenAI Codex CLI
- `opencode`: Opencode CLI
- `gemini`: Gemini CLI
- `rally`: Ralph-loop based agent runner that ships with dune
- `laps`: Minimal CLI-based sequential task manager for agents
- `openspec`: CLI for the OpenSpec planning framework
- `gh`: GitHub CLI for repository, auth, PR, and release workflows
- `git`: source control inside the sandbox
- `delta`: syntax-highlighted Git pager for diffs
- `tmux`: terminal multiplexer for long-lived sessions
- `zsh` with Powerlevel10k: the default interactive shell and prompt
- `vim`, `nano`, `micro`: terminal editors with different levels of complexity
- `ripgrep`: fast recursive code search
- `fd`: friendly file finder
- `fzf`: fuzzy finder for shell navigation and filtering
- `bat`: `cat` with syntax highlighting and paging
- `eza`: modern replacement for `ls`
- `tre`: modern directory tree viewer
- `jq`: JSON query and formatting tool
- `curl`: HTTP client for APIs and downloads
- `ping`: the classic network testing primitive
- `Node.js` and `npm`: JavaScript runtime and package manager used by several CLIs and builds
- `pnpm`: fast JavaScript package manager
- `turbo`: Turborepo build orchestration CLI
- `mise`: runtime manager used to provide current language toolchains in the shell
- `go`: Go toolchain installed through `mise`
- `python`: Python runtime installed through `mise`
- `uv`: fast Python package and environment tool installed through `mise`
- `playwright` with Chromium: browser automation and web testing stack
- `postgresql-client` (`psql`): Postgres client for reaching a project-owned Postgres
- `redis-tools` (`redis-cli`): Redis client for reaching a project-owned Redis
- `docker` and `docker compose`: nested Docker engine inside the sandbox for project-owned service stacks
- `sudo`: passwordless sudo for the `agent` user when elevated commands are needed

App-level service dependencies (Postgres, Redis, Mailpit, etc.) are no longer auto-started inside the workspace. Bring your own `docker compose` stack inside the sandbox; the template ships the clients so you can reach those services from a shell.

## Rally

[Rally](https://github.com/mitchell-wallace/rally) ships in the sbx template and is available inside every dune workspace.

- Rally is a Ralph-loop based agent runner that comes with dune
- Rally configuration lives in `rally.toml` at the workspace root
- The sbx template installs `rally` from GitHub Releases
- `rally` can update itself independently inside the sandbox

## Networking

The workspace runs inside an `sbx` microVM sandbox with its own kernel and network namespace. The sandbox does not bypass `sbx` mediation: outbound egress is governed by the `sbx` sandbox policy layer, not by a host-side proxy. The Dune network-policy baseline, domain-opening affordance, and `sbx policy log` source are tracked in `sbx-4-sbx-network-and-secrets`. `dune logs` reads the sandbox's `/var/log/dune/` log directory via `sbx exec`.

## Migrating from Docker Compose workspaces

The sbx runtime is a deliberate breaking hard-cut from the previous multi-container Docker Compose topology. Existing Compose workspaces are **not** migrated automatically:

- the `dune-persist-<profile>` Docker volumes are replaced by the profile persist directory under `~/.local/share/dune/persist/<profile>`; Dune does not copy volume contents across, so re-auth agent CLIs on first use of a profile
- orphaned Compose containers, volumes, networks, and generated `compose.yaml` files under `~/.local/share/dune/projects/` are left in place — full cleanup of stale Docker artifacts is part of `sbx-6-sbx-kits-and-cleanup`
- `Dockerfile.dune` files at repo roots are no longer detected or built

## Development

Useful checks:

```sh
go test ./...
go build ./cmd/dune
./.bin/dune profile list
```

`test/smoke/sbx-runtime.sh` exercises the real `sbx` runtime end to end against the Dune sbx template: it confirms the attached shell's working directory is the mounted repository root and that the profile persist directory survives a sandbox recreate.
