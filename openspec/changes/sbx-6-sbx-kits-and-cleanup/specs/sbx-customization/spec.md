## ADDED Requirements

### Requirement: Kits are the customization layer over the Dune sbx template

Dune SHALL use `sbx` kits as the per-project/team customization layer over the Dune sbx template, replacing `Dockerfile.dune`. Dune SHALL document the recommended kit type(s) and a canonical location for kit definitions, and SHALL NOT reintroduce a per-repo Dockerfile build path.

#### Scenario: A project customizes via a kit, not a Dockerfile
- **GIVEN** a project that needs additional files, env vars, commands, or network rules
- **WHEN** it customizes its Dune environment
- **THEN** the customization is expressed as an `sbx` kit layered on the Dune sbx template
- **AND** no `Dockerfile.dune` build path is used

#### Scenario: Recommended docs-domain rules are available as a kit/recipe
- **GIVEN** the `Balanced` baseline blocks arbitrary docs sites
- **WHEN** a project wants common documentation domains opened
- **THEN** a Dune-recommended kit or documented recipe provides those rules as exact + specific-wildcard entries (not broad catch-alls)

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
