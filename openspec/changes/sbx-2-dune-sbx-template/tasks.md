## 1. Docker-enabled template base

- [x] 1.1 ~~Spike the template base / flag captures~~ Resolved by spike 4: extend `FROM docker/sandbox-templates:shell-docker` (pullable; Ubuntu 26.04 + docker-ce + tini + `com.docker.sandboxes.start-docker=true` label, all inherited — see D1 and `sbx-1-runtime-spike/spike-reports/spike-4-artifacts/Dockerfile.sbx-spike`). `sbx create` has **no** env-injection flag; the landed D4 contract is exec-time `sbx exec -e DUNE_WORKSPACE=<absolute mount path> <name> bash -lc true` on the deterministic hook-firing exec, with a Dune-generated kit `environment.variables` left as the experimental create-time option. The only native create/boot hook is the experimental kit `commands.install`/`startup`, so `setup-persist` uses the template-baked login-shell sentinel (D2a).
- [ ] 1.2 Create the template build inputs (e.g. `Dockerfile.sbx` and/or a `container/sbx/` tree) separate from the root `Dockerfile`, reusing `container/base/scripts/*` (including `setup-persist.sh`), `container/base/tooling.yaml`, and `container/base/home-defaults/` where possible (D2). Do **not** carry over the `container/base/s6-overlay/s6-rc.d/` service tree or the Postgres/Redis/Mailpit installs (D2a/D3).
- [ ] 1.3 Let the sbx template's native Docker init own PID 1 and auto-start `dockerd` on the first `sbx exec` (no manual `dockerd` start), matching the generic `-docker` template behavior (D5). s6-overlay is not installed.
- [ ] 1.4 Verify inside an ephemeral sandbox, in a single `sbx exec`: `docker version` (engine reported), `docker compose version`, `docker run --rm hello-world`, and a nested `docker compose up -d` reachable from inside the sandbox, with the sbx init not crash-looping.

## 2. Remove in-container app services; re-home `setup-persist`

- [ ] 2.1 Drop the Postgres/Redis/Mailpit packages, their s6 service scripts, and their build/init steps (`postgresql`, `redis-server`, `mailpit`, the `initdb` step, the `/var/run/postgresql` handling) from the sbx template build (D3). The previously planned Postgres runtime-directory fix is removed with them.
- [ ] 2.2 Re-home `setup-persist` (`container/base/scripts/setup-persist.sh`) onto the login-shell sentinel hook (D2a, resolved by spike 4): hook body in `/etc/dune/setup-persist-hook.sh`, sourced from `/etc/profile.d/` and `/etc/zsh/zprofile`, sentinel `~/.dune-setup-persist-done`, timestamped output to `/var/log/dune/setup-persist.log` (D5a; explicit log lines, the script is silent on success). Guard the image build's own `bash -lc` steps against firing the hook (spike-4 pitfall: `DUNE_SETUP_PERSIST_SKIP` + final sentinel/log `rm -f`).
- [ ] 2.3 Verify inside a fresh sandbox that the persist symlinks are wired (`readlink ~/.claude` → `/persist/agent/.claude`, etc.) and `/var/log/dune/setup-persist.log` exists.
- [ ] 2.4 Verify that project-owned `docker compose` inside the sandbox can host app dependencies (e.g. a Postgres/Mailpit service via a nested Compose project) — this is now the only path for app dependency lifecycle.

## 3. Workspace path compatibility

- [ ] 3.1 Remove reliance on a static `WORKDIR /workspace` (root `Dockerfile` line 218) and the `/workspace` entry in the build-time `chown`; do not ship a `/workspace` directory that masks the mount (D4).
- [ ] 3.2 Add a first-run/login step that creates/refreshes `/workspace` as a symlink to the mounted repo path, consuming `DUNE_WORKSPACE` per the landed D4 contract: `sbx-3` creates the sandbox with `sbx create --name <name> --template <ref> shell <absolute mount path>`, then runs the deterministic hook exec as `sbx exec -e DUNE_WORKSPACE=<absolute mount path> <name> bash -lc true`. A Dune-generated kit's `environment.variables` remains the experimental create-time option; in-sandbox derivation is not the contract because it must disambiguate multiple rw virtiofs workspaces — the persist dir is also one.
- [ ] 3.3 Verify inside a sandbox created with the chosen mount-path mechanism (create with `sbx create --name <name> --template <ref> shell <absolute mount path>`, then run `sbx exec -e DUNE_WORKSPACE=<absolute mount path> <name> bash -lc true`): `readlink -f /workspace` resolves to the mounted repo and `git -C /workspace rev-parse --show-toplevel` succeeds.

## 4. Toolchain parity

- [ ] 4.1 Ensure the template installs zsh + Dune shell defaults, Rally, Laps, agent CLIs (`claude`, `codex`, `opencode`, `gemini`), `openspec`, `git`/`gh`, and language toolchains (Go, Node, Python, `uv`, `pnpm`).
- [ ] 4.2 Ensure Playwright + Chromium are installed and verify a minimal Chromium launch inside the sandbox. Note (spike 4): on the Ubuntu 26.04 base no stable Playwright installs natively — use `PLAYWRIGHT_HOST_PLATFORM_OVERRIDE=ubuntu24.04-<arch>` with 1.60.0 (Chromium launch verified) until Playwright 1.61 is stable.
- [ ] 4.3 Add a parity smoke check that runs `rally --version`, `laps --version`, `playwright --version` (and a representative agent CLI) inside a sandbox.

## 5. Versioning, build, and publish

- [ ] 5.1 Choose the template image reference and version scheme (e.g. `ghcr.io/mitchell-wallace/dune-sbx:<version>`), kept in lockstep with the CLI `VERSION` (D6).
- [ ] 5.2 Add a build-and-push job for the template image (extend `image.yml` or add a workflow), publishing to GHCR.
- [ ] 5.3 Document `sbx create --template <ref>` usage, `sbx template load` for offline use, and `sbx secret set --registry` for private registries.
- [ ] 5.4 Document that the template is built from source only, is never produced via `sbx template save`, and is not a secret boundary (D7).

## 6. Acceptance verification (spike-2 matrix)

- [ ] 6.1 Run the gating acceptance matrix in an ephemeral sandbox (`sbx create --name <name> --template <ref> shell <mount path>`): sandbox starts and appears in `sbx ls --json`; `/workspace` resolves to the mounted repo; the re-homed `setup-persist` hook has wired the agent-home symlinks into `/persist/agent` and logged to `/var/log/dune/setup-persist.log` (D2a/D3/D5a); `dockerd` responds and the sbx init does not crash-loop; Docker + Compose work and `docker run --rm hello-world` succeeds; nested `docker compose up -d` (service bound to `0.0.0.0`) works; Playwright launches Chromium; `sbx ports <name> --publish <port> --json` exposes the nested service and the host can reach the published port.
- [ ] 6.2 Verify network mediation is intact: `sbx policy deny network --sandbox <name> example.com` blocks both shell (`forward`) and nested-Docker (`transparent`) traffic, with blocked records in `sbx policy log <name> --limit <n>`.
- [ ] 6.3 Remove all temporary sandboxes created during verification.

## 7. Documentation

- [ ] 7.1 Document the Dune sbx template: what it contains, how it is built/published/versioned, the `/workspace` compatibility behavior, the removal of in-container Postgres/Redis/Mailpit and s6-overlay (app deps now via project-owned Compose, a breaking change), and the re-homed `setup-persist` boot hook + `/var/log/dune/` log path. Note that the CLI runtime that launches it lands in `sbx-3-sbx-runtime-backend`.
