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

**Verification point (largely resolved by spike 4 against `sbx` v0.32.0).** Spike 4 exercised the kit surface empirically: the subcommands are `sbx kit add|inspect|pack|pull|push|validate` (all marked EXPERIMENTAL); the apply mechanism is `sbx create --kit <ref>` (repeatable; directory, ZIP, OCI ref, or `git+https://…#dir=…`), which only takes effect at create time, plus `sbx kit add <sandbox> <ref>` for running sandboxes (re-runs install commands, re-copies files). A minimal mixin kit (`schemaVersion: "1"`, `kind: mixin`, validated by `sbx kit validate`) was created against the generic template and confirmed: `commands.install` ran once at creation; `commands.startup` ran at every boot (create + restart after `stop`); `environment.variables` were visible in non-login `sbx exec … bash -c` and survived restart. Remaining for this change: repeat the smoke check against the *Dune* template specifically, and pin the spellings in fakeRunner where Dune constructs them. The experimental marking stands — Dune documents kits as the customization path with that caveat, and keeps docs-only recipes as the fallback if the surface drifts.

### D2: A documented docs-domain kit recipe
`sbx-4` leaves `Balanced` blocking arbitrary docs sites. Provide a Dune-recommended documented kit recipe carrying common docs-domain network rules (exact + specific-wildcard, per `sbx-4` D3), so docs-heavy agent workflows have an easy on-ramp without broad catch-alls. This is opt-in, not a default-open posture, and the core runtime must not depend on it.

The recipe's rules MUST use the `sbx-4`-verified sandbox-scoped policy form, expressed either as kit-authored network rules (if D1 confirms kits carry them) or as the equivalent `sbx policy allow network --sandbox <instance> <domain>:443` / `'*.<domain>:443'` commands, with both the exact root and the specific wildcard listed when both are needed (neither covers the other, per `sbx-4` D3). **Verification point:** smoke-test the recipe against the `sbx` binary — with the recipe applied to a sandbox, a fetch of a representative docs domain that `Balanced` blocks succeeds, while an out-of-recipe domain still blocks (confirmed via `sbx policy log <instance>`). The recipe MUST NOT mutate global policy (no `sbx policy set-default`) and MUST NOT use broad catch-alls.

### D3: Template refresh/versioning in a kit-aware world
Formalize how the Dune sbx template is updated and republished and how kits relate to template versions:
- Keep the `VERSION` + template `IMAGE_VERSION` lockstep from `sbx-2` D6; retire the base image's role in that lockstep (D4 removes the base build). Concretely, the live source of the published ref is the `version.SbxTemplateRef()` accessor added in `sbx-3` D10, which reads the template's own `container/sbx/IMAGE_VERSION` (`sbx-2` D6); "lockstep with `VERSION`" is the release/version-bump convention, not the literal file the accessor reads (per `sbx-3`). D4's base-image removal MUST leave `version.SbxTemplateRef()` and its unit test intact and only remove `version.BaseImageRef()`/`container/base/IMAGE_VERSION` once nothing reads them.
- Kits pin or target a template version so a kit and the template it layers on stay compatible; document the refresh flow (rebuild/republish template, then update kits as needed). The kit-side version-targeting field is part of the kit YAML schema confirmed in D1; if the verified schema offers no such field, kit/template compatibility is documented as a convention against the published template tag rather than asserted as a kit feature.

