# Spike 4: Flag Resolution, Durable Persistence, and the s6-less Template
Date: TBD
Branch: `experiment/docker-sbx`
Status: **planned, not yet executed** — this is the execution checklist, to be run where the `sbx` binary is installed and run; fill in the "Findings" sections as it is performed.

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

- [ ] `sbx create --help` — capture:
  - create-time **env injection** flag (`--env`/`-e`?) for the `DUNE_WORKSPACE` contract (sbx-2 D4 / sbx-3 D4). Record exact syntax or "absent".
  - **second-mount / volume** flag for the durable persist mapping (sbx-3 D3). Record exact syntax or "absent".
  - any **create/boot hook** / lifecycle-hook option usable to run `setup-persist` once (sbx-2 D2a). Record exact syntax or "absent".
- [ ] `sbx run --help` — capture the **Start** shape underlying `dune up`/`rebuild` (sbx-3 D7, sbx-5 D1). Record flags actually used.
- [ ] `sbx ports --help` — capture whether a **port unpublish/remove** spelling exists (sbx-5 D1). Record exact spelling or "absent → `dune ports` ships list+publish only".

### Findings (Part 1)
_TBD — paste `--help` output and the resolved spelling for each item._

## Part 2 — Re-homing `setup-persist` (sbx-2 D2a)
Goal: a definite mechanism for running the idempotent `setup-persist.sh` exactly once per fresh sandbox, writing to `/var/log/dune/setup-persist.log` (sbx-2 D5a).

- [ ] If Part 1 found a native create/boot hook: wire `setup-persist` to it; confirm it runs once on a fresh sandbox and **not** on every subsequent `sbx exec`.
- [ ] Otherwise: implement the login-shell sentinel fallback (run on first login, guard with a per-sandbox sentinel file); confirm idempotency across multiple `exec`s.
- [ ] Either way, verify on a fresh sandbox: `readlink ~/.claude` → `/persist/agent/.claude` (and the other seeded paths), and `/var/log/dune/setup-persist.log` exists and is non-empty.

### Findings (Part 2)
_TBD — chosen mechanism (native hook vs sentinel), exact wiring, idempotency result._

## Part 3 — Durable persistence recreate-survival (sbx-3 D3)
Goal: prove the profile-scoped persist location survives `rm`/recreate, reproducing the old `dune-persist-<profile>` volume semantics.

- [ ] Create a sandbox with the profile-persist mapping (using the second-mount/volume mechanism from Part 1, or an `sbx`-native per-profile volume if that is the only option).
- [ ] Write a sentinel file under `/persist/agent` from inside the sandbox.
- [ ] `sbx rm <name>`, then recreate via the same mapping.
- [ ] Confirm the sentinel is still present.
- [ ] If neither a second-mount flag nor a native per-profile volume survives recreate: **record as a blocker** for sbx-3 cutover (per sbx-3 D3, accepting `rebuild` credential loss is not an allowed fallback).

### Findings (Part 3)
_TBD — mechanism used, sentinel survival result, or blocker._

## Part 4 — The s6-less merged template (sbx-2 D8)
Goal: confirm the template builds and runs with **no s6-overlay and no in-container app services**, with `dockerd` and persist wiring up under sbx's native init.

- [ ] Build the Dune sbx template (ephemeral build is fine; ~12–15 min per project notes) with Postgres/Redis/Mailpit and the s6 service tree removed (sbx-2 D2a/D3).
- [ ] In a single `sbx exec` into a fresh sandbox: `docker version` reports a running engine and the sbx init is not crash-looping; the persist symlinks are wired (Part 2); `docker compose version` works; `docker run --rm hello-world` succeeds; a nested `docker compose up -d` (service bound to `0.0.0.0`) is reachable; Playwright launches Chromium.
- [ ] `sbx ports <name> --publish <port> --json` exposes the nested service and the host can reach it.
- [ ] `sbx policy deny network --sandbox <name> example.com` blocks both shell (`forward`) and nested-Docker (`transparent`) traffic, visible in `sbx policy log <name> --limit <n>`.
- [ ] Remove all temporary sandboxes created during the spike.

### Findings (Part 4)
_TBD — build outcome, acceptance-matrix results, image-size delta vs. the s6 base._

## Plan updates this spike unblocks
On completion, fold the findings back into the plans so no hedges remain:
- sbx-2 D2a/D8 — record the chosen `setup-persist` hook and the build outcome.
- sbx-2 D4 / sbx-3 D4 — pin env injection vs. in-sandbox derivation.
- sbx-3 D3 — pin the durable-persist mechanism (or record the blocker).
- sbx-3 D7 / sbx-5 D1 — pin the `sbx run` start shape and the `sbx ports` unpublish decision.
