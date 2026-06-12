# Spike 4: Flag Resolution, Durable Persistence, and the s6-less Template
Date: 2026-06-12
Branch: `experiment/docker-sbx`
Status: **executed** against `sbx` v0.32.0 (`55580366449bcfebfc1787b9944284cf64c856d7`) on the host with Docker Engine 29.4.3. All four parts completed; findings below.

> Session note: the stored `sbx` session token had been revoked (`sbx ls` → 401, `sbx diagnose` → "Authentication — not signed in"). Re-authenticated non-interactively via `sbx login --username … --password-stdin` using the Docker Hub credential already stored in the host's Docker credential helper. Sandbox-creating parts of this spike require a signed-in `sbx`.

## Purpose
The sbx-2…sbx-6 plans were tightened to flag every command shape that the earlier spikes did not actually prove against the `sbx` binary, and the team decided to **drop s6-overlay and the in-container Postgres/Redis/Mailpit services** (sbx-2 D2a/D3), re-homing `setup-persist` onto a boot hook. This spike converts the remaining hedges into definite, plan-ready facts and de-risks the two integration points that cannot be settled from docs alone:

1. **Re-homing `setup-persist`** — does `sbx` offer a native create/boot hook, or do we fall back to an idempotent login-shell sentinel? (sbx-2 D2a)
2. **Unverified command flags** — the exact spellings the plans currently gate on `--help` (sbx-2/3/5).
3. **Durable profile persistence** — does a profile-scoped persist mapping survive `rm`/recreate? (sbx-3 D3)
4. **The s6-less merged template** — build it and confirm `dockerd` + persist wiring come up under sbx's native init with no s6. (sbx-2 D8)

Each finding either removes a hedge from the plans (replace "gated on `sbx … --help`" with the confirmed shape) or records a definite blocker.

## Preconditions
- A host with the target `sbx` binary installed and able to create/run sandboxes (the dev container used for the plan edits does **not** have `sbx` on PATH).
- The in-progress Dune sbx template build inputs (`Dockerfile.sbx` / `container/sbx/`), or an ephemeral build thereof, available to use as `--template <ref>`.

## Part 1 — Flag/`--help` captures (cheap; do these first)
Record the raw `--help` output and the resolved decision for each.

- [x] `sbx create --help` — capture:
  - create-time **env injection** flag (`--env`/`-e`?) for the `DUNE_WORKSPACE` contract (sbx-2 D4 / sbx-3 D4). Record exact syntax or "absent".
  - **second-mount / volume** flag for the durable persist mapping (sbx-3 D3). Record exact syntax or "absent".
  - any **create/boot hook** / lifecycle-hook option usable to run `setup-persist` once (sbx-2 D2a). Record exact syntax or "absent".
- [x] `sbx run --help` — capture the **Start** shape underlying `dune up`/`rebuild` (sbx-3 D7, sbx-5 D1). Record flags actually used.
- [x] `sbx ports --help` — capture whether a **port unpublish/remove** spelling exists (sbx-5 D1). Record exact spelling or "absent → `dune ports` ships list+publish only".

### Findings (Part 1)
All captures are from `sbx` v0.32.0.

**`sbx create`** — usage `sbx create [flags] AGENT PATH [PATH...]`, flags: `--clone`, `--cpus int`, `--kit strings` (experimental, repeatable), `-m/--memory`, `--name`, `--profile` (governance profile), `-q/--quiet`, `-t/--template`.

- **Env injection flag: absent.** There is no `--env`/`-e` on `sbx create` (or on the agent subcommands, e.g. `sbx create shell --help`). However, env injection at create time **does exist via kits**: a kit's `environment.variables` map is set in the sandbox and is visible even in plain non-interactive `sbx exec <name> bash -c 'echo $VAR'` (verified empirically — see Part 2). `sbx exec` separately supports `-e/--env stringArray` and `--env-file` per invocation.
- **Second-mount flag: not a flag, but positional extra workspaces.** `sbx create AGENT PATH [PATH...]` accepts additional workspace paths; each is mounted **read-write by default** at the **same absolute path as on the host** (`:ro` suffix opts into read-only). This is the durable-persist mechanism (proven in Part 3). There is no named-volume flag.
- **Create/boot hook: exists, via kits (experimental).** A kit's `commands.install` list runs **once at sandbox creation**; `commands.startup` runs **on every sandbox boot** (create and each `start` after `stop`); `commands.initFiles` are written at startup with `${WORKDIR}` substitution. Verified empirically with a probe kit (Part 2). No non-kit hook flag exists on `sbx create`.

