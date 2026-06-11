## ADDED Requirements

### Requirement: Service-identifier secrets are the preferred secrets mechanism

Dune SHALL prefer `sbx` service-identifier secrets (`sbx secret set`) for credentials tied to a built-in agent or kit-declared service identifier, and for registry authentication (`sbx secret set --registry`). These have an observed clean set/list/remove lifecycle.

#### Scenario: A service-identifier secret has a clean lifecycle
- **GIVEN** a credential associated with a service identifier
- **WHEN** it is set via `sbx secret set`, listed, and then removed via `sbx secret rm`
- **THEN** it appears while set and no longer appears after removal

---

### Requirement: Custom secrets are experimental and not Dune-managed in v1

Dune SHALL treat `sbx` custom secrets (`sbx secret set-custom`) as experimental and out of v1 lifecycle ownership: no core Dune boot path SHALL depend on them, and any optional/experimental use SHALL warn that cleanup may require manual intervention because removal is not reliable, they are not auto-injected into the sandbox environment, they can accumulate duplicate entries, and they can survive sandbox removal.

#### Scenario: Core boot does not depend on custom secrets
- **GIVEN** a Dune sbx workspace with no custom secrets configured
- **WHEN** `dune up` runs and the sandbox is entered
- **THEN** the workspace starts successfully without requiring any custom secret

#### Scenario: Experimental custom-secret use is warned
- **GIVEN** a user opting into experimental custom-secret usage
- **WHEN** Dune surfaces the option
- **THEN** it warns that custom-secret cleanup may require manual `sbx`/Docker intervention

---

### Requirement: No secrets are baked into the Dune template

The Dune sbx template SHALL NOT contain baked-in secrets, and Dune SHALL NOT produce the template via an `sbx template save` snapshot of a configured sandbox. Agent-provider credentials SHALL live in the profile-scoped persisted location (not in the template image) until a suitable service identifier is available for `sbx secret` injection.

#### Scenario: Template carries no secrets
- **GIVEN** the published Dune sbx template
- **WHEN** it is inspected
- **THEN** it contains no embedded credentials
- **AND** it was built from source rather than captured from a configured sandbox
