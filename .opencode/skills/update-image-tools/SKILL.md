---
name: update-image-tools
description: Rebuild the Dune base image with refreshed tool versions and cut a patch release. Use when the user wants to update the bundled agents/tools (claude, codex, opencode, gemini, rally, laps, agy, thenn) shipped in the image, bump the version, and publish. This is a host-side repo workflow — agents inside a running Dune container cannot do it (no docker build from inside the container); use the in-container `update-tools` command for ad-hoc runtime updates instead.
license: MIT
compatibility: Requires the Dune repo on `main`, a clean tree, docker, and `just`.
metadata:
  author: dune
  version: "1.0"
---

Rebuild the Dune base image with updated tool versions and release a patch.

This workflow runs **on the host against the Dune repo**, not inside a
container. The image installs the latest version of each tool at build time, so
a rebuild naturally picks up new releases; the release is what makes users get
them via `dune --update`.

## Prerequisites

- Branch: `main`, working tree clean.
- Tools available: `docker`, `just`, `go`, `gh` (authed).
- The base image build is large: expect ~10–15 min for a cold build and a similar
  CI runtime. Long quiet stretches during npm / Playwright / mise / agent CLI
  installs are normal as long as the process stays alive.

## Tools whose versions are refreshed by a rebuild

- npm: `claude`, `codex`, `opencode`, `gemini` (declared in `tooling.yaml` → `npm`)
- release binaries: `rally`, `laps`, `agy`, `thenn` (`tooling.yaml` → `release_scripts`)
- The Dockerfile installs these *unpinned* (latest at build time), so no version
  string in the Dockerfile normally needs changing for a refresh.

## Steps

1. **Survey current latest versions**

   ```bash
   # npm tools
   npm view @anthropic-ai/claude-code version
   npm view @openai/codex version
   npm view opencode-ai version
   npm view @google/gemini-cli version
   # release tools
   gh release view --repo mitchell-wallace/rally --json tagName -q .tagName
   gh release view --repo mitchell-wallace/laps  --json tagName -q .tagName
   gh release view --repo mitchell-wallace/thenn --json tagName -q .tagName
   ```

2. **Check smoke-test downgrade pins are still valid**

   `test/smoke/tool-updates.sh` (mirrored by `update_pin` in `tooling.yaml`) pins
   each updatable tool to an *older* version to prove `update-tools` can downgrade
   and then move back to latest. If any pinned release has been unpublished, or a
   pin equals the current latest (so the "moved off the pin" assertion fails),
   pick a new pin that is an older, still-installable version.

3. **Make any additive/removal changes**

   - Add a tool: follow the `add-new-agent` skill and `tooling.yaml` schema.
   - Remove a tool: drop it from `Dockerfile`, `tooling.yaml`, and the smoke
     assertions; verify nothing else references it.

4. **Bump the version (keep CLI + image in lockstep)**

   Patch both files to the same new version (e.g. `0.4.5`):

   ```bash
   printf '0.4.5\n' > VERSION
   printf '0.4.5\n' > container/base/IMAGE_VERSION
   ```

   See the "Version bump checklist" in `AGENTS.md`: `VERSION` is read by the Go
   CLI at build time, `container/base/IMAGE_VERSION` is read by the image build
   workflow; they must always match.

5. **Run static + unit checks**

   ```bash
   just test     # golangci-lint, shellcheck, hadolint, tooling-check, go test
   ```

   `tooling-check` (`cmd/check-container-tooling`) cross-validates `tooling.yaml`
   against the Dockerfile and both smoke suites — it catches missing wiring when
   adding/removing tools.

6. **Build and smoke-test the image locally**

   ```bash
   docker build --build-arg BUILDKIT_INLINE_CACHE=1 \
     --build-arg GITHUB_TOKEN="$(gh auth token)" -t dune-base:<ver> .
   just smoke-base  --skip-build --image dune-base:<ver>
   just smoke-tools --skip-build --image dune-base:<ver>
   ```

   Spot-check key versions, e.g. `rally version` and `laps version` inside the
   image, to confirm the rebuild picked up new releases.

7. **Commit and push to `main`**

   A single cohesive commit is fine (see prior releases for style). Pushing
   triggers two workflows:
   - `auto-tag` → creates `v<ver>` tag → dispatches `release` (goreleaser builds
     the CLI binary with the image version baked in via ldflags).
   - `image` → builds + pushes `ghcr.io/mitchell-wallace/dune-base:<ver>` and
     `:latest`.

8. **Monitor CI to green**

   ```bash
   gh run list --branch main -L 4
   gh run view <image-run-id> --repo mitchell-wallace/dune
   ```

   The `image` run is the long pole (~10–15 min). Do not release/announce until
   both `release` and `image` (verify + build-and-push) succeed.

9. **Verify the upgrade end-to-end**

   Install/refresh the host CLI and confirm it pulls the new image:

   ```bash
   dune --update                       # NOTE: it's the --update flag, not a subcommand
   dune --version                      # should print the new version
   docker pull ghcr.io/mitchell-wallace/dune-base:<ver>
   ```

   Optionally `dune up` in a throwaway workspace and confirm the running
   container reports `ghcr.io/mitchell-wallace/dune-base:<ver>`, then `dune down`.

## Reference

- **Version files (lockstep):** `VERSION`, `container/base/IMAGE_VERSION`.
- **Tool manifest:** `container/base/tooling.yaml` (npm / release_scripts /
  binary_releases; `update_pin` drives the tool-updates smoke test).
- **Update mechanism in-container:** the `update-tools` command (see
  `container/base/scripts/update-tools.sh`) — for ad-hoc runtime updates that do
  not require a release.
- **Workflows:** `.github/workflows/auto-tag.yml`, `release.yml`, `image.yml`.
