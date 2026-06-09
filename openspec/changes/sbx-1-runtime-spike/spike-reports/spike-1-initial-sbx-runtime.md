# Spike 1: Initial sbx Runtime Investigation
Date: 2026-06-09
Branch: `experiment/docker-sbx`
Status: exploratory spike, not production implementation
## Purpose
This spike investigates whether the standalone `sbx` CLI could become a future runtime substrate for Dune.
Dune currently uses regular Docker Compose: the host-side Go CLI generates a Compose file, starts an `agent` container plus a Pipelock sidecar, bind-mounts the repository at `/workspace`, and persists profile state with Docker volumes. The future direction explored here is an sbx-backed runtime where Dune creates or enters a microVM sandbox instead of a regular Docker Compose workspace.
This report documents observed behavior and gaps from small experiments. It does not propose a production migration yet.
## Current host and sbx state observed
The standalone `sbx` CLI is installed and authenticated.
Observed:
- `sbx` path: `/usr/bin/sbx`
- `sbx version`: `v0.32.0`
- `sbx diagnose --output json`: all checks passed
- daemon: healthy
- authentication: signed in
- default network policy selected for this spike: Open
The Open policy was confirmed with `sbx policy ls`:
```text
PROVENANCE   APPLIES_TO   POLICY/RULE         TYPE      DECISION   RESOURCES
local        all          default-allow-all   network   allow      **
```
Open policy was selected only to avoid policy noise during initial runtime tests. It is not the desired production posture.
## sbx capabilities relevant to Dune
The installed `sbx` CLI exposes the current features Dune would need to investigate:
- `sbx create`, `sbx run`, `sbx exec`, `sbx stop`, `sbx rm`, `sbx ls`
- `sbx secret` with `set`, `set-custom`, `ls`, and `rm`
- `sbx policy` with allow/deny/list/log/default-profile support
- `sbx ports` for publishing sandbox services to the host
- `sbx template` for loading, listing, saving, and removing templates
- `sbx kit` for experimental kit artifacts
- `--template` for custom template images
- `--clone` for a private in-sandbox Git clone workflow
- built-in agents including `shell`, `opencode`, `claude`, `codex`, and `gemini`
This means a future Dune sbx backend should target the standalone `sbx` CLI directly.
## Experiment 1: generic sbx shell sandbox
Command:
```bash
sbx create --name dune-sbx-standalone-probe shell /home/mitchell/Documents/Mycode/dune
```
Observed:
- sbx created a direct-mounted workspace sandbox.
- The default `shell` agent used a Docker-enabled sandbox template.
- `sbx ls` showed the workspace as `/home/mitchell/Documents/Mycode/dune`.
- `sbx exec` started with `pwd` at `/home/agent/workspace`.
- The actual Git repository was available at the absolute host path `/home/mitchell/Documents/Mycode/dune`.
- `git -C /home/mitchell/Documents/Mycode/dune rev-parse --show-toplevel` succeeded.
- The sandbox user was `agent`, UID 1000, with sudo and docker group membership.
- Docker Engine was available inside the sandbox.
- `docker compose version` inside the sandbox reported `Docker Compose version v5.1.4`.
- `docker run --rm hello-world` succeeded inside the sandbox.
- A marker file written under `/home/agent/.dune-sbx-probe/` persisted across `sbx stop` and restart.
Generic shell sandbox gaps:
- Rally was not installed.
- Laps was not installed.
- Playwright was not installed.
- Postgres, Redis, and Mailpit were not installed as Dune-provided services.
Implication:
The generic sbx shell template proves the core substrate Dune wants: isolated Docker and Docker Compose inside the sandbox. It does not provide Dune's batteries-included environment.
## Experiment 2: current Dune base image as an sbx template
Command:
```bash
sbx create --name dune-sbx-standalone-base-probe \
  --template ghcr.io/mitchell-wallace/dune-base:0.4.2 \
  shell /home/mitchell/Documents/Mycode/dune
```
Observed successes:
- sbx accepted `ghcr.io/mitchell-wallace/dune-base:0.4.2` as a custom template.
- The sandbox was created successfully.
- The repository was available at `/home/mitchell/Documents/Mycode/dune`.
- Dune's bundled tools were present:
  - `rally`
  - `laps`
  - `claude`
  - `codex`
  - `opencode`
  - `gemini`
  - `playwright`
  - `psql`
  - `redis-cli`
  - `redis-server`
  - `mailpit`
  - Go, Node, Python, and uv