### D4: Retire the legacy Docker Compose scaffolding and base image build
Once the sbx backend is the sole backend (parity proven):
- Remove any leftover compose rendering/helpers/golden+validation test fixtures not already deleted in `sbx-3`. `sbx-3` D8 already removes `project{}`, `compose.yaml.tmpl`, `renderComposeFile`/`ensureComposeFile`/`validateComposeFile`, `ensureVolume`, `prepareAgentImage`, `composeArgs`/`composeUp`/`localImageExists`, and `validateDockerPrerequisites`; this change's removal scope is whatever residue survives that (e.g. compose golden fixtures, `docker compose` helpers, the `BaseImageRef()` path, the `pipelock` package if `sbx-4` left any) — confirmed by grepping for `compose`, `docker compose`, `dune-base`, and `BaseImageRef` returning no live references.
- Remove the legacy `dune-base` Compose image build: the root `Dockerfile`, the `container/base/` tree (after the template build has taken over the assets it needs, per `sbx-2` D2), and the `image.yml` base build-and-push job.
- Update AGENTS.md/version-bump guidance so the lockstep references the template's `IMAGE_VERSION` (`container/sbx/IMAGE_VERSION`), not `container/base/IMAGE_VERSION`, and so the version-bump checklist no longer names the removed file.
- **Verification point:** after removal, `go build ./cmd/dune` and `go test ./...` pass, and a smoke run (`dune up` → attach → `dune doctor` → `dune down`) against a sandbox built from the Dune sbx template still works with no `container/base/` tree present.
Sequencing: this happens only after `sbx-5` and after the template build (`sbx-2`) no longer needs the `container/base/` assets.

### D5: Stale local Docker artifact cleanup
Provide a cleanup story for artifacts left by the pre-migration backend on users' machines. Target only the artifacts the legacy backend actually created, by their exact naming (so the helper can match precisely and never touch non-Dune resources):
- **Persist volumes** named `dune-persist-<profile>` (`internal/dune/app.go`, `PersistVolume: "dune-persist-" + profile`).
- **Local images** named `dune-local-<slug>:latest`, where `<slug>` is `workspace.Slug(root)` = `<sanitized-basename>-<first-sha1-byte-hex>` (`internal/dune/app.go` `AgentImage`, `internal/dune/workspace/workspace.go`). These only exist for repos that had a `Dockerfile.dune`.
- **Generated compose files** at `<XDG_DATA_HOME or ~/.local/share>/dune/projects/<slug>/compose.yaml` (and the now-unused `<XDG_CONFIG_HOME or ~/.config>/dune/pipelock.yaml`).

Provide a small, explicit, opt-in `dune cleanup docker` helper that lists what it would remove and requires confirmation; docs may also include manual commands, but the helper is the supported path. Never remove non-Dune Docker artifacts.

Implementation/verification shape:
- Register `cleanup` as a new top-level command via the existing custom parser (add a `Command` constant + parse case in `internal/dune/cli/options.go` and a `Run` case in `internal/dune/app.go`), mirroring how `profile`/`down` are wired (there is no cobra/urfave framework).
- Route the `docker volume`/`docker image`/`docker rm` calls through the `sbx-3` D5 runner seam (or an equivalent injected runner) so the exact `docker` argument construction is asserted by a fakeRunner test, and discovery is scoped by Dune's name prefixes — e.g. `docker volume ls -q -f name=^dune-persist-`, `docker image ls -q dune-local-*` — rather than an unfiltered prune. The destructive step is gated behind a typed/`--yes` confirmation; default is dry-run/list-only.
- Tests assert: (1) only `dune-persist-*` / `dune-local-*` / Dune-owned compose paths are selected, (2) a sample of unrelated volumes/images is never selected, (3) nothing is removed without confirmation (list-then-confirm), (4) absence of artifacts is a clean no-op. Real removal is not exercised in unit tests; the fakeRunner records the would-be `docker` invocations.

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
- The verified `sbx` kit command surface and YAML schema (incl. whether kits carry network rules and a template-version field), to be confirmed against the installed `sbx` per D1 before kits are documented as supported.
- The exact initial docs domains included in the recommended recipe.
- Exact confirmation UX and dry-run output for `dune cleanup docker` (and whether the verb should align with `sbx-5`'s `dune destroy` confirmation/`--force` convention).
- Exact timing of `container/base/` removal relative to the template build's reuse of those assets (`sbx-2` D2).
