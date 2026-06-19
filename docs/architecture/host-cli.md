# Host CLI — Lifecycle, Logs, Diagnostics, and `dune doctor`

The host-side `dune` control plane lives in Go under `cmd/dune`, with the sbx
execution layer in `internal/dune/runtime/sbx`. This doc covers the finalised
user-facing command surface on the sbx backend (`sbx-5`): the lifecycle verbs
(`up`/`down`/`destroy`/`rebuild`), the `dune logs` composition, `dune ports`,
the structured-diagnostics taxonomy, and the read-only `dune doctor`.

It does **not** cover what the template contains or how it is published — those
live in [Dune sbx Template — Contents, Persistence, and
Workspace](./sbx-template.md) and [Distribution, Versioning, and Registry
Access](./sbx-template-distribution.md). The egress baseline and `sbx policy log`
data source that `dune logs`/`dune doctor` consume are established in `sbx-4`
(see [sbx Network and Secrets Posture](./sbx-network-and-secrets.md)); this doc
covers the Dune-side surfaces that read them.

## CLI flow

The CLI follows a `resolve → build Spec → execute` flow:

- parse `dune`, `dune up`, `dune down`, `dune destroy`, `dune rebuild`,
  `dune logs`, `dune ports`, `dune doctor`, and `dune profile ...`
- resolve the workspace root from git or cwd fallback
- read and write `~/.config/dune/profiles.json`
- build a backend-agnostic `Spec` (instance name, workspace host path, profile,
  template ref, working dir, shell, timezone) and hand it to the sbx backend
- validate the host `sbx` install (on `PATH`, `sbx diagnose --output json` all
  checks passing, installed version at the minimum) before any sandbox operation
- map the instance to a sandbox named `dune-<slug>-<profile>` and drive
  `sbx create`/`run`/`exec`/`stop`/`rm`/`ls`/`ports` through the backend; the
  concrete sbx command shapes stay private to the package and are pinned by tests
- on `up`/`rebuild`, after the sandbox is created, verify the instance's active
  egress posture via `sbx policy ls` (non-`Open` baseline, warn-closed on
  unconfirmable, hard-fail on observed `Open`) — see
  [sbx Network and Secrets Posture](./sbx-network-and-secrets.md)

Docker is no longer a host requirement; `sbx` (installed, authenticated, daemon
healthy, minimum version) is the prerequisite. The Dune sbx template ref comes
from `version.SbxTemplateRef()` (`container/sbx/IMAGE_VERSION`). Profile-scoped
persisted state lives under `~/.local/share/dune/persist/<profile>`, replacing
the old `dune-persist-<profile>` Docker volume with the same semantics.