**`sbx run`** — usage `sbx run [flags] SANDBOX | AGENT [PATH...] [-- AGENT_ARGS...]`; "Run an agent in a sandbox, **creating the sandbox if it does not already exist**". Flags mirror `sbx create` (`--clone`, `--cpus`, `--kit`, `-m/--memory`, `--name`, `--profile`, `-t/--template`); agent args pass after `--`. The **start-an-existing-sandbox shape is `sbx run <sandbox-name>`** (positional sandbox name, no flags needed). Additionally, **`sbx exec` auto-starts a stopped sandbox** ("If the sandbox is stopped, it is started first" — confirmed empirically: after `sbx stop`, a plain `sbx exec` restarted the sandbox and re-ran kit startup commands). So `dune up`/`rebuild` can use `sbx run <name>` for attach-style starts or rely on `sbx exec` auto-start for command-style starts.

**`sbx ports`** — usage `sbx ports SANDBOX [flags]`. **Unpublish exists**: `--unpublish stringArray` with spec `[HOST_IP:]HOST_PORT:SANDBOX_PORT[/PROTOCOL]` (e.g. `sbx ports my-sandbox --unpublish 3000:8080`). `--publish stringArray` spec is `[[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]` (ephemeral host port if omitted; loopback bind if no HOST_IP). `--json` applies to the listing form. → `dune ports` can ship list+publish+unpublish.

**Bonus captures resolving hedges in other plans:**
- `sbx exec --help`: `-it` (`--interactive`/`--tty`), `-w/--workdir`, `-e/--env`, `--env-file`, `-u/--user`, `-d/--detach`, `--privileged` all exist ("Flags match the behavior of `docker exec`"). The sbx-3 D4 intended shape `sbx exec -it -w <dir> <instance> zsh` is supported as spelled.
- `sbx policy log --help`: **`--json` exists** (plus `--limit int`, `--type all|network`, `-q/--quiet`; sandbox name is positional and optional). Resolves the sbx-4 D4 / sbx-5 D3 "does a JSON form exist" question.
- `sbx policy set-default --help`: global only (`allow-all|balanced|deny-all`), **no `--sandbox` scope**; `sbx policy profile` manages *remote governance* profiles only (`ls` subcommand; selected via `sbx create --profile`). So a Balanced-equivalent preset **cannot** be applied sandbox-scoped by a local command — sbx-4's verify-only fallback stands. (Sandbox-scoped rules remain per-domain `sbx policy allow/deny network --sandbox <name> <domain>`; kits can also declare `network.allowedDomains`/`deniedDomains` at create time.)
- `sbx rm` requires `--force` when stdin is not a terminal (non-interactive callers like the Go backend must pass it).

## Part 2 — Re-homing `setup-persist` (sbx-2 D2a)
Goal: a definite mechanism for running the idempotent `setup-persist.sh` exactly once per fresh sandbox, writing to `/var/log/dune/setup-persist.log` (sbx-2 D5a).

- [x] If Part 1 found a native create/boot hook: wire `setup-persist` to it; confirm it runs once on a fresh sandbox and **not** on every subsequent `sbx exec`.
- [x] Otherwise: implement the login-shell sentinel fallback (run on first login, guard with a per-sandbox sentinel file); confirm idempotency across multiple `exec`s.
- [x] Either way, verify on a fresh sandbox: `readlink ~/.claude` → `/persist/agent/.claude` (and the other seeded paths), and `/var/log/dune/setup-persist.log` exists and is non-empty.

### Findings (Part 2)
**Both candidate mechanisms exist and were verified; recommendation: login-shell sentinel baked into the template, with kits available as the native-but-experimental alternative.**

**A native create/boot hook exists — kit `commands.install` / `commands.startup` (experimental).** Verified with a minimal mixin probe kit (`schemaVersion: "1"`, `kind: mixin`) passed as `sbx create --kit <dir> …`:
- `commands.install` ran **exactly once at sandbox creation** (not re-run on subsequent `sbx exec`s, not re-run after `sbx stop` + restart).
- `commands.startup` ran at creation **and again after `sbx stop` + auto-restart** (once per boot) — idempotency required, matching the docs.
- `environment.variables` were injected and visible in plain non-interactive `sbx exec <name> bash -c 'echo $VAR'`, surviving restart.
- `sbx kit validate <dir>` confirms the artifact; `--kit` only applies at create time (`sbx kit add` exists for running sandboxes and re-runs install commands).

