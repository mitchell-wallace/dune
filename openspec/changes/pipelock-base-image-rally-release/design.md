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

## Guiding Principles

When an implementing agent encounters an unforeseen decision during this change, these principles apply:

1. **Every tool must install successfully.** The base image includes a curated, well-refined tool list — boost-cli tools are the only optional category. If a tool install is flaky, consult documentation and use a lightweight ephemeral container (`docker run --rm debian:12-slim ...`) to test the install commands in isolation before running the full image build again. Do not skip or paper over a failing install.
2. **Fail loud, not silent.** If a service can't start, a tool is missing, or a configuration is invalid, the system must surface that clearly — not silently degrade. Prefer explicit errors and early validation over graceful fallbacks that hide problems.

## Decisions

### D1: Docker Compose directly, drop devcontainers

**Decision:** The dune CLI drives `docker compose up/exec/down` directly. devcontainer.json and the `@devcontainers/cli` npm dependency are removed entirely.

**Rationale:** The devcontainer abstraction added complexity (JSON manipulation, postStartCommand orchestration, devcontainer CLI as a Node dependency) without providing value — the tool is used from the terminal via `dune`, not through VS Code's Remote Containers. Docker Compose is sufficient for the two-container topology and is a direct dependency with no intermediary.

**Alternative considered:** Keeping devcontainer.json with `dockerComposeFile` pointing to compose.yaml. Rejected because it preserves a dependency we don't use and adds a layer of indirection between dune and docker.

### D2: Compose file is a Go template, not a static file

**Decision:** The dune CLI generates `compose.yaml` at runtime from an embedded Go template. The generated file is written to a project-specific directory under `~/.local/share/dune/projects/<slug>/compose.yaml`. The slug is `<folderName>-<2hexhash>` where the 2 hex characters are derived from a hash of the absolute workspace path (e.g., `myapp-a3`). This ensures slugs are human-readable and collision-proof.

**Workspace root resolution:** The "workspace path" used for slug derivation, `/workspace` mount target, `Dockerfile.dune` lookup, and Docker build context is defined as the **git repository root** (`git rev-parse --show-toplevel`). If the current directory is not inside a git repo, the workspace path falls back to the current working directory. Running `dune` from any subdirectory within a repo always resolves to the same workspace root. This matches the existing `ResolveRepoRoot()` behaviour in `workspace.go`.

**Rationale:** The compose file needs dynamic values: image name (base vs Dockerfile.dune-built), workspace path, profile-specific volume names, and Pipelock config path. A static file would require sed-style substitution or environment variable interpolation. An embedded template keeps it in Go, testable, and versioned with the CLI binary. The 2-char hash suffix prevents slug collisions when multiple repos share the same folder name (256 possible suffixes is more than sufficient for single-user usage).

**Alternative considered:** Static compose file with `${VAR}` interpolation via a `.env` file. Rejected because compose env interpolation is limited (no conditionals, no computed values) and would still need dune to generate the `.env`. For slug format, `parentDir-folderName` was considered but parent directory names (e.g., `Documents`) are often noise.

### D3: Base image on debian:12-slim, published to GHCR

**Decision:** Build a `debian:12-slim`-based image with all tools pre-installed. The image uses zsh with Powerlevel10k as the default shell (matching the current container UX). Timezone support is included via `tzdata` package and `TZ` environment variable (defaulting to host timezone forwarded by dune, falling back to `UTC`). Publish to GHCR via GitHub Actions CI on pushes to main that touch the image definition. Tag as `ghcr.io/mitchell-wallace/dune-base:latest` and `ghcr.io/mitchell-wallace/dune-base:<sha>`. The CI build must include `--build-arg BUILDKIT_INLINE_CACHE=1` so that `Dockerfile.dune` builds can use `--cache-from` with BuildKit.

**Rationale:** `debian:12-slim` is smaller than `node:22` (which is Debian-based anyway) and gives full control over what's installed. GHCR publishing means `docker pull` on first use is fast and `Dockerfile.dune` can `FROM ghcr.io/<org>/dune-base:latest` with layer caching. SHA tags allow pinning for reproducibility.

**Alternative considered:** Local-only builds with no registry. Rejected because cold start would require building the entire image from scratch (~5-10 min), and `Dockerfile.dune` can't use `--cache-from` without a registry.

### D4: Agent container user is `agent`, credentials persisted per profile via symlinks

**Decision:** The image creates a non-root `agent` user with passwordless sudo and zsh as default shell. A named Docker volume per profile (`dune-persist-<profile>`) is mounted at `/persist/agent` (outside the home directory so the agent only sees symlinks, not the mount). Specific credential and config paths are symlinked from the home directory into the persistent volume:

- `.claude/`, `.codex/`, `.gemini/` — coding agent auth/config
- `.config/opencode/`, `.local/share/opencode/` — Opencode auth/config
- `.config/gh/`, `.gitconfig`, `.git-credentials` — GitHub auth
- `.zshrc`, `.p10k.zsh` — shell configuration

When a new profile is created, defaults from the image are seeded into the profile's persistent volume (so the user gets working `.zshrc`/`.p10k.zsh` etc.), but subsequent boots never overwrite existing files. The default profile is named `default`. No API keys are forwarded as environment variables — all agent CLIs authenticate via OAuth tokens persisted in the volume.

**Rationale:** The primary purpose of persistence is auth sharing — OAuth tokens and credentials for coding agents and GitHub must survive container restarts and rebuilds. Symlinking specific paths (rather than mounting the entire home dir) means the image's baked-in tools, mise shims, Rally binary, and other home-directory contents remain visible without a seeding step. Shell config (`.zshrc`, `.p10k.zsh`) is included so user customisations persist. Per-profile volumes maintain credential isolation between profiles. If a user wants different configs per repo, they use different profiles.

**Alternative considered:** Mount the entire `/home/agent` as a volume. Rejected because on first boot with an empty volume, all image-baked files (mise shims, Rally binary, shell config, agent CLIs) would be hidden behind the mount, requiring a complex rsync-based seeding step. The symlink approach is simpler and more predictable.

### D5: Pipelock in balanced mode with seeded API allowlist

**Decision:** Pipelock runs as a sidecar on both internal and external networks, using a pinned image version (`ghcr.io/luckypipewrench/pipelock:0.x` — pin to specific tag at implementation time). The agent container only connects to the internal network and routes HTTP(S) via `http_proxy`/`https_proxy` environment variables pointing to `pipelock:8888`. The proxy env vars are set in both lowercase and `HTTP_PROXY`/`HTTPS_PROXY` uppercase forms. `no_proxy` and `NO_PROXY` are both set to `localhost,127.0.0.1` to avoid proxying local service traffic.

The baseline Pipelock config is generated via `docker run --rm ghcr.io/luckypipewrench/pipelock:<pinned-tag> generate config --preset balanced` and then customised. Key config fields (using real Pipelock schema):

```yaml
version: 1
mode: balanced
enforce: true
api_allowlist:
  - "*.anthropic.com"
  - "*.openai.com"
  - "*.googleapis.com"
  - "accounts.google.com"
  - "oauth2.googleapis.com"
  - "chatgpt.com"
  - "registry.npmjs.org"
  - "pypi.org"
  - "files.pythonhosted.org"
  - "proxy.golang.org"
  - "crates.io"
  - "mcp.grep.app"
  - "mcp.context7.com"
  - "mcp.exa.ai"
fetch_proxy:
  monitoring:
    blocklist:
      - "*.pastebin.com"
      - "*.hastebin.com"
      - "*.transfer.sh"
      - "file.io"
      - "requestbin.net"
dlp:
  include_defaults: true  # 46 built-in patterns (AWS keys, GitHub tokens, etc.)
response_scanning:
  enabled: true
  action: warn
logging:
  format: json
  output: stdout
```

**Rationale:** Balanced mode provides DLP scanning and rate limiting without hard-blocking legitimate agent traffic. `dlp.include_defaults: true` gives 46 built-in secret detection patterns out of the box, covering Anthropic keys, AWS creds, GitHub tokens, and more — no need to hand-write regex. The `api_allowlist` with wildcards ensures core AI API and package registry traffic is never flagged by heuristics. This is a baseline — logs can be observed and policy tightened later.

**Alternative considered:** Starting in audit-only mode. Rejected because the user wants real protection from day one, just not strict lockdown.

### D6: Dune CLI is a clean rewrite

**Decision:** Rewrite `cmd/dune` and `internal/dune` from scratch. The new CLI has these commands:
- `dune` / `dune up` — start or attach to the agent container for the current directory
- `dune down` — stop the containers
- `dune rebuild` — force rebuild the agent image (useful after Dockerfile.dune changes)
- `dune profile set <name>` — set the profile for the current directory
- `dune profile list` — list profiles and their directory mappings
- `dune logs [service]` — tail compose logs (useful for `dune logs pipelock`)

The only flag is `--profile`/`-p`. The `--directory/-d` flag and `dune config` interactive wizard are dropped — there is nothing to configure beyond profile selection. Profile-to-directory mapping is stored in `~/.config/dune/profiles.json`. The default profile `default` is used when no mapping exists.

No API keys are forwarded as environment variables. Agent CLIs authenticate via OAuth tokens stored in the persisted home volume. The host's `TZ` environment variable is forwarded so container timestamps match the host timezone.

**Rationale:** The existing CLI has ~70% dead code paths after removing gear, modes, dune.toml, devcontainer orchestration, and rally sync. A rewrite is less work than surgical removal and produces cleaner code. The new CLI is small enough (~500 lines) to be written in one pass.

