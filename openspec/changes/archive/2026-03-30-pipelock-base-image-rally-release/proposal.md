## Why

The current container sandboxing architecture has compounding maintenance costs: an iptables/ipset firewall with a hand-maintained domain allowlist and DNS refresh daemon, a node:22 base image that's larger than necessary, a runtime gear system that makes cold starts slow and gear state fragile, and Rally tightly coupled to this repo despite being an independent tool. Replacing the firewall with Pipelock as a compose sidecar, building a batteries-included debian:12-slim image, and externalising Rally will make the system simpler to maintain, faster to start, and easier to evolve independently.

## What Changes

- **BREAKING** Replace `init-firewall.sh` (iptables/ipset allowlist + DNS refresh loop) with Pipelock HTTP proxy running as a Docker Compose sidecar in balanced mode. Agent container routes all traffic through `http_proxy`/`https_proxy` env vars. Core domains from the current allowlist are seeded into Pipelock's `api_allowlist`.
- **BREAKING** Rebase container image from `node:22` to `debian:12-slim` with everything pre-installed: all current gear (postgres server+client, redis, playwright, pnpm, turbo, mailpit, python/uv, go, rust, tmux, boost-cli tools), all agent CLIs (Claude Code, Codex, Opencode, Gemini), zsh with Powerlevel10k, and common dev tools (git, jq, ripgrep, mise, etc.). s6-overlay manages services (postgres, redis, mailpit) with automatic restart. A default mise config provides latest stable Node.js, Go, Python, and Rust.
- **BREAKING** Remove the gear system entirely (manifest.tsv, gear-cli.sh, per-gear install scripts, runtime gear detection). Per-repo tool additions move to `Dockerfile.dune`.
- **BREAKING** Remove security modes (std/lax/yolo/strict). No more mode gating — Pipelock provides network policy, sudo is always available.
- **BREAKING** Eliminate `dune.toml`. Model preferences and beads config move to `rally.toml` at the workspace root (Rally reads directly). Version pins are handled by mise or user choice. Profile selection moves to the dune CLI with central storage (folder path → profile name mapping).
- **BREAKING** Restructure profiles: string names instead of single-character IDs, `--profile`/`-p` CLI flag, default profile named `default`, named Docker volumes per profile (e.g. `dune-persist-default`).
- Move Rally to its own GitHub repository with GoReleaser for cross-platform releases. Container installs Rally from GitHub Releases via install script. Rally gains `rally update` for self-updating without sudo.
- Introduce `compose.yaml` for multi-container topology (agent + pipelock). The `dune` CLI drives `docker compose` for container lifecycle.
- Support per-repo `Dockerfile.dune` for repo-specific image extensions (build context is the workspace root), built with layer caching from the base image.
- Retain the credentials persistence approach using a per-profile volume mounted at `/persist/agent` with symlinks from the home directory for specific credential and config paths (`.claude/`, `.codex/`, `.gemini/`, `.config/opencode/`, `.config/gh/`, `.gitconfig`, `.git-credentials`, `.zshrc`, `.p10k.zsh`). OAuth tokens and shell customisations survive restarts without explicit API key forwarding.

## Capabilities

### New Capabilities
- `network-proxy`: Pipelock sidecar configuration, proxy routing, DLP patterns, domain allowlisting, and balanced-mode network policy
- `base-image`: Batteries-included debian:12-slim container image with all tools, runtimes, and agent CLIs pre-installed
- `compose-lifecycle`: Docker Compose-based container lifecycle management (agent + pipelock topology, volume management, per-repo image builds)
- `profile-management`: String-named profiles with central folder→profile mapping, per-profile credential volumes, and CLI-driven selection
- `rally-release`: GoReleaser-based release pipeline for Rally as an independent binary with self-update capability

### Modified Capabilities
<!-- No existing specs to modify — this is the first use of OpenSpec in this project -->

## Impact

- **Container runtime**: Two-container topology replaces single container. Agent loses direct internet access; all HTTP(S) goes through Pipelock proxy.
- **Container image**: Full rebuild from scratch. Significantly larger base image (all tools pre-installed) but eliminates runtime install delays. Layer caching via `Dockerfile.dune` keeps repo-specific builds fast. s6-overlay provides process supervision for postgres/redis/mailpit.
- **dune CLI**: Clean rewrite — drops devcontainer CLI orchestration, config file parsing, gear system, security modes, `dune config` wizard, `--directory/-d` flag, and rally sync commands. Adds `--profile` flag with string names and compose management. External UX goal: `dune` in a repo should still "just work."
- **Rally codebase**: Extracted from this repo entirely. Becomes a standalone Go project with its own release pipeline. Container installs from GitHub Releases instead of host binary sync. Model prefs and beads config move to `rally.toml`.
- **Removed code**: `init-firewall.sh`, `firewall-domains.tsv`, DNS refresh daemon, gear system (manifest.tsv, gear-cli.sh, all `add-*.sh` scripts), security mode logic, `dune.toml` parser, `dune-privileged.sh` mode configuration, `dune config` interactive wizard.
- **Config migration**: `dune.toml` is eliminated. Model prefs and beads move to `rally.toml` at the workspace root (including `rally init` write path). Gear moves to `Dockerfile.dune` if non-default tools are needed. Profile `0` becomes `default`. Credentials re-authenticated via OAuth on first use (persist volume at `/persist/agent` with symlinks, replacing `/persist/agent` direct mounts).
- **Dependencies**: New external dependency on Pipelock container image (`ghcr.io/luckypipewrench/pipelock:<pinned-tag>`). New dependency on GoReleaser for Rally releases. New dependency on s6-overlay for container process supervision.