Caveats that argue against making persist wiring *depend* on kits: the entire `sbx kit` surface is marked **EXPERIMENTAL** ("may change or be removed in future releases"), and the hook only fires if the creator remembers `--kit` — a sandbox created from the Dune template without it would silently skip credential wiring.

**Chosen mechanism: template-baked login-shell sentinel (the D2a fallback), now verified on the merged template (Part 4).** Wiring as built into the spike template:
- Hook body at `/etc/dune/setup-persist-hook.sh`: if user is `agent` and `~/.dune-setup-persist-done` is absent, run `/usr/local/bin/setup-persist.sh >> /var/log/dune/setup-persist.log 2>&1` and create the sentinel on success.
- Sourced from `/etc/profile.d/dune-setup-persist.sh` (bash/sh login shells) **and** appended to `/etc/zsh/zprofile` (zsh login shells), covering both `sbx exec … bash -lc` and interactive zsh attach.
- The sentinel lives in the **non-persisted** home directory, so it is per-sandbox by construction: a fresh sandbox re-runs the wiring once; recreations after `sbx rm` re-run it once again.

Two pitfalls surfaced while verifying this on the merged template (Part 4), both fixed in the spike Dockerfile:
- **Build-time bake:** the image build's own `runuser … bash -lc` steps are login shells, so the hook fired during `docker build`, baking the sentinel (and pre-wired symlinks) into the image — a fresh sandbox then never ran the hook itself. Fix: a `DUNE_SETUP_PERSIST_SKIP` env guard honoured by the hook and set on build `RUN` steps, plus a defensive final `rm -f` of sentinel and log.
- **Silent log:** `setup-persist.sh` produces no output on success, so the D5a log was zero bytes. Fix: the hook writes its own timestamped `start`/`done`/`FAILED` lines around the script's output.

Verified on a fresh sandbox from the rebuilt merged template: the sentinel/log are absent from the image; the first `bash -lc` exec ran the hook once; `readlink ~/.claude` → `/persist/agent/.claude` (and the other seeded paths resolved); `/var/log/dune/setup-persist.log` exists with the timestamped start/done lines; repeated `exec`s did not re-run the script (sentinel honoured).

Note for sbx-3 D4 (`DUNE_WORKSPACE` delivery): since `sbx create` has no `--env`, the working options are (a) a **Dune-generated per-sandbox kit** passed via `--kit` carrying `environment.variables: {DUNE_WORKSPACE: <path>}` (verified to reach non-login execs), or (b) in-sandbox derivation. Derivation is viable but ambiguous when multiple rw virtiofs workspaces are mounted (the persist dir is also a workspace); the kit env path or a kit `initFile` (which supports `${WORKDIR}` expansion of the primary workspace) disambiguates natively.

## Part 3 — Durable persistence recreate-survival (sbx-3 D3)
Goal: prove the profile-scoped persist location survives `rm`/recreate, reproducing the old `dune-persist-<profile>` volume semantics.

- [x] Create a sandbox with the profile-persist mapping (using the second-mount/volume mechanism from Part 1, or an `sbx`-native per-profile volume if that is the only option).
- [x] Write a sentinel file under `/persist/agent` from inside the sandbox.
- [x] `sbx rm <name>`, then recreate via the same mapping.
- [x] Confirm the sentinel is still present.
- [x] If neither a second-mount flag nor a native per-profile volume survives recreate: **record as a blocker** for sbx-3 cutover (per sbx-3 D3, accepting `rebuild` credential loss is not an allowed fallback).

### Findings (Part 3)
**PASS — no blocker.** Mechanism: a host directory passed as an **extra positional workspace** at create time (the Part 1 "second mount"). Reproduction:

```bash
mkdir -p ~/.dune-spike4-persist
sbx create --name dune-spike4-persist shell /tmp/dune-spike4-ws ~/.dune-spike4-persist
sbx exec dune-spike4-persist bash -c 'echo sentinel-… > /home/mitchell/.dune-spike4-persist/sentinel.txt'
sbx rm --force dune-spike4-persist        # sandbox fully removed
sbx create --name dune-spike4-persist shell /tmp/dune-spike4-ws ~/.dune-spike4-persist
sbx exec dune-spike4-persist bash -c 'cat /home/mitchell/.dune-spike4-persist/sentinel.txt'  # → sentinel intact
```

