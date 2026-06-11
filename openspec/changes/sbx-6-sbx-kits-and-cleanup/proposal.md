## Status

**Partly optional / can be split.** This is the maturity/teardown phase of the sbx migration and should land only after the sbx backend has proven parity in real use. The kit-customization and the Docker-teardown halves can ship independently if useful.

## Why

After the sbx backend reaches parity (`sbx-2` through `sbx-5`), two follow-on concerns remain.

First, **customization**. `Dockerfile.dune` was dropped in `sbx-3` and not migrated. The `sbx` docs describe **kits** — experimental YAML artifacts that layer commands, env vars, files, network rules, credentials, and memory on top of a template — as the natural successor for per-project/team customization, with the heavy Dune template (`sbx-2`) as the base. Spike 3 confirmed templates and kits are designed to work together (a heavy template as the base, thin kits for config/credentials/network rules). This is also the natural home for the Dune-recommended docs-domain network rules flagged in `sbx-4`.

Second, **teardown**. Once the sbx path is the sole backend, the Docker Compose code, the legacy `dune-base` Compose image build (root `Dockerfile`, `container/base/`, the `image.yml` base build job), and related test scaffolding should be removed so Dune does not carry a dead backend indefinitely. Stale local Docker artifacts left on users' machines from the pre-migration backend (the `dune-persist-<profile>` volumes, `dune-local-<slug>` images, generated compose files) also need a cleanup story.

This proposal scopes **kits as the customization layer** and the **teardown/cleanup** of the legacy Docker path. It assumes everything `sbx-2`..`sbx-5` deliver.

## What Changes

- **NEW** Establish `sbx` kits as the customization story that replaces `Dockerfile.dune`: thin kits layering network rules, credentials, files, and per-project/team additions on top of the Dune sbx template. Decide which kit types Dune ships or recommends (agent kits vs. mixin kits) and where kit definitions live (e.g. per-repo, alongside `rally.toml`).
- **NEW** (optional) A Dune-recommended default kit or documented kit recipe for common docs-domain network rules, addressing the `Balanced`-blocks-docs friction from `sbx-4`.
- **NEW** A template refresh/versioning strategy in a kit-aware world: how the Dune sbx template is updated and republished, and how kit definitions relate to template versions. This formalizes the version lockstep from `sbx-2` D6 (`VERSION` + the template's `IMAGE_VERSION`), and retires the base image's role in that lockstep.
- **REMOVE** the remaining Docker Compose scaffolding once the sbx backend is the only backend: any leftover compose rendering/helpers/golden+validation test fixtures not already removed in `sbx-3`, the legacy `dune-base` Compose image (root `Dockerfile`, `container/base/`), and the `image.yml` base build-and-push job.
- **NEW** A cleanup story for stale local Docker artifacts from the pre-migration backend (`dune-persist-<profile>` volumes, `dune-local-<slug>` images, generated compose files): either documented manual steps or a small `dune` helper, with clear user-facing migration notes.

### Non-goals (explicitly deferred)

- Anything `sbx-2`..`sbx-5` own (template, backend, egress/secrets, lifecycle/diagnostics/doctor).
- Reintroducing a dual-backend model; the Docker path is being retired, not preserved.
- Building a large kit framework beyond what Dune actually needs for per-project/team customization.

## Capabilities

### New Capabilities

- `sbx-customization`: `sbx` kits as the customization layer over the Dune sbx template (replacing `Dockerfile.dune`), plus the template refresh/versioning strategy in a kit-aware world.
- `dune-cleanup`: retirement of the legacy Docker Compose backend/scaffolding and the `dune-base` image build, and a cleanup story for stale local Docker artifacts.

### Removed Capabilities

- The legacy Docker Compose backend scaffolding and `dune-base` Compose image build are retired. There is no promoted main spec to emit a `REMOVED` delta against, so the retirement is captured as `ADDED` requirements in the `dune-cleanup` spec.

## Impact

- **Customization**: Per-project/team customization moves to kits; `Dockerfile.dune` stays gone. Repos can carry kit definitions.
- **Build/CI**: The root `Dockerfile` / `container/base/` tree and the `image.yml` base build job are removed once the template is the sole runtime artifact; the version lockstep moves fully to the template (per `sbx-2` D6 and AGENTS.md, which today tie `VERSION` and `container/base/IMAGE_VERSION`).
- **Codebase**: Remaining Compose rendering/helpers/test fixtures are deleted.
- **Users**: A documented one-time cleanup of stale Docker volumes/images/compose files (manual or helper-assisted). No new required config.
- **Sequencing**: Last in the series; depends on proven parity of `sbx-2`..`sbx-5`. The two halves (kits; teardown/cleanup) can be split into separate landings.

## Depends On

All earlier sbx changes (`sbx-2` through `sbx-5`) and confirmed parity of the sbx backend.
