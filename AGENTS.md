When addressing container build issues, you should try building an ephemeral container to test things in. You have permission to do this. The compiled `dune` Go binary is the canonical entry point.
If a package or installer is part of the core container build, do not paper over failures by skipping it. Fix the install path so the tool actually installs successfully in the image.
The full base image is intentionally large and can take roughly 10-15 minutes to build on a partly cold cache; a local `0.2.2` verification build took 12m45s. Long quiet stretches during npm, Playwright, mise, or agent CLI installation are normal as long as the process is still alive.

When working through a plan, you should break your work into commits to checkpoint progress. Make sure that each commit leaves a state which builds successfully (docker build or go build as appropriate). After completing a plan, all work should be committed.

Ad-hoc work that forms a sizeable change should come with an offer to commit, pending confirmation of build status.

## The `dunex` branch: the sbx migration line

This branch (`dunex`) is the integration line for a deliberate **breaking hard-cut** of Dune's runtime substrate: from the local Docker Compose topology on `main` to the standalone `sbx` microVM-sandbox CLI launching a dedicated **Dune sbx template image**. `Dockerfile.dune` and the old proxy sidecar are dropped, not carried forward.

- **Staged as OpenSpec changes** under `openspec/changes/`: `sbx-1` (research spikes, done) → `sbx-2` (the sbx template image) → `sbx-3` (CLI runtime backend, the central cutover) → `sbx-4` (egress/secrets + proxy removal) → `sbx-5` (lifecycle/doctor) → `sbx-6` (kits + legacy teardown). Each depends on the **landed** implementation of the prior one, so they land on this branch in order. Execution is driven through Rally laps (`.laps/laps.json`).
- **Two divergent lines, one repo (for now).** `main` keeps the current Compose backend and is still released; `dunex` carries the sbx future. They are incompatible product directions once sbx lands. Keep them in one repo until the trigger below, because the sbx template still **reuses `container/base/` assets** (`scripts/*`, `tooling.yaml`, `home-defaults/`) — so config-layer fixes on `main` should flow to both.
- **Merge `main` in regularly; never rebase/force-push** this branch (it is shared and pushed). The prepare-laps convention also forbids history rewrites as a cleanup strategy.
- **Split trigger:** revisit promoting `dunex` to a separate repo / release line only *after* `sbx-6` removes the `container/base/` tree (the last shared asset) — at that point a split is lossless. A "separate release" identity does **not** require a separate repo: the sbx template gets its own image ref (`dune-sbx`) and its own version source (`container/sbx/IMAGE_VERSION`, introduced in `sbx-2`); `sbx-6` updates the version-bump checklist below to point the lockstep at the template.

The `## Architecture` section below now describes the **sbx** runtime that `sbx-3` cut this branch over to. (`main` still runs the Docker Compose topology; see *Two divergent lines, one repo* above.)

## Architecture: host vs container

The sandbox's repository mount (visible at its absolute host path, with `/workspace` as a compatibility symlink) contains the **user's project**, not the dune source code. The host-side `dune` CLI resolves the workspace and profile, builds a backend-agnostic environment `Spec`, validates the host `sbx` install (present, authenticated, daemon-healthy, minimum version), ensures the profile-specific persist directory, and launches a sandbox built from the **Dune sbx template** — there is no Docker Compose pair and no `Dockerfile.dune`.

Rally is an independently released tool that is installed into the sbx template from GitHub Releases and can self-update inside the sandbox. Repo-specific Rally configuration lives in `/workspace/rally.toml`.

## Manually publishing the base image

The `image.yml` workflow pushes to GHCR whenever the `build-and-push` job runs on `main`. It triggers automatically on `push` events to `main` that touch `container/base/IMAGE_VERSION` (or the workflow file itself). If a push event is missed and the image wasn't published, you can recover by dispatching the workflow manually:

```bash
gh workflow run image.yml --ref main
```

The `build-and-push` job condition (`github.ref == 'refs/heads/main'`) allows both `push` and `workflow_dispatch` events on `main`, so no temporary edits are needed.

## Version bump checklist

When bumping the version, update **both** files (always kept in lockstep — CLI and image versions are tied together):

- `VERSION` — consumed by the Go CLI at build time
- `container/base/IMAGE_VERSION` — consumed by the image build workflow