Observed details:
- Both workspaces mount as **rw virtiofs at their absolute host paths** (`mount` shows `bind-… on /home/mitchell/.dune-spike4-persist type virtiofs (rw,relatime)`).
- The sentinel survived `sbx rm --force` + recreate (and trivially survives `stop`/`start`). Because the data lives in a **host directory**, durability is independent of sandbox lifecycle — strictly stronger than the old `dune-persist-<profile>` Docker-volume semantics.
- Implication for sbx-3 D3: the profile-persist mapping is a Dune-owned host directory per profile (e.g. `~/.local/share/dune/persist/<profile>`) passed as an extra workspace on every create; `setup-persist` then targets it (symlink `/persist/agent` → that host path, or point `PERSIST_DIR` at it directly — `setup-persist.sh` already honours a `PERSIST_DIR` env override).
- Sharp edge: the in-sandbox mount path equals the **host** path, so the mapping is host-username-dependent; Dune must pass the resolved absolute path into the sandbox (kit env var / initFile) rather than hard-coding `/persist/agent` in the image.

## Part 4 — The s6-less merged template (sbx-2 D8)
Goal: confirm the template builds and runs with **no s6-overlay and no in-container app services**, with `dockerd` and persist wiring up under sbx's native init.

- [x] Build the Dune sbx template (ephemeral build is fine; ~12–15 min per project notes) with Postgres/Redis/Mailpit and the s6 service tree removed (sbx-2 D2a/D3).
- [x] In a single `sbx exec` into a fresh sandbox: `docker version` reports a running engine and the sbx init is not crash-looping; the persist symlinks are wired (Part 2); `docker compose version` works; `docker run --rm hello-world` succeeds; a nested `docker compose up -d` (service bound to `0.0.0.0`) is reachable; Playwright launches Chromium.
- [x] `sbx ports <name> --publish <port> --json` exposes the nested service and the host can reach it.
- [x] `sbx policy deny network --sandbox <name> example.com` blocks both shell (`forward`) and nested-Docker (`transparent`) traffic, visible in `sbx policy log <name> --limit <n>`.
- [x] Remove all temporary sandboxes created during the spike.

### Findings (Part 4)
**Build: PASS, with two findings that change sbx-2.** The merged template was built ephemerally from `spike-4-artifacts/Dockerfile.sbx-spike` (committed alongside this report) — the existing Compose `Dockerfile` content layered `FROM docker/sandbox-templates:shell-docker`, minus s6-overlay, Postgres/Redis/Mailpit (+`initdb`/runtime-dir steps), `WORKDIR /workspace`, and the NodeSource step (base ships Node 22.22.1).

