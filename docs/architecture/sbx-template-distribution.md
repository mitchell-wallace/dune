# Dune sbx Template — Distribution, Versioning, and Registry Access

This doc covers how the Dune sbx template image is referenced, versioned, built,
published, and consumed: registry and offline template usage, and the secret
boundaries (design `sbx-2` D6/D7). The CLI runtime that launches the template
lands in `sbx-3-sbx-runtime-backend`; the broader "what the template contains"
and migration narrative lives in [Container Runtime](./container-runtime.md).

The legacy Compose base image (`ghcr.io/mitchell-wallace/dune-base`) and its
publish job remain until `sbx-6` retires them.

## Image reference and versioning (D6)

The Dune sbx template is published as its own image reference:

```
ghcr.io/mitchell-wallace/dune-sbx:<version>
```

It is the durable runtime artifact the sbx backend launches via
`sbx create --template <ref>`. The version is sourced from
[`container/sbx/IMAGE_VERSION`](../../container/sbx/IMAGE_VERSION), kept in
lockstep with the CLI [`VERSION`](../../VERSION) file. Both move together on
every release (the version-bump convention recorded in
[`AGENTS.md`](../../AGENTS.md)).

The sbx-3 backend reads this ref through a `version.SbxTemplateRef()` accessor
(`sbx-3` D10) that mirrors `version.BaseImageRef()`: an ldflags-injectable repo
+ version, with the version falling back to a source-file read of
`container/sbx/IMAGE_VERSION` (not the top-level `VERSION`). "Lockstep with
`VERSION`" is the release convention, not the literal file the accessor reads.

## Build and publish

The build inputs live under [`container/sbx/`](../../container/sbx)
(`Dockerfile.sbx` + `etc/`), built from the repo root:

```sh
docker build -f container/sbx/Dockerfile.sbx -t ghcr.io/mitchell-wallace/dune-sbx:dev .
```

Publishing is automated by the `image` workflow
([`.github/workflows/image.yml`](../../.github/workflows/image.yml)), which has
two jobs for the template mirroring the base-image jobs:

- `verify-template` — builds the template on pull requests that touch the sbx
  build inputs (`container/sbx/**`) or the workflow file.
- `build-and-push-template` — on `main`, builds and pushes
  `ghcr.io/mitchell-wallace/dune-sbx:<version>` and `:latest` to GHCR. It is
  gated on `github.ref == 'refs/heads/main'` and triggers automatically on
  pushes that touch `container/sbx/IMAGE_VERSION`, plus `workflow_dispatch`.

> **Authoring the workflow is not the same as publishing.** The actual GHCR
> push happens when the workflow runs on `main` (see
> [`AGENTS.md`](../../AGENTS.md) — *Manually publishing the base image*). If a
> push event is missed, recover with a manual dispatch:
> `gh workflow run image.yml --ref main`.

The template is **built from source only**. It is never produced from a sandbox
snapshot, and no secrets are baked in (D7, below).

## Consuming the template

### From a registry (default)

`sbx create` pulls the template image directly from the registry when you pass
`--template`/`-t`. The verified create shape (sandbox name on `--name`, the
`shell` agent and the absolute host mount path positional):

```sh
sbx create --name my-dune \
  --template ghcr.io/mitchell-wallace/dune-sbx:<version> \
  shell /absolute/path/to/repo
```

For a private template registry (e.g. a private GHCR package), store pull
credentials first — see [Registry credentials](#registry-credentials).

### Offline / local builds via `sbx template load`

A **host-local Docker image name is not usable as `--template`**: the sbx runtime
has its own image store, and `sbx create` against a local image name fails with
`pull failed` (confirmed in spike 4). The verified offline/local-build path is
to export the image to a tarball and load it into the sbx runtime's store:

```sh
# Build and export from Docker
docker build -f container/sbx/Dockerfile.sbx -t dune-sbx:dev .
docker save dune-sbx:dev -o /tmp/dune-sbx.tar

# Load into the sbx runtime's image store, then use the loaded ref
sbx template load /tmp/dune-sbx.tar
sbx create --name my-dune --template dune-sbx:dev shell /absolute/path/to/repo
```

`sbx template load FILE` loads an image tarball (typically produced by
`docker save` or `sbx template save`) into the sbx runtime's image store; the
loaded ref is then accepted by `--template`. List and remove loaded templates
with `sbx template ls` and `sbx template rm`. This is the path for air-gapped
hosts and for iterating on a locally built template without pushing to a
registry.

## Registry credentials

`sbx secret set --registry` stores pull credentials for a container registry,
used to pull private template images (and kit artifacts) before sandbox
creation. The credentials are host-only by default (used for pulls, not injected
into sandboxes); `-g`/`--global` additionally writes them as
`~/.docker/config.json` inside every new sandbox.

```sh
# Host-only: used for template/kit pulls, not injected into sandboxes
gh auth token | sbx secret set --registry ghcr.io --password-stdin

# Global: host pulls AND injected into every new sandbox
gh auth token | sbx secret set -g --registry ghcr.io --password-stdin

# Scoped to a single sandbox (name is the first argument)
gh auth token | sbx secret set my-dune --registry ghcr.io --password-stdin
```

`--username` is available for username/password auth (omit it for token-only
auth, as with `gh auth token`). Manage stored credentials with `sbx secret ls`
and `sbx secret rm`.

## The template is not a secret boundary (D7)

`sbx template save` of a configured sandbox captures the **whole filesystem**,
including any manually stored secrets. The Dune sbx template is therefore:

- **Built from source only** — from `container/sbx/Dockerfile.sbx` in this repo,
  via the `image` workflow. It is never produced via `sbx template save`.
- **Not a secret boundary** — no secrets are baked in. Credentials and tokens
  are injected at runtime by the backend (e.g. `sbx secret set`) or supplied by
  the profile-persist volume, never embedded in the published image.

Do not store secrets in a sandbox you intend to snapshot, and do not derive a
custom Dune template from a `sbx template save` of a configured sandbox.
