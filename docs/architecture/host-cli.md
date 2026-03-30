# Host CLI

The host-side `dune` control plane now lives in Go under `cmd/dune`.

Responsibilities:

- parse `dune`, `dune up`, `dune down`, `dune rebuild`, `dune logs`, and `dune profile ...`
- resolve the workspace root from git or cwd fallback
- read and write `~/.config/dune/profiles.json`
- generate `compose.yaml` from the embedded template
- ensure the first-run Pipelock config exists at `~/.config/dune/pipelock.yaml`
- start and manage the compose project for the `agent` and `pipelock` services

Compatibility:

- profile mapping semantics are preserved through the rewrite, but the old mode, gear, and devcontainer flows are gone