Runtime flags `-d` / `--directory` (workspace directory) and `-p` / `--profile`
(profile name) apply to every runtime verb; `--verbose` shows diagnostic command
and stderr details on failure (see [Diagnostics](#structured-diagnostics-d4)).

## Lifecycle commands (D1)

The `dune` verbs map onto the `sbx-3` backend operations. They are user-facing
verbs over the backend; the verbs do not call `sbx` primitives directly except
through the `sbx-3` runner seam, so their argument construction is
fakeRunner-assertable.

| Verb | Behaviour | sbx shape (verified) |
| --- | --- | --- |
| `dune` / `dune up` | `Validate` → ensure template available → `Ensure` (create if missing) → `Start` (if stopped) → `Shell` at the mounted repo path. An already-running sandbox is attached without recreation. | `sbx create`/`run`/`exec -it` |
| `dune down` | `Stop` (state retained). The sandbox is **retained**, not removed. | `sbx stop <name>` |
| `dune destroy` | `Destroy` — removes the sandbox. Profile-scoped persisted state survives. Confirmation required, or `--force`/`-f`. | `sbx rm --force <name>` |
| `dune rebuild` | recreate from the (current) template, preserving profile-scoped persisted state. No `Dockerfile.dune` build. | `sbx rm --force` → `create` → `run` |
| `dune logs` | Dune-owned setup/runtime logs + `sbx policy log` (see [`dune logs` composition](#dune-logs-composition-d3)) | `sbx exec`, `sbx policy log` |
| `dune ports` | list / publish / unpublish host→sandbox ports (see [`dune ports`](#dune-ports-d1)) | `sbx ports <name>` |

### `dune destroy` is added; profile state survives (D2)

`sbx-3` deferred whether a removal command exists. Because the durable persist
location (`sbx-3` D3) decouples profile state from the sandbox, removing a
sandbox is safe for profile data, so `dune destroy` is the first-class removal
verb. `down` remains stop-only.

- `dune destroy` runs `sbx rm --force <instance>`. After it, the instance is
  absent from `sbx ls --json`, and a subsequent `dune up` recreates it with
  profile-scoped persisted state intact (smoke-verified).
- **Profile-scoped persisted state survives** destroy. Credentials, config, and
  tooling state under `~/.local/share/dune/persist/<profile>` are untouched; the
  persist directory is decoupled from any single sandbox and shared across a
  profile's sandboxes.
- `sbx rm` requires `--force` when stdin is not a TTY, so `dune destroy` always
  passes `--force` to `sbx rm`. The Dune-side confirmation prompt is separate:
  by default `dune destroy` asks you to type the instance name to confirm, and
  `-f` / `--force` skips that prompt (required in non-interactive scripts).

### `dune ports` (D1)

`dune ports` wraps the verified `sbx ports` surface through the runner seam:

- **list** (default): `sbx ports <sandbox>` — the raw human-readable list is
  surfaced verbatim. (`sbx ports <sandbox> --json` exists, but JSON flags are
  **not uniform** across sbx verbs per `sbx-3` D6; Dune surfaces the
  human-readable list and does not assume a uniform `--json`.)
- **publish**: `--publish <spec>` (repeatable; one flag per spec), where each
  spec follows `[[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]`. An omitted
  `HOST_PORT` yields an ephemeral host port; an omitted `HOST_IP` binds to
  loopback.
- **unpublish**: `--unpublish <spec>` (repeatable), where each spec follows
  `[HOST_IP:]HOST_PORT:SANDBOX_PORT[/PROTOCOL]`.

> **Loopback-vs-all-interfaces caveat (spike 2).** A published host port
> forwards to the **sandbox interface** only. A nested service bound solely to
> sandbox loopback (`127.0.0.1`) may not be reachable via the published host
> port — bind dev servers to all sandbox interfaces (e.g. `--host 0.0.0.0`) when
> you want host exposure. On a publish attempt, `dune ports` prints this guidance
> to stderr before invoking `sbx ports --publish`.

Spec validation guards the constructed invocation: a port spec must be a single
token over a conservative character set (digits, letters, `.`, `:`, `/`) and
must not look like a flag, so a malformed spec cannot reshape the `sbx` command.

## `dune logs` composition (D3)

`dune logs` composes three Dune-owned / sbx-owned sources, in order, each under
a labelled section:

1. **`== dune host lifecycle log ==`** — the host-side lifecycle log the runtime
   verbs already write. It records sandbox create/start/attach and setup-hook
   diagnostics emitted via the `sbx-3` runner seam, one timestamped line per
   event, at `$XDG_STATE_HOME/dune/logs/<instance>.log`
   (`~/.local/state/dune/logs/<instance>.log` if unset).
2. **`== dune sandbox logs (/var/log/dune) ==`** — the in-sandbox Dune
   setup/runtime output that `sbx-2` D5a writes to `/var/log/dune/*.log` (e.g.
   the re-homed `setup-persist` boot hook's `setup-persist.log`), read via
   `sbx exec <instance> bash -lc <script>`. With no argument (or `all`), every
   `/var/log/dune/*.log` file is concatenated; `dune logs <service>` narrows the
   read to `/var/log/dune/<service>.log`.
3. **`== sbx policy log ==`** — `sbx policy log <instance> --limit 50` (the
   default limit; Dune consumes and re-renders the blocked/allowed host records).
   This is the egress observability source from `sbx-4` D4.

The presence of the two Dune-owned sources (host lifecycle + `/var/log/dune/`)
in the composed output is a verification point, asserted in tests rather than
left as a standing question.

### `dune logs pipelock` is gone

`dune logs pipelock` **is gone**. Invoking it exits non-zero with the message:

> `dune logs pipelock is not available on the sbx backend; use dune logs for
> Dune runtime logs and sbx policy records`

Pipelock, its generated `pipelock.yaml`, and the proxy-env model are fully
removed for the sbx backend (`sbx-4` D6). Egress observability now comes from
`sbx policy log`, surfaced as the third section above. See
[sbx Network and Secrets Posture — Pipelock is
gone](./sbx-network-and-secrets.md#pipelock-is-gone).

### App-dependency service logs come from `docker compose logs` inside the sandbox

App-dependency service logs are intentionally **not** aggregated by `dune logs`.
There are no in-container app services to aggregate (`sbx-2` removed
Postgres/Redis/Mailpit), and there is no single `sbx logs` command to wrap (spike
2). Bring your own `docker compose` stack inside the sandbox — the template
ships the clients (`psql`, `redis-cli`, etc.) — and read project-owned service
logs via `docker compose logs <service>` **inside the sandbox**. `dune logs`
is Dune-owned runtime output plus `sbx policy log`, not an app-service aggregator.

## Structured diagnostics (D4)

Dune maps sbx backend failures to a small `DiagnosticError` type with stable
codes, preserved stderr, and recovery hints:

```go
type DiagnosticError struct {
    Code     string
    Summary  string
    Detail   string
    Cause    error
    Command  []string
    Stderr   string
    Recovery []string
}
func IsDiagnostic(err error) bool
func AsDiagnostic(err error) (*DiagnosticError, bool)
func WrapCommandError(code, summary string, result CommandResult, err error) error
func NewDiagnosticError(code, summary, detail string, cause error) error
```

The backend maps a failure to a code at the boundary where context is known
(e.g. a create failure whose stderr mentions `template`/`pull`/`image` becomes
`template.unavailable`). The initial, deliberately small code set:

| Code | Failure mode |
| --- | --- |
| `sbx.not_installed` | `sbx` not on PATH |
| `sbx.diagnose_failed` | daemon unhealthy / auth / readiness from `sbx diagnose` |
| `sbx.version_below_min` | installed sbx older than the required `v0.32.0` |
| `sbx.create_failed` | `sbx create` failed (non-template cause) |
| `sbx.start_failed` | `sbx run` (start) failed |
| `sbx.stop_failed` | `sbx stop` failed |
| `sbx.exec_failed` | `sbx exec` failed (shell/attach, logs read, setup hook) |
| `sbx.rm_failed` | `sbx rm` failed |
| `template.unavailable` | template ref cannot be pulled/loaded |
| `policy.apply_failed` | applying an egress rule failed |
| `workspace.invalid` | workspace root / path does not resolve |
| `profile.config_corrupt` | `profiles.json` could not be parsed |

**Preserved stderr.** The underlying command's stderr is preserved on the error,
not replaced by a friendly summary, so the real `sbx` failure text reaches the
user. Common host/setup failures (`sbx.not_installed`, `sbx.diagnose_failed`,
`sbx.version_below_min`, `template.unavailable`, `policy.apply_failed`,
`workspace.invalid`, `profile.config_corrupt`) carry a short `Recovery` hint.

**Concise by default, verbose on request.** Default CLI output is the code, the
summary, and any recovery hints. Pass `--verbose` to also see `Detail`, the exact
`Command`, the captured `Stderr`, and the wrapped `Cause`. Tests assert codes
and key fields, not rendered prose.

## `dune doctor` (D5)

`dune doctor` (+ `--verbose`, optional `--json`) inspects readiness **without**
starting or entering the environment. It reports readiness; it does not guarantee
the environment works (the smoke tests remain the end-to-end gate).

```go
type Check struct {
    ID, Group, Severity, Status, Summary, Detail string
    Recovery []string
}
// Status: pass | warn | fail | skip
```

It reuses the D4 diagnostic codes/recovery hints where a check corresponds to a
runtime failure mode, and aggregates an overall status: `fail` if any check
fails, else `warn` if any warns, else `pass`.

### Check groups

Each group names the read-only `sbx` command it relies on (pinned in
fakeRunner; reconfirm via `sbx <verb> --help`):

| Group | Checks | Read-only source |
| --- | --- | --- |
| **workspace/profile/config** | `workspace.resolve`, `workspace.slug`, `config.dir`, `data.dir`, `profile.store`, `profile.effective`, `persist.dir` | local filesystem only |
| **host/sbx** | `sbx.path`, `sbx.diagnose`, `sbx.version` | `sbx diagnose --output json`, `sbx version` |
| **template** | `template.ref` | `version.SbxTemplateRef()` (configured/non-empty; no forced pull) |
| **sandbox** | `sandbox.status` | `sbx ls --json` (reuses the `sbx-3` Status parse) |
| **egress** | `egress.posture` | `sbx policy ls` (from `sbx-4` D4) |

Notes:

- **host/sbx** uses the `--output json` form of `sbx diagnose` (`sbx-3` D6: JSON
  flags are not uniform — `sbx diagnose --output json` vs. `sbx ls --json` vs.
  `sbx ports ... --json`); the installed version must meet the minimum `v0.32.0`.
- **template** is deliberately lightweight: it checks the template ref is known
  and configured, and does **not** force a full image pull by default (a deep
  pull is opt-in, not the default behaviour).
- **sandbox** reuses the same `sbx ls --json` parse that `sbx-3` D7 `Status`
  uses, so doctor does not introduce a second `sbx ls` parser. Absent sandboxes
  report `pass` ("Sandbox is absent — dune doctor does not create sandboxes").
- **egress** follows `sbx-4` D1's read-only contract: `warn` (not `fail`) when
  the posture cannot be confirmed, `fail` only when an `Open` posture is
  positively observed, and `skip` when the sandbox is absent. Doctor never
  mutates policy.

### `--json`

`dune doctor --json` emits a structured `{status, checks}` object where each
check carries `id`/`group`/`severity`/`status`/`summary` and optional
`detail`/`recovery`. This is the machine-readable form for editor/CI
integration; the default human form prints `STATUS  GROUP  SUMMARY` per check
plus a `N failed, N warning, N passed, N skipped` tally.

### Read-only contract

All `sbx` calls `dune doctor` makes are read-only (`diagnose`/`version`/`ls`/
`policy ls`). Doctor **MUST NOT** invoke `sbx create`/`run`/`exec`/`rm`, and
tests assert no sandbox-mutating command is constructed. Smoke-verified against
absent and stopped sandboxes: running `dune doctor` does not change what
`sbx ls --json` reports.

## Compatibility

- profile mapping, workspace resolution, profile-name validation, timezone
  resolution, and self-update are preserved through the rewrite; the Docker
  Compose topology, the generated `compose.yaml`, the `Dockerfile.dune` build,
  and the mode/gear/devcontainer flows are gone.
