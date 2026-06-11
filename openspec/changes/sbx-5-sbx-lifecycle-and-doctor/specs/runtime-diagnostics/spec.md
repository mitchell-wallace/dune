## ADDED Requirements

### Requirement: sbx runtime failures return structured, coded diagnostics

Runtime failures from the sbx backend SHALL be returned as structured diagnostic errors carrying a stable code, a concise summary, preserved command stderr, and (for common host/setup failures) actionable recovery hints. The code set SHALL be small and sbx-scoped, and the original cause SHALL remain unwrap-compatible.

#### Scenario: A failing sbx command yields a coded diagnostic with stderr
- **GIVEN** an sbx command that fails (for example sandbox creation)
- **WHEN** the backend returns the error
- **THEN** the error exposes a stable code (such as `sbx.create_failed`)
- **AND** the failing command's stderr is preserved on the error
- **AND** a recovery hint is present for common host/setup failures

#### Scenario: Readiness failures are distinguishable by code
- **GIVEN** a host where `sbx` is missing, `sbx diagnose` fails, or the version is below the minimum
- **WHEN** a runtime command is attempted
- **THEN** the diagnostic code distinguishes the case (`sbx.not_installed`, `sbx.diagnose_failed`, or `sbx.version_below_min`)

---

### Requirement: Diagnostics are concise by default and detailed on demand

Default CLI output for a diagnostic SHALL be concise (summary plus recovery hints), with full command/stderr detail shown only under a verbose mode. Tests SHALL assert diagnostic codes and key fields rather than exact rendered prose.

#### Scenario: Concise default, verbose detail
- **GIVEN** a runtime failure that produced a diagnostic error
- **WHEN** it is displayed without verbose mode
- **THEN** the output shows a concise summary and recovery hints
- **AND** full command/stderr detail is shown when verbose mode is enabled
