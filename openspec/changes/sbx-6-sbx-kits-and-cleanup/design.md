## Context

By the end of `sbx-5`, Dune runs solely on the sbx backend: a Docker-enabled Dune sbx template (`sbx-2`), an `internal/dune/runtime/sbx` backend (`sbx-3`), an `sbx`-owned egress posture with Pipelock removed (`sbx-4`), and a finalised command surface with diagnostics and `dune doctor` (`sbx-5`). `Dockerfile.dune` was dropped in `sbx-3` and not migrated.

Two threads remain. **Customization**: the `sbx` docs position kits (experimental YAML layering commands, env, files, network rules, credentials, memory) as the per-project/team layer over a heavy template — the successor to `Dockerfile.dune`. **Teardown**: the legacy Compose code, the `dune-base` image build (root `Dockerfile`, `container/base/`, the `image.yml` base job), and any stale local Docker artifacts should be retired once the sbx path is the sole backend.

## Goals / Non-Goals

**Goals**
- Establish kits as Dune's customization layer over the template, replacing `Dockerfile.dune`.
- Define the template refresh/versioning strategy in a kit-aware world.
- Retire the legacy Docker Compose scaffolding and the `dune-base` image build.
- Provide a cleanup story for stale local Docker artifacts.

**Non-Goals**
- Anything owned by `sbx-2`..`sbx-5`.
- A dual-backend model or a large bespoke kit framework.

## Decisions

### D1: Kits are the customization layer (replacing Dockerfile.dune)
Per-project/team customization is expressed as `sbx` kits layered on the Dune sbx template, not a per-repo Dockerfile. Decide the kit types Dune ships/recommends:
- **Mixin kits** to extend the Dune template with project files, env vars, commands, and network rules (the common case, the closest replacement for `Dockerfile.dune`).
- **Agent kits** (which can define a full agent incl. `agent.image`) only if Dune needs a packaged agent; otherwise prefer mixins over the heavy template.
Decide where kit definitions live (recommended: per-repo, alongside existing project config such as `rally.toml`). Kits remain experimental in `sbx`, so Dune treats them as the recommended-but-evolving customization path and avoids hard dependencies on unstable kit behavior.

### D2: A documented docs-domain kit recipe
`sbx-4` leaves `Balanced` blocking arbitrary docs sites. Provide a Dune-recommended documented kit recipe carrying common docs-domain network rules (exact + specific-wildcard, per `sbx-4` D3), so docs-heavy agent workflows have an easy on-ramp without broad catch-alls. This is opt-in, not a default-open posture, and the core runtime must not depend on it.

### D3: Template refresh/versioning in a kit-aware world
Formalize how the Dune sbx template is updated and republished and how kits relate to template versions:
- Keep the `VERSION` + template `IMAGE_VERSION` lockstep from `sbx-2` D6; retire the base image's role in that lockstep (D4 removes the base build).
- Kits pin or target a template version so a kit and the template it layers on stay compatible; document the refresh flow (rebuild/republish template, then update kits as needed).

### D4: Retire the legacy Docker Compose scaffolding and base image build
Once the sbx backend is the sole backend (parity proven):
- Remove any leftover compose rendering/helpers/golden+validation test fixtures not already deleted in `sbx-3`.
- Remove the legacy `dune-base` Compose image build: the root `Dockerfile`, the `container/base/` tree (after the template build has taken over the assets it needs, per `sbx-2` D2), and the `image.yml` base build-and-push job.
- Update AGENTS.md/version-bump guidance so the lockstep references the template's `IMAGE_VERSION`, not `container/base/IMAGE_VERSION`.
Sequencing: this happens only after `sbx-5` and after the template build (`sbx-2`) no longer needs the `container/base/` assets.

### D5: Stale local Docker artifact cleanup
Provide a cleanup story for artifacts left by the pre-migration backend on users' machines:
- `dune-persist-<profile>` Docker volumes, `dune-local-<slug>:latest` images, and generated compose files under the Dune config/data dirs.
- Provide a small, explicit, opt-in `dune cleanup docker` helper that lists what it would remove and requires confirmation; docs may also include manual commands, but the helper is the supported path. Never remove non-Dune Docker artifacts.

## Risks / Trade-offs

- **Kit instability.** Kits are experimental in `sbx`; building hard dependencies on them is risky. Mitigation: treat kits as recommended-but-evolving; keep the template self-sufficient for core Dune.
- **Premature teardown.** Removing the Compose backend before parity is proven would strand users. Mitigation: gate D4 on confirmed parity; allow splitting the kit and teardown halves.
- **Destructive cleanup.** A cleanup helper could remove wanted data. Mitigation: opt-in, list-then-confirm, Dune-scoped artifacts only.
- **Version drift between kit and template.** Mitigation: kits target a template version; document the refresh flow (D3).

## Migration Plan

1. Land after `sbx-2`..`sbx-5` and confirmed sbx parity.
2. Establish kit types, location, and the docs-domain kit recipe (D1, D2); document the template refresh/versioning flow (D3).
3. Remove leftover Compose scaffolding, the `dune-base` image build, and the `image.yml` base job; update version-bump guidance (D4).
4. Ship the stale-artifact cleanup helper and guidance (D5).

## Open Questions

- Which kit types Dune ships vs. recommends, and the canonical per-repo location for kit definitions.
- The exact initial docs domains included in the recommended recipe.
- Exact confirmation UX and dry-run output for `dune cleanup docker`.
- Exact timing of `container/base/` removal relative to the template build's reuse of those assets (`sbx-2` D2).
