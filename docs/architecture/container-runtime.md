# Container Runtime

The active runtime artifact on this branch is the **Dune sbx template**, not the
Compose base image. The host-side `dune` CLI launches a sandbox built from the
template (`ghcr.io/mitchell-wallace/dune-sbx:<version>`); what the template
contains, the s6 services it deliberately drops, the re-homed `setup-persist`
boot hook, the `/var/log/dune/` log path, and the `/workspace` compatibility
symlink are covered in [Dune sbx Template — Contents, Persistence, and
Workspace](./sbx-template.md).

Responsibilities that move with the sbx template:

- seed default home-directory files into `/persist/agent` once per sandbox via
  the sentinel-guarded login-shell hook
- create the persisted symlinks used by agent CLIs and shell config
- install Rally (and Laps/agy/thenn) from GitHub Releases during the image build
- configure in-container agent tools during the image build
- auto-start the nested Docker engine so project-owned `docker compose` stacks
  work inside the sandbox

In-container app servers (PostgreSQL, Redis, Mailpit) and the `s6-overlay`
supervisor that ran them are deliberately dropped from the sbx template; app
dependencies now come from project-owned Compose stacks inside the sandbox.

The legacy Compose base image (`ghcr.io/mitchell-wallace/dune-base`) and the
[`container/base/`](../../container/base) tree remain until `sbx-6` retires them
— the sbx template reuses their `scripts/`, `tooling.yaml`, and `home-defaults/`
assets but is built from its own [`container/sbx/`](../../container/sbx) inputs.
On `main` the Compose base image is still the active runtime; on this branch it
is retained only as a build-asset source.