- `playwright --version` returned `Version 1.58.2`.
- `rally --version` returned `rally v0.8.0`.
- `laps --version` returned `0.4.6`.
- Redis responded with `PONG`.
- Mailpit's HTTP API responded.
- Dune's persistence symlinks existed:
  - `/home/agent/.claude -> /persist/agent/.claude`
  - `/home/agent/.codex -> /persist/agent/.codex`
  - `/home/agent/.config/opencode -> /persist/agent/.config/opencode`
  - `/persist/agent` existed
Observed gaps:
- `pwd` was `/workspace`, because the Dune image has `WORKDIR /workspace`.
- `/workspace` was not the Git repository.
- The Git repository was at the absolute host path `/home/mitchell/Documents/Mycode/dune`.
- Docker was not installed in the Dune base template.
- `docker compose` was therefore unavailable in the Dune base template.
- Postgres did not respond during the probe.
Postgres details observed:
- The Postgres service supervisor was present.
- The `postgres` process was not running when inspected.
- `/var/lib/postgresql/data` existed.
- `/var/run/postgresql` did not exist when inspected.
- Running Postgres manually enough to capture output produced:
```text
FATAL: could not create lock file "/var/run/postgresql/.s.PGSQL.5432.lock": No such file or directory
```
This suggests the current image's Postgres startup path needs runtime-directory troubleshooting under sbx. The spike did not prove whether the missing directory is the only issue.
Implication:
The current Dune base image is useful evidence that much of the Dune environment can run under sbx, but it should not be treated as the final sbx template. It lacks Docker/Compose and has at least one service startup gap to investigate.
## Experiment 3: clone mode
Command:
```bash
sbx create --clone --name dune-sbx-clone-probe shell /home/mitchell/Documents/Mycode/dune
```
Observed:
- sbx detected the Git repository.
- sbx created a sandbox remote on the host named `sandbox-dune-sbx-clone-probe`.
- sbx reported the source as a read-only mount from `/home/mitchell/Documents/Mycode/dune` to `/run/sandbox/source`.
- `/run/sandbox/source` existed inside the sandbox and contained the repository.
- Immediate `sbx exec` landed in `/home/agent/workspace`, which was not a Git repository during the probe.
- Removing the sandbox removed the host sandbox remote.
Implication:
Clone mode is promising for stronger repository isolation, but its exact interactive and exec behavior needs more investigation before Dune depends on it. A copy-rather-than-mount model is acceptable to explore if it gives a cleaner safety boundary, but direct mounting also needs further investigation because it is simpler and closer to current Dune behavior.
## Network security and Pipelock migration story
Current Dune uses Pipelock as a sidecar to mediate outbound HTTP(S) traffic from the regular Docker Compose workspace.
The sbx direction should drop Pipelock for the sbx backend and rely on sbx as the network security layer.
Rationale:
- sbx provides network policy commands.
- sbx provides network logs through `sbx policy log`.
- sbx has a secret injection model through `sbx secret`.
- sbx policy is part of the sandbox runtime boundary rather than a sidecar Dune has to compose and configure.
- sbx can enforce policy at the substrate layer for the sandbox environment.
Observed with Open policy:
```bash
sbx policy log --limit 20
```
The log included:
- sandbox name
- network host
- proxy path, such as `forward` and `forward-bypass`
- rule/reason
- last seen time
- count
Production migration story:
1. Keep Pipelock for the current regular Docker backend while that backend exists.
2. For the sbx backend, do not start Pipelock.
3. Represent Dune's desired egress posture as sbx policy defaults and, later, possibly sbx template or kit configuration.
4. Move agent/API secrets away from persisted raw config where possible and toward `sbx secret` injection.
5. Use `sbx policy log` as the first-class network observability path for sbx-backed workspaces.
6. Once the sbx backend reaches parity, decide whether the regular Docker backend and Pipelock remain supported or are retired.
The production default should not be Open. This spike used Open only to reduce friction while probing runtime behavior.
## Docker and Docker Compose inside sbx
The generic sbx shell sandbox proves Docker and Docker Compose can run inside sbx.
Observed:
- Docker Engine was available.
- Docker Compose was available as `docker compose`.
- `docker run --rm hello-world` succeeded.
The current Dune base template did not include Docker. Therefore the next Dune sbx template should start from, or replicate the important behavior of, a Docker-enabled sbx template.
Open investigation:
- Determine the correct base image for a Dune sbx template that includes Docker and Compose.
- Determine whether Dune should derive from a Docker-enabled shell template or install/configure Docker itself.
- Verify that app-level `docker compose up`, volume creation, port publishing, and image builds work inside the Dune sbx template.
- Verify whether containers launched inside the sandbox can reach the sbx credential/network proxy correctly, especially for package installs and AI-provider requests.
## Repo mount vs copy model
Direct mount observations:
- Direct-mounted sbx workspaces are visible at the absolute host path.
- This differs from current Dune's `/workspace` convention.
- `/workspace` should not be assumed under sbx unless Dune creates its own compatibility layer.
Clone mode observations:
- sbx can create a private in-sandbox clone and a host `sandbox-<name>` remote.
- The source mount appears at `/run/sandbox/source`.
- More testing is needed to find the expected private clone working directory and how it behaves across `run`, `exec`, and agent attach flows.
Possible Dune approaches:
- Use absolute host path as the canonical sbx workspace path.
- Add a `/workspace` compatibility symlink or wrapper if safe and reliable.
- Use clone mode for safer agent work and fetch results back through the sandbox remote.
- Explore an explicit copy-in/copy-out model if mounting creates too many path or trust-boundary problems.
The copy-rather-than-mount model is acceptable to explore, but should be compared against direct mount and clone mode with concrete workflow tests.
## Dune sbx template direction
Dune likely needs a dedicated sbx template rather than using the current Compose-oriented Dune base image unchanged.
The template should include:
- Rally
- Laps
- supported agent CLIs where Dune still wants them available
- zsh and shell defaults
- Go, Node, Python, uv, and common development tooling
- Playwright and browser dependencies
- Redis
- Mailpit
- Postgres with sbx-safe runtime setup
- Docker and Docker Compose inside the sandbox
The initial template attempt showed that the current Dune base image can be launched as an sbx template and retains many Dune-specific tools and persistence symlinks. The gaps were Docker/Compose absence, `/workspace` mismatch, and Postgres startup behavior.
After Dune is successfully replicated onto an sbx backend, sbx kits may become useful for lighter-weight customization, network policy, and secret wiring. Kits should not be the primary focus until the template and runtime lifecycle are proven.
## Mapping table
```text
Dune concept              Current regular Docker shape                  Future sbx shape
workspace repo mount      bind mount repo at /workspace                 absolute-path mount, clone mode, or copy model; /workspace needs compatibility decision
profile persistence       Docker volume dune-persist-<profile>          named sandbox/profile mapping; VM state persists until rm; shared profile semantics need design
base image                ghcr.io/mitchell-wallace/dune-base            dedicated Dune sbx template, likely Docker-enabled
Dockerfile.dune           per-repo image extension via Compose build     future template customization or later kit-based extension; needs redesign
Pipelock egress           sidecar proxy service                          drop for sbx backend; rely on sbx policy, logs, and secrets
agent CLI config          symlinked persisted config under /persist      prefer sbx secret injection plus minimal persisted config
Rally                     installed in Dune base image                   install in Dune sbx template
Laps                      installed in Dune base image                   install in Dune sbx template
Postgres/Mailpit/Redis    services inside agent container                services inside Dune sbx template; Postgres needs startup troubleshooting
Playwright                installed in Dune base image                   install in Dune sbx template and verify browser execution
Docker in environment     not part of current Dune base image            required in Dune sbx template; generic sbx shell proves feasibility
shell attach              docker compose exec agent zsh                  sbx exec -it -w <workspace> <sandbox> zsh
logs                      docker compose logs and pipelock logs          sbx policy log plus service logs inside sandbox
rebuild                   rebuild Compose agent image                    rebuild/load/publish Dune sbx template; recreate sandbox as needed
down/up lifecycle         docker compose down/up                         sbx stop/run/rm/create
```
## Commands run during this spike
Representative commands:
```bash
sbx version
sbx --help
sbx create --help
sbx run --help
sbx exec --help
sbx secret --help
sbx policy --help
sbx ports --help
sbx kit --help
sbx template --help
sbx ls
sbx policy ls
sbx diagnose --output json
sbx create --name dune-sbx-standalone-probe shell /home/mitchell/Documents/Mycode/dune
sbx exec dune-sbx-standalone-probe sh -lc 'docker version'
sbx exec dune-sbx-standalone-probe docker compose version
sbx exec dune-sbx-standalone-probe sh -lc 'docker run --rm hello-world'
sbx stop dune-sbx-standalone-probe
sbx create --name dune-sbx-standalone-base-probe --template ghcr.io/mitchell-wallace/dune-base:0.4.2 shell /home/mitchell/Documents/Mycode/dune
sbx exec dune-sbx-standalone-base-probe sh -lc 'rally --version; laps --version; playwright --version'
sbx exec dune-sbx-standalone-base-probe sh -lc 'pg_isready -h 127.0.0.1 -p 5432; redis-cli -h 127.0.0.1 ping'
sbx create --clone --name dune-sbx-clone-probe shell /home/mitchell/Documents/Mycode/dune
sbx policy log --limit 20
sbx ports dune-sbx-standalone-probe --json
sbx rm dune-sbx-standalone-probe dune-sbx-standalone-base-probe dune-sbx-clone-probe
go test ./...
```
All temporary sandboxes were removed after testing.
## Recommended next step
Build a dedicated Dune sbx template spike.
Suggested scope:
1. Start from a Docker-enabled sbx-compatible base.
2. Add only the minimum Dune environment pieces needed for parity testing.
3. Ensure `docker` and `docker compose` work inside the sandbox.
4. Add Rally and Laps.
5. Add Redis, Mailpit, and Postgres with runtime-safe startup.
6. Add Playwright and verify a minimal browser launch.
7. Decide whether Dune attaches at the absolute workspace path, creates `/workspace` compatibility, or uses clone/copy mode.
8. Define the sbx lifecycle mapping for `up`, `down`, `logs`, `rebuild`, and profile naming.
9. Draft the Pipelock removal path for the sbx backend around sbx policy, logs, and secrets.
## Open questions
1. Should sbx become a parallel backend first, or should Dune move directly toward sbx as the future backend after parity is proven?
2. What compatibility level is required for existing `/workspace` assumptions?
3. Should Dune prefer direct mount, clone mode, or copy-in/copy-out for repositories?
4. Should Dune profiles map to sandbox names, sbx governance profiles, secret scopes, or some combination?
5. How much cross-repo profile persistence should survive in an sbx world?
6. What should replace `Dockerfile.dune`: a template Dockerfile, a generated template layer, or later kit-based customization?
7. What is the desired non-Open production network policy baseline for Dune on sbx?
8. Which secrets should be migrated first to `sbx secret` injection?
9. Should Pipelock remain only for the current regular Docker backend while sbx drops it entirely?
10. What minimum sbx version should Dune require?
