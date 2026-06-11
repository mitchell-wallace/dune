## ADDED Requirements

### Requirement: The legacy Docker Compose backend and base image build are retired

Once the sbx backend is the sole backend with proven parity, Dune SHALL remove the remaining Docker Compose scaffolding (leftover compose rendering/helpers and golden/validation test fixtures), the legacy `dune-base` Compose image build (root `Dockerfile`, the `container/base/` tree, and the `image.yml` base build-and-push job), and SHALL move the version lockstep onto the template image.

#### Scenario: No legacy Compose backend remains
- **GIVEN** the sbx backend is the sole backend
- **WHEN** the codebase and CI are inspected
- **THEN** there is no Docker Compose backend scaffolding, no `dune-base` Compose image build, and no `image.yml` base build job
- **AND** the version lockstep references the template image version rather than `container/base/IMAGE_VERSION`

---

### Requirement: Stale local Docker artifacts have a cleanup story

Dune SHALL provide an opt-in `dune cleanup docker` helper for stale local Docker artifacts left by the pre-migration backend — `dune-persist-<profile>` volumes, `dune-local-<slug>` images, and generated compose files. The cleanup SHALL be explicit (list-then-confirm), SHALL only target Dune-scoped artifacts, and SHALL NOT remove unrelated Docker resources. Documentation MAY also include manual commands, but the helper is the supported cleanup path.

#### Scenario: Cleanup targets only Dune artifacts and confirms first
- **GIVEN** stale `dune-persist-<profile>` volumes, `dune-local-<slug>` images, and generated compose files on the host
- **WHEN** the user runs the Dune cleanup helper
- **THEN** it lists the Dune-scoped artifacts it would remove and requires confirmation
- **AND** it does not remove non-Dune Docker resources
