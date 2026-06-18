# Dune sbx Template — Contents, Persistence, and Workspace

This doc covers **what the Dune sbx template image contains and how it behaves
in-sandbox**: the toolchain it ships, the in-container services and s6-overlay
supervisor it removes (design `sbx-2` D2a/D3), the re-homed `setup-persist`
boot hook and the canonical `/var/log/dune/` log path (D2a/D5a), and the
`/workspace` compatibility symlink (D4).

It does **not** cover build, publish, versioning, registry, or the secret
boundary — those live in
[Distribution, Versioning, and Registry Access](./sbx-template-distribution.md)
(D6/D7). The CLI runtime that *launches* this template — the `dune` command
mapping onto `sbx create`/`sbx exec`, the `version.SbxTemplateRef()` accessor,
and the post-create hook exec — lands in
[`sbx-3-sbx-runtime-backend`](../../openspec/changes/sbx-3-sbx-runtime-backend).
Until `sbx-3` lands, the template is a build/publish artifact, not something
`dune` invokes.

The legacy Compose base image (`ghcr.io/mitchell-wallace/dune-base`) and the
[`container/base/`](../../container/base) tree remain until `sbx-6` retires
them; the sbx template reuses their `scripts/`, `tooling.yaml`, and
`home-defaults/` assets but is built from its own
[`container/sbx/`](../../container/sbx) inputs.

## What the template is

The Dune sbx template is the durable runtime artifact of the sbx backend — the
sbx-world equivalent of today's `dune-base` image. It is a Docker-enabled
sandbox base with Dune's batteries-included toolchain layered on top. Build
inputs live under [`container/sbx/`](../../container/sbx)
(`Dockerfile.sbx` + `etc/`); see
[Distribution, Versioning, and Registry Access](./sbx-template-distribution.md)
for how it is built, tagged, and published as
`ghcr.io/mitchell-wallace/dune-sbx:<version>`.

It derives `FROM docker/sandbox-templates:shell-docker` and **inherits**:

- **`tini` as PID 1** (`ENTRYPOINT ["tini", "--"]`, `CMD ["bash"]`) — the sbx
  template's native init, *not* s6-overlay.
- **Docker Engine + containerd + the `docker compose` plugin**, auto-started
  inside the sandbox.
- **`USER agent`** (uid 1000, passwordless `sudo`, `docker` group) — matching
  Dune's existing user.
- The OCI label **`com.docker.sandboxes.start-docker=true`**, which is what
  makes the sbx runtime auto-start `dockerd` in sandboxes from this template
  (D5). The label (and the sbx flavor detection) are inherited by `FROM`.
- **Node 22.x**, so unlike the Compose root `Dockerfile` there is no NodeSource
  step.

## What the template contains

The template reproduces Dune's toolchain on top of the Docker-enabled base
(sbx-2 task 4 — toolchain parity):

- **Shell:** `zsh` with Powerlevel10k and Dune shell defaults.
- **Agent CLIs:** `claude`, `codex`, `opencode`, `gemini`, plus `openspec`.
- **Dune harnesses:** Rally, Laps, `agy`, `thenn` (installed from GitHub
  Releases during the image build; Rally can self-update inside the sandbox).
- **Git/GitHub:** `git`, `gh`, `delta`, `gitui`.
- **CLI tooling:** `bat`/`fd` (aliased from `batcat`/`fdfind`), `eza`, `fzf`,
  `ripgrep`, `tre`, `tree`, `jq`, `tmux`, `micro`, `nano`, `vim`, `ping`.
- **Language toolchains via `mise`:** Go, Node, Python, `uv`, `pnpm`
  (`latest`, pinned in `~/.config/mise/config.toml`), exposed on
  `~/.local/bin` for parity with the Compose base.
- **Build/JS tooling:** `pnpm`, `turbo`.
- **Playwright + Chromium.** No stable Playwright supports the base's Ubuntu
  26.04 yet, so 1.60.0 is installed with the
  `PLAYWRIGHT_HOST_PLATFORM_OVERRIDE=ubuntu24.04-<arch>` escape hatch
  (Chromium 148 launch verified in-sandbox by spike 4). The override can be
  dropped once Playwright 1.61 is stable.
