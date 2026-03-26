## Context

Dune (`mitchell-wallace/dune`) provides sandboxed containers for running AI coding agents. The current architecture uses a single container with an iptables firewall (domain allowlist + DNS refresh daemon), a `node:22` base image, a runtime gear system for optional tool installation, and Rally (agent orchestrator) coupled into this repo.

This is being replaced with: Pipelock proxy sidecar for network policy, a batteries-included `debian:12-slim` image, Docker Compose for multi-container orchestration, and Rally extracted to its own repo. The dune CLI is rewritten as a thin compose driver.

**Current state being replaced:**
- Container lifecycle: `npx @devcontainers/cli up` driven by `devcontainer.json`
- Network: `init-firewall.sh` with iptables/ipset, `firewall-domains.tsv`, DNS refresh loop
- Image: `node:22` with runtime gear installs
- Config: `dune.toml` (profile, mode, gear, model prefs, version pins)
- Rally: Built on host, synced into container via `dune rally build/update`

## Goals / Non-Goals

**Goals:**
- Replace iptables firewall with Pipelock HTTP proxy sidecar — real DLP without maintaining domain/IP lists
- Build a single batteries-included base image with all tools pre-installed — no runtime install delays
- Drive container lifecycle via `docker compose` directly from the dune CLI — drop devcontainers dependency
- Extract Rally to its own repo with GoReleaser releases and `rally update` self-updating
- Simplify profiles to string names with a `--profile` / `-p` CLI flag
- Publish base image to GHCR for fast pulls and `Dockerfile.dune` extensibility
- Persist the agent's entire home directory per profile for credential/config survival

**Non-Goals:**
- Strict network lockdown — Pipelock runs in balanced mode; tightening comes later after observing logs
- VS Code Remote Containers integration — devcontainer.json is dropped, not preserved
- Runtime tool installation — everything is in the base image or `Dockerfile.dune`; no gear system
- Per-repo config files — `dune.toml` is eliminated; all dune config is CLI-driven and centrally stored
- Rally feature changes — this change extracts Rally as-is; new Rally features are out of scope

## Decisions

### D1: Docker Compose directly, drop devcontainers

**Decision:** The dune CLI drives `docker compose up/exec/down` directly. devcontainer.json and the `@devcontainers/cli` npm dependency are removed entirely.

**Rationale:** The devcontainer abstraction added complexity (JSON manipulation, postStartCommand orchestration, devcontainer CLI as a Node dependency) without providing value — the tool is used from the terminal via `dune`, not through VS Code's Remote Containers. Docker Compose is sufficient for the two-container topology and is a direct dependency with no intermediary.

**Alternative considered:** Keeping devcontainer.json with `dockerComposeFile` pointing to compose.yaml. Rejected because it preserves a dependency we don't use and adds a layer of indirection between dune and docker.

### D2: Compose file is a Go template, not a static file

**Decision:** The dune CLI generates `compose.yaml` at runtime from an embedded Go template. The generated file is written to a project-specific directory under `~/.local/share/dune/projects/<slug>/compose.yaml`.

**Rationale:** The compose file needs dynamic values: image name (base vs Dockerfile.dune-built), workspace path, profile-specific volume names, API key forwarding, and Pipelock config path. A static file would require sed-style substitution or environment variable interpolation. An embedded template keeps it in Go, testable, and versioned with the CLI binary.

**Alternative considered:** Static compose file with `${VAR}` interpolation via a `.env` file. Rejected because compose env interpolation is limited (no conditionals, no computed values) and would still need dune to generate the `.env`.

### D3: Base image on debian:12-slim, published to GHCR

**Decision:** Build a `debian:12-slim`-based image with all tools pre-installed. Publish to GHCR via GitHub Actions CI on pushes to main that touch the image definition. Tag as `ghcr.io/mitchell-wallace/dune-base:latest` and `ghcr.io/mitchell-wallace/dune-base:<sha>`.

**Rationale:** `debian:12-slim` is smaller than `node:22` (which is Debian-based anyway) and gives full control over what's installed. GHCR publishing means `docker pull` on first use is fast and `Dockerfile.dune` can `FROM ghcr.io/<org>/dune-base:latest` with layer caching. SHA tags allow pinning for reproducibility.

**Alternative considered:** Local-only builds with no registry. Rejected because cold start would require building the entire image from scratch (~5-10 min), and `Dockerfile.dune` can't use `--cache-from` without a registry.