**Alternative considered:** Incremental strip-down of existing code. Rejected because the coupling between config parsing, devcontainer manipulation, and gear management makes clean extraction harder than a fresh start.

### D7: Rally extracted to its own repo with GoReleaser

**Decision:** Move `cmd/rally` and `internal/rally` to a new GitHub repo. Set up GoReleaser to produce cross-platform binaries (linux/darwin, amd64/arm64) published as GitHub Releases. Add an `install.sh` script to the release assets. The container Dockerfile installs Rally via `curl | sh` from the latest release.

Rally gains a `rally update` subcommand that downloads the latest release to `~/.local/bin/rally`. A background version check on startup prints a one-line notice if a newer version is available (suppressible via `RALLY_NO_UPDATE_CHECK=1`).

**Rationale:** Rally is an agent orchestrator that happens to run inside dune containers but has no build-time dependency on dune. Coupling them means every Rally change requires a dune rebuild and binary sync. Independent releases let Rally iterate on its own cadence.

**Alternative considered:** Keep Rally in this repo but add GoReleaser here. Rejected because the build/release lifecycle is fundamentally different — dune is a host tool distributed by cloning, Rally is a container tool distributed via binary releases.

### D8: Dockerfile.dune for per-repo extensions

**Decision:** If a `Dockerfile.dune` exists in the repo/workspace root, `dune` builds it (tagged as `dune-local-<slug>:latest`) using the workspace root as the build context, with `--cache-from ghcr.io/mitchell-wallace/dune-base:latest`. The base image must have been built with `BUILDKIT_INLINE_CACHE=1` for `--cache-from` to work under BuildKit. Dune pulls the base image before building to ensure cache layers are available. The resulting image is used as the agent service image. Otherwise, the base image is used directly.

The `Dockerfile.dune` must `FROM ghcr.io/mitchell-wallace/dune-base:latest` (or whatever the base image tag is). This is a convention, not enforced. `COPY` commands in the Dockerfile.dune are relative to the workspace root.

**Rationale:** Replaces the gear system for repo-specific tools. Docker layer caching means repo-specific builds are fast (only the added layers rebuild). No runtime install, no gear state files, no need for network access during tool setup.

### D9: Pipelock config location and management

**Decision:** Pipelock config lives at `~/.config/dune/pipelock.yaml`. On first run, `dune` generates the baseline by running `docker run --rm ghcr.io/luckypipewrench/pipelock:<pinned-tag> generate config --preset balanced`, then applies customisations (api_allowlist additions, blocklist, logging config) and writes the result. This ensures the config stays compatible with the installed Pipelock version. Users can edit the file to tighten or loosen policy. Pipelock supports hot-reload via file watcher, so config changes take effect without container restart.

**Rationale:** Global config means network policy is consistent across all repos/projects. Using Pipelock's own generator as the baseline avoids maintaining a hand-written config that could drift from the schema. Per-repo overrides are a non-goal for now — this can be added later by allowing a `pipelock.yaml` in the repo root to override or extend the global config.

### D10: Entrypoint and service management via s6-overlay

**Decision:** The agent container uses s6-overlay as PID 1 and process supervisor. PostgreSQL, Redis, and Mailpit are defined as s6 `longrun` service directories under `/etc/s6-overlay/s6-rc.d/`. A `setup-persist` `oneshot` service runs at boot to create symlinks from the home directory into `/persist/agent` and seed defaults for new profiles (see D4). s6 starts all services on container boot and automatically restarts any long-running service that crashes. No `dune-privileged.sh`, no mode configuration, no firewall init.

s6-overlay is installed in the Dockerfile from the official s6-overlay release tarball. Each long-running service gets a `run` script (e.g., `exec postgres -D /var/lib/postgresql/...`) and a `type` file (`longrun`). The `setup-persist` oneshot seeds default files into the persist volume if they don't exist, then creates the symlinks. The container entrypoint is s6's `/init`, which handles PID 1 duties (signal forwarding, zombie reaping), starts all services, and then stays running as the container's long-lived foreground process. The user gets an interactive shell via `docker compose exec agent zsh`.

**Rationale:** With everything pre-installed and no mode gating, the entrypoint is dramatically simpler. s6-overlay adds ~1MB to the image and provides automatic service restart — critical for long-running agent sessions where a crashed postgres would otherwise go unnoticed. s6-overlay is the standard for multi-service Docker containers (linuxserver.io uses it across hundreds of images). The `setup-persist` oneshot replaces the old `setup-agent-persist.sh` with a cleaner s6-native approach.

**Alternative considered:** tini as PID 1 with a simple bash entrypoint. Simpler but no automatic restart — if postgres crashes at hour 3 of an agent session, the user doesn't know until work is lost.

