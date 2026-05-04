````md
## Proposal: Improve Runtime Diagnostics

## Summary

Introduce structured runtime diagnostics for Dune backend failures.

This change should make runtime failures easier to understand, test, and eventually surface through `dune doctor`, without changing Dune’s core lifecycle behaviour or adding new backends.

The main goal is to replace ad hoc error wrapping around Docker/Compose execution with stable diagnostic categories, useful stderr preservation, and targeted recovery hints.

## Depends On

```text
introduce-environment-plan-boundary
extract-docker-compose-backend
````

## Problem

Dune currently depends on several side-effectful runtime operations:

* Docker prerequisite checks;
* Pipelock config generation/reconciliation;
* Compose rendering and validation;
* Docker volume creation;
* base image pull;
* `Dockerfile.dune` build;
* Compose startup;
* shell attach;
* logs streaming.

When these fail, errors are mostly wrapped at the call site. That makes failures hard to classify in tests and hard for users to act on.

## Objectives

This change should:

* add stable diagnostic error codes;
* preserve relevant command stderr/stdout;
* attach recovery hints for common failures;
* make failure-mode tests assert codes instead of brittle strings;
* keep user-facing output concise;
* prepare diagnostic primitives that `dune doctor` can reuse later.

## Non-Goals

This change should not:

* add `dune doctor`;
* add JSON output unless already trivial;
* redesign backend execution;
* implement new runtime backends;
* change command success behaviour;
* change Compose generation;
* change Pipelock config semantics.

## Proposed Design

Add a small diagnostic error type:

```go
type DiagnosticError struct {
    Code     string
    Summary  string
    Detail   string
    Cause    error
    Command  []string
    Stdout   string
    Stderr   string
    Recovery []string
}
```

Add helpers:

```go
func IsDiagnostic(err error) bool
func AsDiagnostic(err error) (*DiagnosticError, bool)
func WrapCommandError(code, summary string, result CommandResult, err error) error
```

The backend should return diagnostic errors for known runtime failures.

Initial codes:

```text
docker.compose_missing
docker.daemon_unavailable
docker.compose_validation_failed
docker.volume_create_failed
image.pull_failed
image.build_failed
pipelock.config_generate_failed
pipelock.config_invalid
runtime.start_failed
runtime.logs_failed
runtime.shell_failed
profile.config_corrupt
workspace.invalid
```

Codes should be stable enough for tests, but not treated as a public API yet unless explicitly documented.

## Error Display

Default CLI output should stay concise:

```text
Dune runtime error: Docker daemon unavailable

Docker did not respond to `docker info`.

Try:
- Start Docker Desktop or the Docker daemon.
- Run `docker info` on the host to confirm access.
```

Verbose/debug modes can show command details and stderr more fully if such a mode already exists or is easy to add. Avoid noisy stack-like output by default.

## Backend Integration

The Docker Compose backend should map failures at the boundary where context is known.

Examples:

```text
docker compose version fails -> docker.compose_missing
docker info fails -> docker.daemon_unavailable
docker compose config fails -> docker.compose_validation_failed
docker volume create fails -> docker.volume_create_failed
docker pull fails -> image.pull_failed
docker compose build fails -> image.build_failed
docker compose up fails -> runtime.start_failed
docker exec fails -> runtime.shell_failed
```

On `compose up` failure, preserve the existing behaviour of collecting recent logs where applicable. Attach those logs to the diagnostic detail or stderr field.

## Testing

Add failure-mode tests using the fake command runner from the backend extraction.

Required cases:

```text
docker compose version failure
docker info failure
compose validation failure with stderr
volume create failure
base image pull failure
Dockerfile.dune build failure
compose up failure with recent logs
logs command failure
shell attach failure
corrupt profiles.json
invalid/stale Pipelock config where currently detectable
```

Tests should assert:

* diagnostic code;
* summary is non-empty;
* relevant stderr is preserved;
* recovery hints exist for common host/setup failures;
* original cause is still unwrap-compatible where useful.

## Acceptance Criteria

This change is complete when:

* runtime failures return structured diagnostic errors;
* common Docker/Compose failures have stable codes;
* stderr from failed commands is preserved;
* user-facing error output is clearer than raw command failures;
* failure-mode tests assert diagnostic codes;
* existing success-path behaviour is unchanged;
* existing smoke tests still pass.

## Risk Areas

### Overbuilding the taxonomy

Keep the first diagnostic taxonomy small. Add codes only for failures Dune can currently distinguish.

### Losing low-level details

Do not replace stderr with friendly summaries. Preserve both.

### Brittle user-facing tests

Tests should assert codes and key fields, not exact rendered prose.

### Scope creep into doctor

This change should create reusable diagnostic primitives, not a diagnostic command.

````