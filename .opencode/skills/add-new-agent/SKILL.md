---
name: add-new-agent
description: Add a new CLI coding agent to the Dune base Docker image. Use when the user wants to add a new agent (e.g., Claude Code, Codex, Gemini, OpenCode, Antigravity) to the container build. Covers install, persist, update-tools, aliases, startup message, smoke tests, and version bump.
license: MIT
compatibility: Requires standard Dune repo tooling (tooling.yaml, Dockerfile, shell scripts).
metadata:
  author: dune
  version: "1.0"
---

Add a new CLI coding agent to the Dune base Docker image.

**Input**: The agent's name, install command (npm package name or curl-pipe-bash URL), how it verifies its version (e.g., `agy --version`), and whether it stores data in a home directory that should be persisted.

**Steps**

1. **Research the agent's install method and data directories**

   - Determine whether it's an npm package or installed via a release script (curl-pipe-bash)
   - Check what `~/.<name>/` directories it uses for config/auth that should survive container rebuilds
   - Determine its version command (e.g., `agy --version`)
   - Check if the install script supports version pinning via an env var (if using release_scripts pattern)

2. **Add to `container/base/tooling.yaml`**

   For an **npm package**:
   ```yaml
   npm:
     - name: <name>
       package: "<npm-package-name>"
       verify: <name> --version
       update: true
       update_pin: <current-version>
   ```

   For a **release script** (curl-pipe-bash):
   ```yaml
   release_scripts:
     - name: <name>
       install_script: /usr/local/bin/install-<name>.sh
       version_env: <NAME>_VERSION
       verify: <name> --version
       update: true
       update_pin: ""   # leave empty if install script doesn't support version pinning
   ```

3. **Install the agent in the Dockerfile**

   For a **release script**, create `container/base/scripts/install-<name>.sh` following the existing pattern (see `install-rally.sh` / `install-laps.sh` / `install-agy.sh`):
   ```bash
   #!/usr/bin/env bash
   set -euo pipefail
   tmpdir="$(mktemp -d)"
   cleanup() { rm -rf "${tmpdir}"; }
   trap cleanup EXIT
   curl -fsSL <install-url> -o "${tmpdir}/install.sh"
   bash "${tmpdir}/install.sh"
   ```

   Then in the Dockerfile:
   - `COPY` the install script
   - Add it to the `chmod 0755` line
   - Run it in the `runuser -u agent` block (after the existing agents)
   - Create `~/.<name>/` in the `install -d` home directory setup block if the agent has a config dir

   For an **npm package**, add it to the `npm install -g` RUN line.

4. **Persist the agent's home directory data**

   If the agent stores config in `~/.<name>/`:
   - Ensure an empty seed directory exists at `container/base/home-defaults/.<name>/` (already present for `.gemini/`)
   - Add `seed_dir ".<name>"` in `container/base/scripts/setup-persist.sh` (in the seed block after existing seed_dir calls)
   - Add `link_path ".<name>" ".<name>"` in `container/base/scripts/setup-persist.sh` (in the link block after existing link_path calls)
   - Create `/home/agent/.<name>/` in the Dockerfile's `install -d` home directory block

5. **Wire up update-tools support**

   - Add to `container/base/scripts/tooling-data.sh`:
     - For npm: add `"<name>:<npm-package>"` to the `NPM_TOOLS` array
     - For release: add `"<name>:/usr/local/bin/install-<name>.sh:<NAME>_VERSION"` to the `RELEASE_TOOLS` array
   - Add `<name>` to the `Tools:` line in `container/base/scripts/update-tools.sh` usage help text

6. **Add aliases**

   Two files to update:
   - `container/base/home-defaults/.agent-shell-setup.sh` — base image defaults
   - `container/home/.agent-shell-setup.sh` — agent container startup

   In each, add:
   ```bash
   if command -v <name> >/dev/null 2>&1; then
     alias <shortcut>='<name> --dangerously-skip-permissions'
   fi
   ```

7. **Add to the container startup message**

   In `container/home/.agent-shell-setup.sh`, add the agent to the `_show_agent_startup_message` function's "Sandbox aliases" section:
   ```bash
   if command -v <name> >/dev/null 2>&1; then
     printf '%s\n' "  <shortcut>      -> <name> --dangerously-skip-permissions (<DisplayName> CLI)"
   fi
   ```

8. **Add smoke tests**

   In `test/smoke/base-image.sh`, add:
   ```bash
   assert_container_command "<name> --version"
   ```

   In `test/smoke/tool-updates.sh`, add an update test:
   - If version pinning is supported: use the existing `update_npm_tool` pattern
   - If no pinning: add a simple `update-tools <name>` + `<name> --version` sequence (see the agy block)

9. **Handle the check-container-tooling consistency tool**

   - If the install script doesn't support version pinning and `update_pin` is empty, the check tool (`cmd/check-container-tooling/main.go`) allows empty pins — it only verifies the verify command exists in `tool-updates.sh`. No changes needed to the Go code unless the empty-pin behavior needs adjustment.

10. **Build and verify**

    - Run `go build ./...` to verify Go compilation
    - Run `go run ./cmd/check-container-tooling` to verify manifest consistency
    - Run `docker build -t dune-base-test .` to verify the image builds
    - Run `docker run --rm --entrypoint bash dune-base-test -lc "<name> --version"` to verify the agent works
    - Run `docker run --rm --entrypoint bash dune-base-test -lc "update-tools <name>"` to verify update-tools works
    - Mount a test persist volume and verify `readlink -f /home/agent/.<name>` points to `/persist/agent/.<name>`

11. **Bump versions and commit**

    - Bump `VERSION` and `container/base/IMAGE_VERSION` (patch bump for a single agent addition)
    - Commit with a descriptive message listing all files changed
    - Push

**Files to modify (checklist)**

| File | Action |
|------|--------|
| `container/base/tooling.yaml` | Add agent entry |
| `container/base/scripts/install-<name>.sh` | NEW — install script (release only) |
| `Dockerfile` | COPY, chmod, run install, create home dir |
| `container/base/scripts/tooling-data.sh` | Add to NPM_TOOLS or RELEASE_TOOLS |
| `container/base/scripts/update-tools.sh` | Add to Tools: help text |
| `container/base/scripts/setup-persist.sh` | seed_dir + link_path for config dir |
| `container/base/home-defaults/.agent-shell-setup.sh` | Add alias |
| `container/home/.agent-shell-setup.sh` | Add alias + startup message entry |
| `test/smoke/base-image.sh` | Add version assertion |
| `test/smoke/tool-updates.sh` | Add update test |
| `cmd/check-container-tooling/main.go` | Only if empty-pin logic needs changes |
| `VERSION` | Bump patch |
| `container/base/IMAGE_VERSION` | Bump patch |

**Guardrails**
- Follow the existing patterns — copy from `install-rally.sh`/`install-laps.sh`/`install-agy.sh` for release scripts
- If version pinning isn't supported, set `update_pin: ""` and use the simple update test pattern
- Always create the `~/.<name>/` directory in both the Dockerfile home setup and seed/link it in setup-persist.sh
- Run the check-container-tooling tool after all changes to catch consistency issues early
- Keep the version bump minimal (patch) unless the scope warrants more
- Always verify the image builds and the agent responds to `--version` before committing
