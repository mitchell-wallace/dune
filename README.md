# claudebox

`dune` is a host-side Go CLI that starts a two-container workspace:

- `agent`: the interactive development container
- `pipelock`: the outbound HTTP(S) proxy sidecar

The old devcontainer, gear, firewall-script, and `dune.toml` flow has been removed.

## Usage

Install the alias with `./install-dune-alias.sh`, then run:

```sh
dune
dune up
dune down
dune rebuild
dune logs
dune logs pipelock
dune profile set work
dune profile list
```

`dune` resolves the workspace root from `git rev-parse --show-toplevel` and falls back to the current directory outside a git repo. It stores generated compose files under `~/.local/share/dune/projects/<slug>/compose.yaml`.

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

## Rally

Rally is no longer built or synced from this repo.

- The base image installs `rally` from GitHub Releases
- Rally configuration lives in `rally.toml` at the workspace root
- `rally` can update itself independently inside the container

## Networking

The `agent` container is attached only to an internal Docker network and reaches external HTTP(S) services through the `pipelock` sidecar via proxy environment variables.

Git via SSH is not natively supported in this architecture because SSH traffic does not flow through the HTTP proxy sidecar. Prefer HTTPS remotes inside dune workspaces.

## Development

Useful checks:

```sh
go test ./...
go build ./cmd/dune
./dune.sh profile list
```