- **DB *clients*:** `postgresql-client` (`psql`) and `redis-tools` (`redis-cli`)
  are kept so projects can reach their **own** compose-hosted Postgres/Redis
  from inside the sandbox. The *servers* are not — see below.

The template reuses Dune's existing build assets rather than duplicating them:
`container/base/home-defaults/` (shell + agent defaults, baked to
`/opt/home-defaults/`), and the reusable installers
`container/base/scripts/{configure-agents,setup-persist,install-rally,install-laps,install-agy,install-thenn,tooling-data,update-tools}.sh`.
This asset reuse is why the `container/base/` tree is not removed until
`sbx-6`.

## What was deliberately removed (D2a/D3/D4)

Versus the Compose base image (root `Dockerfile`), the sbx template
**deliberately does not** ship:

- **s6-overlay.** The Compose base runs `s6-overlay` as PID 1
  (`ENTRYPOINT ["/init"]`) to supervise services. The sbx template's native
  Docker init (tini + the `start-docker` label) owns PID 1 instead. Running
  two supervisors would collide; only one is kept.
- **In-container Postgres, Redis, and Mailpit.** Their packages, s6 service
  scripts, the `initdb` step, and the `/var/run/postgresql` runtime-dir
  handling are all dropped from the sbx build. App dependencies now come from
  project-owned `docker compose` inside the sandbox (see
  [Migration note](#migration-note-app-deps-via-project-owned-compose)).
- **A static `WORKDIR /workspace` directory.** The root `Dockerfile`'s
  `WORKDIR /workspace` and its build-time `chown` are removed. The template
  ships **no** `/workspace`; one is created at runtime as a symlink (see
  [Workspace compatibility](#workspace-compatibility-symlink-d4)).
- **NodeSource Node 22.** The base already ships Node 22.x.

No service-native health probes (`pg_isready`, `redis-cli ping`, Mailpit HTTP)
exist in the template, and `sbx-5` doctor/logs depend on none.

## PID 1 and Docker (D5)

`tini` is PID 1, and the inherited `com.docker.sandboxes.start-docker=true`
label makes the sbx runtime auto-start `dockerd` so `docker` and
`docker compose` work in the first `sbx exec` without a manual daemon start —
matching the generic `-docker` template behavior. Nested
`docker compose up -d`, `docker run`, image builds, and volumes all work
inside the sandbox.

## Persistence: the re-homed `setup-persist` boot hook (D2a)

The Compose base runs `setup-persist` as an s6 `oneshot` that seeds and
symlinks agent credentials/config into the profile-persist volume
(`/persist/agent`): `~/.claude → /persist/agent/.claude`, and likewise for
`.codex`, opencode, gh config, `.gitconfig`, `.git-credentials`, `.zshrc`,
`.p10k.zsh`. This wiring is mandatory for persistence, so it survives the
s6 removal — re-homed onto a **login-shell sentinel hook** baked into the
template (the mechanism spike 4 settled on; the experimental `sbx kit`
create/boot hook is opt-in and must not be depended on):

- **Hook body:** [`/etc/dune/setup-persist-hook.sh`](../../container/sbx/etc/dune/setup-persist-hook.sh),
  sourced from
  [`/etc/profile.d/dune-setup-persist.sh`](../../container/sbx/etc/profile.d/dune-setup-persist.sh)
  (bash/sh login shells) and
  [`/etc/zsh/zprofile`](../../container/sbx/etc/zsh/zprofile)
  (zsh login shells).
- **Runs once per sandbox instance**, guarded by a sentinel at
  `~/.dune-setup-persist-done`. The sentinel lives in the **non-persisted**
  home, so recreating the sandbox re-runs the hook.
- **The backend forces it deterministically post-create** with one
  non-interactive login shell — `sbx exec <name> bash -lc true` — rather than
  relying on the user's first attach. Plain `bash -c` execs do not source
  profile files and do not fire the hook.

**Build-time pitfall (guarded):** the image build's own `bash -lc` steps are
login shells and would fire the hook during `docker build`, baking the
sentinel into the image so fresh sandboxes never run it. The template guards
against this with `DUNE_SETUP_PERSIST_SKIP=1` on build `RUN` steps that use
login shells, plus a final `rm -f` of the sentinel and log as the last root
step. A fresh sandbox is therefore guaranteed to re-run the hook on its first
login shell.

## Canonical in-container log path (D5a)

The re-homed `setup-persist` boot hook writes timestamped `start`/`done`/
`FAILED` lines (plus script output) to a fixed in-container path:

```
/var/log/dune/setup-persist.log
```

The explicit log lines matter because `setup-persist.sh` is silent on
success. This path is the **verified contract** `sbx-2` hands to `sbx-5`:
`dune logs` (`sbx-5`) reads `/var/log/dune/` via `sbx exec`. The directory is
created (owned by `agent`) during the image build.

## Workspace compatibility symlink (D4)

Under direct-mount sbx the repository is visible at its **absolute host path**
inside the sandbox, not at `/workspace`. The Compose base's static
`WORKDIR /workspace` is actively misleading in that world: `/workspace` would
exist but not be the repo.

The template therefore ships no `/workspace`. Instead, `/workspace` is created
at runtime as a **compatibility symlink to the real mounted repo**, so legacy
`/workspace` assumptions resolve correctly. The canonical working directory
remains the real mounted repo path; `/workspace` is a bridge only.

Because only the runtime backend knows the absolute mount path, the backend
(`sbx-3`) supplies it via the `DUNE_WORKSPACE` environment variable on the
deterministic hook-firing exec — the **landed sbx-2 contract**:

```sh
sbx create --name <name> --template <ref> shell <absolute mount path>
sbx exec -e DUNE_WORKSPACE=<absolute mount path> <name> bash -lc true
```

The login-shell hook consumes `DUNE_WORKSPACE`, resolves it to a physical
path, creates or refreshes `/workspace` as a symlink to that target, then
runs the sentinel-guarded `setup-persist` step. Later execs and user shells
do not need to carry `DUNE_WORKSPACE`; the symlink remains in the sandbox
filesystem until refreshed. `sbx create` has no `--env` flag (verified by
spike 4), which is why delivery is exec-time rather than create-time.

## Migration note: app deps via project-owned Compose

Removing the in-container Postgres/Redis/Mailpit services and s6-overlay is a
**deliberate breaking change** (spike-3 recommendations #4/#5). Docker-in-sbx
makes project-owned `docker compose` inside the sandbox the canonical path for
app dependencies, which eliminates the dual-init risk.

**If you relied on the auto-started in-container services**, bring your own
Compose stack inside the sandbox. For example, a project that previously
expected Postgres/Redis/Mailpit on localhost now ships a compose file and
brings it up inside the sandbox:

```sh
docker compose up -d   # run inside the sandbox; services reach each other
```

The template still ships the **clients** (`psql`, `redis-cli`) so you can
reach your compose-hosted Postgres/Redis from a shell. Mailpit has no
in-template equivalent; run it as another compose service.

App-dependency lifecycle, readiness, and logs now belong to that
project-owned Compose stack, not to the template.

## What lands later

- **`sbx-3-sbx-runtime-backend`** — the `dune` CLI runtime that launches this
  template: the `sbx create`/`sbx exec` command mapping, the
  `version.SbxTemplateRef()` accessor, and the post-create hook exec carrying
  `DUNE_WORKSPACE`. Until sbx-3 lands, no `dune` command invokes the template.
- **`sbx-4-sbx-network-and-secrets`** — the network policy baseline and
  `sbx secret` wiring (replacing Pipelock's role).
- **`sbx-5-sbx-lifecycle-and-doctor`** — `dune doctor`, the diagnostics
  taxonomy, and `dune logs` reading `/var/log/dune/`.
- **`sbx-6-sbx-kits-and-cleanup`** — `sbx` kits as the customization layer,
  and retirement of the legacy Compose base image + `container/base/` tree.