### D4: Agent container user is `agent`, home dir persisted per profile

**Decision:** The image creates a non-root `agent` user with passwordless sudo. A named Docker volume per profile (`dune-home-<profile>`) is mounted at `/home/agent`. The default profile is named `default`.

**Rationale:** Persisting the entire home directory catches all tool credential paths regardless of convention (`~/.config/`, `~/.codex/`, `~/.gemini/`, etc.) without needing explicit mounts for each. Per-profile volumes maintain credential isolation between profiles.

**Alternative considered:** Mount only `~/.config` with symlinks for non-conforming tools. Rejected because it's fragile — every new tool with a non-standard credential path would need a symlink added to the Dockerfile.

### D5: Pipelock in balanced mode with seeded API allowlist

**Decision:** Pipelock runs as a sidecar on both internal and external networks. The agent container only connects to the internal network and routes HTTP(S) via `http_proxy`/`https_proxy` environment variables pointing to `pipelock:8888`. Pipelock config uses balanced mode with `enforce: true`, DLP patterns for common secrets (API keys, AWS creds, GitHub tokens), and an `api_allowlist` seeded from core domains in the current firewall allowlist.

**Core allowlist domains to seed:** `api.anthropic.com`, `statsig.anthropic.com`, `api.openai.com`, `auth.openai.com`, `chatgpt.com`, `generativelanguage.googleapis.com`, `accounts.google.com`, `oauth2.googleapis.com`, `registry.npmjs.org`, `pypi.org`, `files.pythonhosted.org`, `proxy.golang.org`, `crates.io`, `mcp.grep.app`, `mcp.context7.com`, `mcp.exa.ai`.

**Rationale:** Balanced mode provides DLP scanning and rate limiting without hard-blocking legitimate agent traffic. The allowlist ensures core AI API and package registry traffic is never flagged by heuristics. This is a baseline — logs can be observed and policy tightened later.

**Alternative considered:** Starting in audit-only mode. Rejected because the user wants real protection from day one, just not strict lockdown.

### D6: Dune CLI is a clean rewrite

**Decision:** Rewrite `cmd/dune` and `internal/dune` from scratch. The new CLI has these commands:
- `dune` / `dune up` — start or attach to the agent container for the current directory
- `dune down` — stop the containers
- `dune rebuild` — force rebuild the agent image (useful after Dockerfile.dune changes)
- `dune profile set <name>` — set the profile for the current directory
- `dune profile list` — list profiles and their directory mappings
- `dune logs [service]` — tail compose logs (useful for `dune logs pipelock`)

Profile-to-directory mapping is stored in `~/.config/dune/profiles.json`. The default profile `default` is used when no mapping exists.

**Rationale:** The existing CLI has ~70% dead code paths after removing gear, modes, dune.toml, devcontainer orchestration, and rally sync. A rewrite is less work than surgical removal and produces cleaner code. The new CLI is small enough (~500 lines) to be written in one pass.

**Alternative considered:** Incremental strip-down of existing code. Rejected because the coupling between config parsing, devcontainer manipulation, and gear management makes clean extraction harder than a fresh start.

### D7: Rally extracted to its own repo with GoReleaser

**Decision:** Move `cmd/rally` and `internal/rally` to a new GitHub repo. Set up GoReleaser to produce cross-platform binaries (linux/darwin, amd64/arm64) published as GitHub Releases. Add an `install.sh` script to the release assets. The container Dockerfile installs Rally via `curl | sh` from the latest release.

Rally gains a `rally update` subcommand that downloads the latest release to `~/.local/bin/rally`. A background version check on startup prints a one-line notice if a newer version is available (suppressible via `RALLY_NO_UPDATE_CHECK=1`).

**Rationale:** Rally is an agent orchestrator that happens to run inside dune containers but has no build-time dependency on dune. Coupling them means every Rally change requires a dune rebuild and binary sync. Independent releases let Rally iterate on its own cadence.

**Alternative considered:** Keep Rally in this repo but add GoReleaser here. Rejected because the build/release lifecycle is fundamentally different — dune is a host tool distributed by cloning, Rally is a container tool distributed via binary releases.

### D8: Dockerfile.dune for per-repo extensions

**Decision:** If a `Dockerfile.dune` exists in the repo root, `dune` builds it (tagged as `dune-local-<slug>:latest`) using `--cache-from ghcr.io/mitchell-wallace/dune-base:latest` and uses the resulting image as the agent service image. Otherwise, the base image is used directly.

