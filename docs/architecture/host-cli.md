# Host CLI

The host-side `sand` control plane now lives in Go under `cmd/sand`.

Responsibilities:

- parse `sand`, `sand config`, and `sand rebuild`
- discover and merge `sand.toml`
- generate effective devcontainer config for `workspace_mode=copy`
- invoke `npx @devcontainers/cli up`
- rename and reconcile workspace containers
- apply configured addons after startup

Compatibility:

- `sand.sh` remains as a compatibility shim
- profile, mode, workspace mode, and container naming semantics are preserved
