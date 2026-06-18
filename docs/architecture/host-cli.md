# Host CLI

The host-side `dune` control plane lives in Go under `cmd/dune`, with the sbx
execution layer in `internal/dune/runtime/sbx`.

The CLI follows a `resolve → build Spec → execute` flow:

- parse `dune`, `dune up`, `dune down`, `dune rebuild`, `dune logs`, and `dune profile ...`
- resolve the workspace root from git or cwd fallback
- read and write `~/.config/dune/profiles.json`
- build a backend-agnostic `Spec` (instance name, workspace host path, profile,
  template ref, working dir, shell, timezone) and hand it to the sbx backend
- validate the host `sbx` install (on `PATH`, `sbx diagnose --output json` all
  checks passing, installed version at the minimum) before any sandbox operation
- map the instance to a sandbox named `dune-<slug>-<profile>` and drive
  `sbx create`/`run`/`exec`/`stop`/`rm`/`ls` through the backend; the concrete
  sbx command shapes stay private to the package and are pinned by tests

Docker is no longer a host requirement; `sbx` (installed, authenticated, daemon
healthy, minimum version) is the prerequisite. The Dune sbx template ref comes
from `version.SbxTemplateRef()` (`container/sbx/IMAGE_VERSION`). Profile-scoped
persisted state lives under `~/.local/share/dune/persist/<profile>`, replacing
the old `dune-persist-<profile>` Docker volume with the same semantics.

Compatibility:

- profile mapping, workspace resolution, profile-name validation, timezone
  resolution, and self-update are preserved through the rewrite; the Docker
  Compose topology, the generated `compose.yaml`, the `Dockerfile.dune` build,
  and the mode/gear/devcontainer flows are gone