The `Dockerfile.dune` must `FROM ghcr.io/mitchell-wallace/dune-base:latest` (or whatever the base image tag is). This is a convention, not enforced.

**Rationale:** Replaces the gear system for repo-specific tools. Docker layer caching means repo-specific builds are fast (only the added layers rebuild). No runtime install, no gear state files, no need for network access during tool setup.

### D9: Pipelock config location and management

**Decision:** Pipelock config lives at `~/.config/dune/pipelock.yaml`. It is generated by `dune` on first run from an embedded default config (the balanced-mode template with seeded allowlist). Users can edit it to tighten or loosen policy.

**Rationale:** Global config means network policy is consistent across all repos/projects. Per-repo overrides are a non-goal for now — this can be added later by allowing a `pipelock.yaml` in the repo root to override or extend the global config.

### D10: Entrypoint and service management

**Decision:** The agent container entrypoint starts essential services (postgres, redis, mailpit) via a simple init script, then drops to the `agent` user's shell. No `dune-privileged.sh`, no mode configuration, no firewall init. Services are started unconditionally since they're always installed.

**Rationale:** With everything pre-installed and no mode gating, the entrypoint is dramatically simpler. Services that were conditional on gear presence are now always available.

## Risks / Trade-offs

**[Larger base image]** → All tools pre-installed means a larger image (~2-3 GB estimated). Mitigated by GHCR caching — the image is pulled once and cached locally. Subsequent starts use the local image. The tradeoff is intentional: startup speed and reliability over disk space.

**[Pipelock as single point of network failure]** → If Pipelock crashes, the agent loses all network access. Mitigated by Docker Compose restart policy (`restart: unless-stopped`) on the Pipelock service. Also mitigated by Pipelock being a mature, purpose-built proxy.

**[Home directory persistence catches too much state]** → Persisting all of `/home/agent` may persist stale tool caches or broken configs between container rebuilds. Mitigated by `dune rebuild` which recreates the container (but preserves the volume). Users can delete the volume for a clean slate. In practice, tool configs and credentials are the main things persisted, and stale caches are rarely harmful.

**[Rally extraction is a repo split]** → Moving Rally out means two repos to maintain and a dependency between them (container needs Rally releases to exist). Mitigated by the install script being resilient to release unavailability (falls back gracefully) and by `rally update` making it easy to stay current.

**[No rollback path for dune.toml users]** → Users with `dune.toml` files lose their config on upgrade. Mitigated by documentation and the fact that the user base is small (single user). Model prefs and beads config move to Rally's own config, which Rally already partially supports.

**[Proxy-unaware tools]** → Some tools may not respect `http_proxy`/`https_proxy` environment variables (e.g., tools that use raw sockets or have their own DNS resolution). Mitigated by the internal-only network — tools that bypass the proxy simply get no connectivity, which surfaces the issue immediately rather than silently leaking traffic.

## Migration Plan

1. **Create Rally repo** — move code, set up GoReleaser, publish first release, verify install script works
2. **Build base image** — write new Dockerfile, build and publish to GHCR, verify all tools work
3. **Configure Pipelock** — generate balanced-mode config, seed allowlist, test proxy routing
4. **Rewrite dune CLI** — new compose-driven lifecycle, profile management, Dockerfile.dune support
5. **Write compose template** — embedded Go template for agent + pipelock topology
6. **Integration test** — run `dune` end-to-end in a test repo, verify agent can reach APIs through proxy, credentials persist across restarts
7. **Delete old code** — remove init-firewall.sh, gear system, devcontainer.json, dune.toml parser, security mode logic, rally code from this repo

## Open Questions

- **Pipelock image tag pinning**: Should we pin to a specific Pipelock version tag rather than `:latest` to avoid surprise breakage? Likely yes, but need to check available tags.
- **Non-HTTP traffic**: Tools like `git` use SSH for `git@github.com:...` remotes. SSH doesn't go through HTTP proxies. Should we configure git to always use HTTPS, or add SSH passthrough? For now, HTTPS is the default git transport and SSH can be added later.
- **Compose project naming**: Docker Compose uses the directory name as project name by default. Since compose files are generated to `~/.local/share/dune/projects/<slug>/`, the project name needs to be set explicitly via `-p` to avoid collisions. The slug should include the profile name.