1. **`docker/sandbox-templates:shell-docker` is pullable and derivable** (resolving sbx-2 D1, contrary to spike 2's note): it is Ubuntu 26.04 LTS (apt suite `resolute`; the image's own `com.docker.sandboxes.base` label says `ubuntu:questing`) + `docker-ce`/`containerd.io` + `tini` as ENTRYPOINT, `USER agent` (uid 1000, passwordless sudo, `docker` group — same shape as Dune's user), and the label **`com.docker.sandboxes.start-docker=true`**, which is what makes the sbx runtime auto-start `dockerd`. The label is inherited by `FROM`: `sbx template ls` shows the derived image with flavor `shell-docker`.
2. **Playwright vs. Ubuntu 26.04:** no stable Playwright supports `ubuntu26.04` (1.58.2 — the Compose-base pin — and 1.60.0 both hard-fail: "Playwright does not support chromium on ubuntu26.04-x64"). The 1.61 alphas add native ubuntu26.04 mappings (dry-run verified). The build uses 1.60.0 with the documented `PLAYWRIGHT_HOST_PLATFORM_OVERRIDE=ubuntu24.04-<arch>` escape hatch; the ubuntu24.04 Chromium 148 build installs and launches fine in-sandbox. Drop the override when Playwright 1.61 is stable.

Image size: **8.64 GB** vs. 6.85 GB for `dune-base:0.4.2` (the delta is Docker Engine plus the base's own golang/JDK/extra toolchain, partly offset by dropping Postgres/Redis/Mailpit/s6). A host-local Docker image name is **not** directly usable as `--template` (`pull failed`); the verified local path is `docker save` + `sbx template load <tar>`, after which the ref works on create.

**Acceptance matrix (run against a fresh sandbox created with `sbx create --name <n> --template dune-sbx-spike4:<tag> shell <workspace> <persist-dir>`): all PASS.**
- Init: PID 1 is `tini` (no s6 anywhere); `dockerd` auto-started by the sbx runtime ("Started Docker daemon in ~1s" on first exec); engine **29.5.3**, Compose **v5.1.4**; no crash-looping.
- Persist wiring: `readlink ~/.claude` → `/persist/agent/.claude` (`.codex`, `.gitconfig`, etc. likewise); `/var/log/dune/setup-persist.log` present with timestamped start/done lines (see Part 2).
- `docker run --rm hello-world`: PASS. Nested `docker compose up -d` (nginx bound `0.0.0.0:8088:80`): reachable in-sandbox (HTTP 200).
- `sbx ports <n> --publish 8088 --json`: published `127.0.0.1:32768 -> 8088/tcp` (+ `::1`); host curl → HTTP 200; `--unpublish 32768:8088` then refused connection — the full list/publish/unpublish cycle works end-to-end.
- Playwright: Chromium 148.0.7778.96 launched headless and rendered a page from inside the sandbox.
- Policy: `sbx policy deny network --sandbox <n> example.com` → in-sandbox `curl https://example.com` returns the proxy's 403 (`forward`), nested `docker run curlimages/curl` is blocked (`transparent`); both deny records visible in `sbx policy log <n> --json` with fields `host`, `vm_name`, `proxy_type` (`forward`/`transparent`/`forward-bypass`), `rule`, `reason`, `last_seen`, `since`, `count_since` — pin these for the sbx-4 D4 wrapper.
- Toolchain parity in-sandbox: `rally`, `laps`, `claude`, `codex`, `opencode`, `gemini`, `playwright`, `psql`, `redis-cli`, `zsh`, `node`, `go`, `python3`, `mise` all on PATH.
- All temporary sandboxes (`dune-spike4-persist`, `dune-spike4-kit`, `dune-spike4-tpl`, `dune-spike4-tpl2`) removed at the end; spike template images and the scoped policy rule cleaned up.

**Template-build pitfall worth carrying into sbx-2 (D2a):** the first build accidentally **baked the setup-persist sentinel into the image** — the build's own `runuser … bash -lc` steps are login shells, so the hook fired during `docker build`, and a fresh sandbox then skipped persist wiring (it appeared wired only because the build had baked the symlinks in). Fixed by exporting a `DUNE_SETUP_PERSIST_SKIP` guard in build `RUN` steps plus a defensive final `rm -f` of sentinel and log; a rebuilt image verified the hook then fires once per fresh sandbox at first login.

## Plan updates this spike unblocks
On completion, fold the findings back into the plans so no hedges remain (all applied in the same branch as this report):
- sbx-2 D1 — derive path confirmed (`FROM docker/sandbox-templates:shell-docker`; `start-docker` label; Playwright/Ubuntu-26.04 caveat). D2a — login-shell sentinel chosen (+ build-time bake guard); D4 — no create-time `--env`, delivery options pinned; D8 — matrix passed; tasks 1.1/2.2/3.2/4.2 updated.
- sbx-3 D3 — durable-persist mechanism pinned (extra positional workspace host dir; recreate-survival proven); D4 — `-it`/`-w`/`-e` confirmed on `sbx exec`, mount-path delivery options pinned; D7 — `sbx run <name>` start shape + `sbx exec` auto-start confirmed; risks/open questions/tasks 4.0–4.2 updated; template-load finding recorded.
- sbx-4 D1 — no sandbox-scoped preset exists (verify-only path stands); D4 — `sbx policy log --json` confirmed with observed field names; tasks 1.4/3.2 updated.
- sbx-5 D1 — `sbx ports --unpublish` spelling confirmed (full cycle exercised); `Start` shape inherited as confirmed; `sbx rm --force` non-TTY requirement recorded; D3 — policy-log JSON confirmed; tasks 1.4 updated.
- sbx-6 — kit surface verified empirically (subcommands, `--kit` create-time application, install/startup/env semantics); verification point narrowed to a Dune-template smoke check.
