```md
## Proposal: Add Dune Doctor

## Summary

Add `dune doctor`, a user-facing diagnostic command that checks whether the current host, workspace, profile, config, and backend are ready to run Dune.

This command should help users distinguish host setup problems from Dune config problems and runtime backend problems.

## Depends On

```text
introduce-environment-plan-boundary
extract-docker-compose-backend
improve-runtime-diagnostics
````

`improve-runtime-diagnostics` is not strictly required, but `dune doctor` is more valuable if it can reuse diagnostic codes and recovery hints.

## Problem

When Dune fails, the user often needs to know whether the issue is:

* Docker is not installed;
* Docker daemon is not running;
* Docker Compose is unavailable;
* the workspace could not be resolved;
* profile config is corrupt;
* Pipelock config is missing/invalid;
* generated project state is unwritable;
* the base image cannot be pulled;
* custom `Dockerfile.dune` build state is broken.

Today these checks are mostly embedded in normal lifecycle commands. There is no low-risk command that inspects readiness without starting or entering the environment.

## Objectives

This change should:

* add `dune doctor`;
* inspect the current workspace/profile/backend readiness;
* avoid starting the environment;
* produce concise human-readable output;
* optionally support machine-readable output;
* reuse backend validation and diagnostic primitives;
* preserve existing lifecycle commands unchanged.

## Non-Goals

This change should not:

* start containers;
* attach to shell;
* rebuild images;
* mutate profile mappings;
* redesign config storage;
* add new backends;
* perform expensive full image builds;
* replace smoke tests.

## Command Shape

Add:

```sh
dune doctor
dune doctor --verbose
dune doctor --json
```

If `--json` is too much for the first implementation, it can be deferred, but the internal check model should not block adding it later.

## Check Model

Suggested internal type:

```go
type Check struct {
    ID       string
    Group    string
    Severity string
    Status   string
    Summary  string
    Detail   string
    Recovery []string
}
```

Suggested statuses:

```text
pass
warn
fail
skip
```

Suggested groups:

```text
host
workspace
profile
config
backend
egress
image
```

## Initial Checks

### Host

```text
docker CLI present
docker compose available
docker daemon reachable
```

### Workspace

```text
workspace root resolves
workspace slug can be computed
workspace root is readable
Dockerfile.dune detection works
```

### Profile

```text
profiles.json exists or can be absent safely
profiles.json parses if present
selected/effective profile resolves
profile name is valid
```

### Config and Data Paths

```text
~/.config/dune readable/writable or creatable
~/.local/share/dune readable/writable or creatable
project data dir readable/writable or creatable
generated compose path parent writable
```

### Egress / Pipelock

```text
pipelock config path resolves
existing pipelock config parses if present
pipelock config can be generated if absent
pipelock image reference is configured
```

### Image / Runtime

```text
base image ref is known
base image exists locally or appears pullable
custom Dockerfile.dune exists when expected
compose file can be rendered from EnvironmentPlan
docker compose config accepts rendered compose
```

Be careful with pullability checks. A full `docker pull` may be too expensive or surprising. Prefer lightweight checks unless `--verbose` or a future `--deep` flag is introduced.

## Backend Integration

The Docker Compose backend should expose doctor checks without forcing environment startup.

Candidate interface extension:

```go
type DoctorBackend interface {
    Doctor(ctx context.Context, plan plan.EnvironmentPlan, opts DoctorOptions) []diagnostics.Check
}
```

Alternatively, keep doctor orchestration outside the backend and call backend validation helpers directly. Prefer backend-owned checks where the logic is Docker-specific.

## Output

Default output should be compact:

```text
Dune doctor

PASS host      Docker CLI found
PASS host      Docker Compose available
FAIL backend   Docker daemon unavailable
PASS workspace Workspace resolved
PASS profile   Effective profile: work
WARN egress    Pipelock config missing; it will be generated on next run

1 failed, 1 warning, 4 passed
```

Verbose output can include command stderr and recovery hints.

JSON output should emit the structured check list if implemented:

```json
{
  "status": "fail",
  "checks": []
}
```

## Testing

Add tests for:

```text
doctor output with all checks passing
doctor output with Docker daemon unavailable
doctor output with corrupt profiles.json
doctor output with unwritable config/data dir where practical
doctor output with missing Pipelock config
doctor output with invalid Pipelock config where currently detectable
doctor --json shape if implemented
```

Use fake backend/runner tests for host/backend checks. Keep real Docker validation separate and skippable.

## Acceptance Criteria

This change is complete when:

* `dune doctor` exists;
* it does not start or enter the environment;
* it reports host/workspace/profile/config/backend readiness;
* failures include actionable recovery hints;
* checks are structured internally;
* Docker-specific checks live in or near the Docker Compose backend;
* normal Dune lifecycle commands are unchanged;
* tests cover pass/fail/warn cases.

## Risk Areas

### Doctor becomes too expensive

Avoid full image builds or surprise pulls. Add deep checks later if needed.

### Doctor mutates too much state

Prefer read-only checks. Creating missing config/data directories may be acceptable only if consistent with existing Dune behaviour.

### Duplicated validation logic

Backend checks should reuse existing validation/diagnostic helpers where possible.

### False assurance

Doctor should report readiness, not guarantee the whole environment will work. Real smoke tests remain necessary.

### Scope creep

Do not turn this into a repair command. A future `dune doctor --fix` can be considered separately.

````