### D11: Rally config moves to rally.toml

**Decision:** Model preferences (`claude_model`, `codex_model`, `gemini_model`, `opencode_model`) and beads configuration currently in `dune.toml` move to `rally.toml`, read directly by Rally. The file lives at the workspace root (`/workspace/rally.toml`). Rally reads the same keys it currently receives via environment variables, just from TOML instead. This file is part of the project codebase and checked into source control — it is not a per-user home directory config.

**Rationale:** Rally is becoming an independent tool. It should own its own configuration rather than receiving it indirectly from dune via env vars. Keeping rally.toml at workspace level means it's versioned with the project, trivially inspectable, and requires no persist-volume symlink management. TOML is already a dependency in the codebase and is the format users are familiar with from `dune.toml`.

### D12: Default mise config for language runtimes

**Decision:** The base image includes a global mise config at `/home/agent/.config/mise/config.toml` that pins `latest` for Node.js, Go, Python, and Rust. mise is installed globally during the Docker build. Shims are placed at `/home/agent/.local/share/mise/shims` and added to PATH in the agent's shell profile. During the Docker build, `mise install` is run as the `agent` user to pre-populate the runtimes so first container start has no install delay.

**Rationale:** mise replaces the old version-pin fields in `dune.toml` (`python_version`, `go_version`, etc.). Using `latest` as the default means the base image always ships with current stable versions. Users can override per-project with a `.mise.toml` in their repo. Running `mise install` at build time ensures runtimes are cached in the image layer.

## Risks / Trade-offs

**[Larger base image]** → All tools pre-installed means a larger image (~2-3 GB estimated). Mitigated by GHCR caching — the image is pulled once and cached locally. Subsequent starts use the local image. The tradeoff is intentional: startup speed and reliability over disk space.

**[Pipelock as single point of network failure]** → If Pipelock crashes, the agent loses all network access. Mitigated by Docker Compose restart policy (`restart: unless-stopped`) on the Pipelock service. Also mitigated by Pipelock being a mature, purpose-built proxy.

**[Persistence symlinks require explicit path list]** → Only the listed paths (agent auth dirs, GitHub creds, shell config) are persisted. A new tool with a non-standard credential path would need to be added to the symlink list in the `setup-persist` oneshot. Mitigated by the fact that the major agent CLIs and GitHub are already covered, and adding a new path is a one-line change in the service script.

**[Rally extraction is a repo split]** → Moving Rally out means two repos to maintain and a dependency between them (container needs Rally releases to exist). Mitigated by the install script being resilient to release unavailability (falls back gracefully) and by `rally update` making it easy to stay current.

**[No rollback path for dune.toml users]** → Users with `dune.toml` files lose their config on upgrade. Mitigated by documentation and the fact that the user base is small (single user). Model prefs and beads config move to `rally.toml`.

**[Proxy-unaware tools]** → Some tools may not respect `http_proxy`/`https_proxy` environment variables (e.g., tools that use raw sockets or have their own DNS resolution). Mitigated by the internal-only network — tools that bypass the proxy simply get no connectivity, which surfaces the issue immediately rather than silently leaking traffic. Both lowercase and uppercase proxy env vars are set to maximise compatibility.

**[s6-overlay learning curve]** → s6 service directories are a different pattern from simple bash scripts. Mitigated by the fact that there are only 3 services to define, each with a trivial `run` script, and s6-overlay is extensively documented.

## Migration Plan

1. **Create Rally repo** — move code, set up GoReleaser, add `rally.toml` config reading, publish first release, verify install script works
2. **Build base image** — write new Dockerfile with s6-overlay, zsh/p10k, mise with default runtimes, all tools pre-installed, timezone support. Build with `BUILDKIT_INLINE_CACHE=1` and publish to GHCR
3. **Configure Pipelock** — generate balanced-mode baseline via `pipelock generate config --preset balanced`, customise allowlist/blocklist, embed in dune CLI
4. **Rewrite dune CLI** — new compose-driven lifecycle, profile management (string names, `folder-2hexhash` slugs), Dockerfile.dune support, TZ forwarding
5. **Write compose template** — embedded Go template for agent + pipelock topology, proxy env vars (both cases), no API key forwarding
6. **Integration test** — run `dune` end-to-end in a test repo, verify agent can reach APIs through proxy, OAuth credentials persist across restarts, services auto-restart via s6
7. **Delete old code** — remove init-firewall.sh, gear system, devcontainer.json, dune.toml parser, security mode logic, dune config wizard, rally code from this repo

## Open Questions

- **Non-HTTP traffic**: Tools like `git` use SSH for `git@github.com:...` remotes. SSH doesn't go through HTTP proxies. Should we configure git to always use HTTPS, or add SSH passthrough? For now, HTTPS is the default git transport and SSH can be added later.
