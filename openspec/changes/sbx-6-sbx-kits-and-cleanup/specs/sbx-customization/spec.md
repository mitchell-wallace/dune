## ADDED Requirements

### Requirement: Kits are the customization layer over the Dune sbx template

Dune SHALL use `sbx` kits as the per-project/team customization layer over the Dune sbx template, replacing `Dockerfile.dune`. Dune SHALL document the recommended kit type(s) and a canonical location for kit definitions, and SHALL NOT reintroduce a per-repo Dockerfile build path.

#### Scenario: A project customizes via a kit, not a Dockerfile
- **GIVEN** a project that needs additional files, env vars, commands, or network rules
- **WHEN** it customizes its Dune environment
- **THEN** the customization is expressed as an `sbx` kit layered on the Dune sbx template
- **AND** no `Dockerfile.dune` build path is used

#### Scenario: A kit's additions are verified against the installed sbx before kits are documented as supported
- **GIVEN** spike 4 recorded the `sbx` kit subcommand surface and create-with-kit semantics against the installed binary, but not against the Dune template specifically
- **WHEN** Dune documents kits as the supported customization path
- **THEN** the verified `sbx` kit subcommand surface, YAML schema, and create-with-kit flow are recorded against the installed binary
- **AND** a minimal mixin kit is smoke-tested so a sandbox built from the **Dune** template with that kit carries the kit's declared addition (e.g. an env var or file is present in the sandbox)
- **AND** if the installed `sbx` cannot apply kits as documented, Dune falls back to docs-only recipes and does not claim an unverified kit build path

#### Scenario: Recommended docs-domain rules are available as a kit/recipe
- **GIVEN** the `Balanced` baseline blocks arbitrary docs sites
- **WHEN** a project wants common documentation domains opened
- **THEN** a Dune-recommended kit or documented recipe provides those rules as exact + specific-wildcard entries (not broad catch-alls)
- **AND** the recipe uses the sandbox-scoped policy form and does not mutate global policy (`sbx policy set-default`)
- **AND** with the recipe applied, a fetch of a representative `Balanced`-blocked docs domain succeeds while an out-of-recipe domain still blocks, confirmed via `sbx policy log <instance>`

---

### Requirement: The template has a kit-aware refresh and versioning strategy

The Dune sbx template SHALL have a documented refresh/republish flow, keeping the CLI `VERSION` in lockstep with the template image version. Kit definitions SHALL target or pin a template version so a kit and the template it layers on remain compatible.

#### Scenario: Template version stays in lockstep with the CLI
- **GIVEN** a Dune CLI release
- **WHEN** the template is rebuilt and republished
- **THEN** the template image version matches the CLI version per the lockstep convention

#### Scenario: A kit targets a compatible template version
- **GIVEN** a kit layered on the Dune sbx template
- **WHEN** the kit is applied
- **THEN** it targets a template version it is compatible with
