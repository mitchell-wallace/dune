# Shell Script Refactoring Candidates

Analysis of `container/` shell scripts for substantial refactoring opportunities.
Scripts in this directory are stable infrastructure (not being absorbed into the Go TUI).

---

## Priority 1: Extract `container/lib/utils.sh` — shared utilities

Three functions are copy-pasted across multiple files:

| Function | Files |
|---|---|
| `canonicalize_mode()` | `sand-privileged.sh`, `addons-cli.sh`, `sand-poststart.sh` |
| `resolve_ipv4s_with_retry()` | `sand-privileged.sh`, `add-playwright.sh`, `init-firewall.sh` |
| `run_as_target_user()` | Every addon install script (~6 copies) |

A `lib/utils.sh` sourced by these scripts eliminates the most pervasive duplication.
Low risk to introduce, high payoff. Also candidates for the shared lib:
- `mode_enabled()` — `sand-privileged.sh`, `addons-cli.sh`
- `run_as_root()` — `install-project-tools.sh`, `boost-cli.sh`
- `log()` — different implementations in nearly every file

---

## Priority 2: Break up `sand-privileged.sh` (639 lines)

Monolithic dispatcher for three unrelated concerns:

- **Locale/timezone setup** (~100 lines)
- **Service management** for PostgreSQL, Redis, Mailpit (~230 lines)
- **Addon execution** (~85 lines)

The three service blocks (`pg_local_cmd`, `redis_local_cmd`, `mp_local_cmd`) all implement
the same interface (start/stop/restart/status/logs/shell/url/help) and differ only in
service-specific details. They could be split into separate files or driven by a shared
service template function.

Proposed split:
- `sand-privileged-config.sh` — locale, timezone, mode/profile normalization
- `sand-privileged-services.sh` — pg/redis/mailpit service management (or per-service files)
- `sand-privileged-addons.sh` — addon execution and manifest ops

---

## Priority 3: Extract firewall domain allowlist from `init-firewall.sh` (553 lines)

Lines 411–495 are ~85 hardcoded domain entries inline in the firewall script.
Every new tool or service requiring network access requires editing a 553-line file.

Proposed: extract to `firewall-domains.conf` (TSV or one-per-line format), with
`init-firewall.sh` reading it at runtime. Cleanly separates *policy* from *mechanism*
and makes the domain list reviewable without reading firewall logic.

Also: `resolve_ipv4s_with_retry()` is duplicated here (see Priority 1).

---

## Priority 4: Consolidate addon install scripts

The following script groups are 90%+ identical, differing only in tool name and install command:

**Mise-based tools** (~41 lines each):
- `add-go.sh`, `add-rust.sh`, `add-python-uv.sh`

**NPM global installs** (~33–41 lines each):
- `add-pnpm.sh`, `add-turbo.sh`, `add-gemini.sh`, `add-opencode.sh`

Options:
- Shared installer function in `lib/utils.sh` with per-script thin wrappers
- Single parameterized script driven by addon manifest entries
- Generate from manifest at build time

---

## Priority 5: Deduplicate `setup-agent-persist.sh` internals (127 lines)

`seed_dir_if_empty()` (lines 35–58) and `seed_file_if_empty()` (lines 60–83) share
nearly identical logic. The hardcoded mapping calls at lines 115–126 mean adding a new
persisted path requires touching the script body.

Proposed: move mappings to a table at the top of the file (or external config), iterated
by a single generic loop using the unified seed function.

---

## Shared Function Registry (candidates for `lib/utils.sh`)

```sh
canonicalize_mode()           # 3 copies
mode_enabled()                # 2 copies
normalize_profile()           # 2 copies
resolve_ipv4s_with_retry()    # 3 copies
run_as_target_user()          # 6+ copies (all addon scripts)
run_as_root()                 # 2+ copies
log()                         # ~10 different implementations
```

---

## Script Complexity Reference

| Script | Lines | Complexity |
|---|---|---|
| `runtime/sand-privileged.sh` | 639 | Very High |
| `runtime/init-firewall.sh` | 553 | Very High |
| `addons/add-playwright.sh` | 208 | High |
| `addons/boost-cli.sh` | 152 | Medium |
| `runtime/addons-cli.sh` | 138 | Medium |
| `runtime/setup-agent-persist.sh` | 127 | Medium |
| `runtime/sand-poststart.sh` | 103 | Medium |
| `setup/install-project-tools.sh` | 86 | Low-Medium |
| `addons/add-postgres.sh` | 81 | Medium |
| `home/.agent-shell-setup.sh` | 71 | Low |
| `addons/add-mailpit.sh` | 66 | Medium |
| `addons/add-redis.sh` | 66 | Low |
| `addons/add-python-uv.sh` | 49 | Low |
| `addons/add-go.sh` / `add-rust.sh` | 41 each | Low |
| `addons/add-gemini.sh` / `add-opencode.sh` | 41 each | Low |
| `addons/add-pnpm.sh` / `add-turbo.sh` | 33 each | Low |
| `runtime/sand-entrypoint.sh` | 38 | Minimal |
| `setup/configure-agents.sh` | 13 | Minimal |

---

## Recommended Starting Point

**Start with Priority 1** (`lib/utils.sh`) — it unblocks everything else.
Once `resolve_ipv4s_with_retry`, `canonicalize_mode`, and `run_as_target_user` live in
one place, the other refactors become safer and more isolated.

**Priority 3** (firewall domains config) is entirely self-contained and the easiest
standalone win.
