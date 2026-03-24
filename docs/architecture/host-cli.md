# Host CLI

The host-side `dune` control plane now lives in Go under `cmd/dune`.

Responsibilities:

- parse `dune`, `dune config`, and `dune rebuild`
- discover and merge `sand.toml`
- generate effective devcontainer config for `workspace_mode=copy`
- invoke `npx @devcontainers/cli up`
- rename and reconcile workspace containers
- apply configured gear after startup

Compatibility:

- `sand.sh` remains as a compatibility shim while `dune` becomes the primary host CLI
- profile, mode, workspace mode, and container naming semantics are preserved
