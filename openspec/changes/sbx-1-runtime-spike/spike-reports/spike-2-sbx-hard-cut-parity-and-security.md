# Spike 2: sbx Hard-Cut Parity and Network Security
Date: 2026-06-10
Branch: `experiment/docker-sbx`
Status: exploratory spike, not production implementation
## Purpose
This spike continues from `spike-1-initial-sbx-runtime.md` with a stronger product assumption: Dune should move decisively toward an `sbx`-backed runtime rather than teetering between a long-lived Docker Compose backend and an `sbx` backend.
The working direction is:
- hard-cut away from the current regular Docker Compose runtime once `sbx` parity is sufficiently verified;
- drop `Dockerfile.dune` instead of carrying it forward, because it has not been used in practice;
- use a well-rounded Dune `sbx` template as the near-term customization unit;
- treat kits as the future maturity path for extension/customization after the base runtime is proven;
- drop Pipelock for the `sbx` runtime and rely on `sbx` policy, logs, and secrets.
The goal of this spike was to test the important runtime and security assumptions that shape that refactor.
## Current sbx state observed
Observed host state:
- `sbx` path: `/usr/bin/sbx`
- `sbx version`: `v0.32.0`
- `sbx diagnose --output json`: all 8 checks passed
- daemon healthy
- authenticated
- no probe sandboxes remained after cleanup
The default policy still includes a local allow-all rule:
```text
local all default-allow-all network allow **
```
That is useful for exploratory probing, but it should not be the production Dune default.
## Decision shift from spike 1
Spike 1 still framed the question as "parallel backend first or direct move to sbx?".
This spike changes the working assumption:
```text
sbx is the definite future runtime shape.
```
That changes the refactor target. The question is no longer how to preserve two backends indefinitely. The question is how to reshape Dune around `sbx` cleanly enough that the hard cut is safe.
Implications:
- Do not preserve `Dockerfile.dune`; remove it from the future runtime model.
- Do not design for a long-lived dual-backend matrix unless a concrete blocker appears.
- Continue to use a semantic planning boundary if it helps structure the code, but do not over-generalize around hypothetical backends.
- Treat the current Docker Compose implementation as migration scaffolding, not a peer backend to support forever.
- Dune's durable runtime artifact should become an `sbx` template, with kits later.
## Experiment 1: generic Docker-enabled sbx template
Command:
```bash
sbx create --name dune-sbx-spike2-shell shell /home/mitchell/Documents/Mycode/dune
```
Observed:
- `sbx` used Docker's `docker/sandbox-templates:shell-docker` template internally.
- `sbx exec` started Docker daemon automatically.
- user was `agent`, UID `1000`, with `sudo` and `docker` groups.
- `pwd` was `/home/agent/workspace`.
- `/workspace` did not exist.
- the Dune repository was available at the absolute host path `/home/mitchell/Documents/Mycode/dune`.
- `git -C /home/mitchell/Documents/Mycode/dune rev-parse --show-toplevel` succeeded.
- `git rev-parse --show-toplevel` from the default `pwd` failed.
- Docker Engine was available: `29.5.3`.
- Docker Compose was available: `Docker Compose version v5.1.4`.
Nested Compose test:
- created `/tmp/dune-compose-probe/compose.yaml` inside the sandbox;
- ran `docker compose up -d`;
- started a `python:3.12-alpine` HTTP server;
- `docker compose ps --services --status running` returned `web`;
- `curl http://127.0.0.1:18081` inside the sandbox succeeded.
Implication:
Docker-in-sbx, including Compose, is real enough to build around. The generic template proves the substrate, but it lacks Dune's batteries-included toolchain.
## Experiment 2: sbx ports and nested Compose services
First nested Compose service bound loopback only:
```yaml
ports:
  - "127.0.0.1:18081:8000"
```
Inside sandbox:
- curl to `127.0.0.1:18081` succeeded.
Then published through `sbx`:
```bash
sbx ports dune-sbx-spike2-shell --publish 18081 --json
```
Observed:
- `sbx` published host loopback port `32769` to sandbox port `18081`.
- host curl to the published port failed with connection reset.
Second nested Compose service bound all sandbox interfaces:
```yaml
ports:
  - "18082:8000"
```
Inside sandbox:
- curl to `127.0.0.1:18082` succeeded.
Published through `sbx`:
```bash
sbx ports dune-sbx-spike2-shell --publish 18082 --json
```
Observed:
- `sbx` published host loopback port `32770` to sandbox port `18082`.
- host HTTP request to `127.0.0.1:32770` returned status `200`.
Implication:
`sbx ports` works, including for nested Docker/Compose services, but Dune must account for bind address. Services bound only to sandbox loopback may not be reachable through `sbx` port publishing. Dune docs, diagnostics, or helpers should guide projects toward binding dev servers to all sandbox interfaces when host exposure is desired.
## Experiment 3: current Dune base image as sbx template
Command:
```bash
sbx create --name dune-sbx-spike2-base --template ghcr.io/mitchell-wallace/dune-base:0.4.2 shell /home/mitchell/Documents/Mycode/dune
```
Observed:
- sandbox created successfully;
- `pwd` was `/workspace`;
- `/workspace` existed but was not the Git repository;
- the Git repository was available at `/home/mitchell/Documents/Mycode/dune`;
- Docker was missing;
- `docker compose` was missing;
- Rally was present: `rally v0.8.0`;
- Laps was present: `0.4.6`;
- Claude Code was present: `2.1.152`;
- OpenCode was present: `1.15.11`;
- Playwright was present: `1.58.2`;
- `psql`, `redis-cli`, `mailpit`, Node, Go, Python, and uv were present;
- Redis responded with `PONG`;
- Mailpit HTTP API responded;
- Postgres did not respond.
The Dune base image remains useful as a toolchain reference, but it is not the future `sbx` template because it lacks Docker/Compose and has runtime service issues.
## Experiment 4: Postgres runtime directory
Fresh Dune base template observation:
```text
/var/run/postgresql: missing
pg_isready: no response
```
Current service script:
```bash
exec "${pg_bin}" -D /var/lib/postgresql/data -k /var/run/postgresql
```
Manual workaround tested inside the sandbox:
```bash
sudo install -d -o agent -g agent -m 0755 /var/run/postgresql
/usr/lib/postgresql/15/bin/postgres -D /var/lib/postgresql/data -k /var/run/postgresql
```
Observed:
- Postgres started.
- `pg_isready -h 127.0.0.1 -p 5432` reported accepting connections.
- `psql -h 127.0.0.1 -d postgres -Atc 'select 1'` returned `1`.
Implication:
The Postgres issue is not a deep incompatibility. The immediate fix is to create `/var/run/postgresql` at runtime before launching Postgres, because image-build-time creation is not reliable under `sbx`.
Follow-up:
- update the future Dune `sbx` template's service startup path to create and chown runtime directories;
- make Postgres restart behavior testable;
- consider adding explicit service health checks because `s6-svstat` was not available in the current image.
## Experiment 5: logging and service status surfaces
Observed:
- `sbx ls --json` provides useful lifecycle state, including sandbox name, status, workspaces, and published ports.
- There is no obvious top-level `sbx logs` command in the CLI help output.
- The current Dune image does not include `s6-svstat`.
- Current service logs are not exposed through a clean Dune-friendly path.
- `/var/log/postgresql/postgresql-15-main.log` existed, but the manually launched Postgres path emitted to the chosen stdout/stderr file instead.
- s6 service run scripts exist under `/run/s6-rc:.../servicedirs/...`, but that is not a user-facing log API.
Implication:
For hard cut confidence, Dune needs a deliberate logging/status story for the `sbx` template:
- service health probes for Postgres, Redis, Mailpit;
- a consistent log directory or command for supervised services;
- a `dune logs` mapping that combines service logs and `sbx policy log` as appropriate;
- no dependency on Docker Compose logs or Pipelock logs.
## Experiment 6: network policy enforcement
Scoped deny rule tested:
```bash
sbx policy deny network --sandbox dune-sbx-spike2-shell example.com
```
Direct shell request:
```bash
curl -fsS --max-time 10 http://example.com
```
Observed:
- before deny: request succeeded;
- after deny: curl returned HTTP `403` and failed;
- after removing the deny: request succeeded again.
Nested Docker request:
```bash
docker run --rm busybox:1.36 wget -q -T 10 -O - http://example.com
```
Observed with the sandbox-scoped deny active:
- Docker image pull from Docker Hub was allowed;
- `wget` to `example.com` failed with `Network is unreachable`;
- `sbx policy log dune-sbx-spike2-shell --limit 10` showed the blocked request;
- the nested Docker request appeared with proxy mode `transparent`.
Representative policy log facts:
- direct shell blocked traffic appeared as `forward`;
- nested Docker blocked traffic appeared as `transparent`;
- allowed Docker registry traffic appeared as `forward-bypass`;
- deny rules took precedence over the global allow-all rule.
Implication:
This is strong evidence that `sbx` policy can replace Pipelock for the `sbx` runtime, including traffic from nested Docker containers. Dune should lean into `sbx policy log` as the network observability source rather than preserving Pipelock.
Production policy remains open:
- decide the Dune default policy profile;
- decide whether Dune writes sandbox-scoped rules or relies on a default profile;
- decide how users inspect and modify policy;
- test package-manager and agent-provider traffic under a non-open baseline.
## Experiment 7: fake custom secret behavior
Command used a fake value only:
```bash
sbx secret set-custom dune-sbx-spike2-shell \
  --host httpbin.org \
  --env DUNE_FAKE_API_KEY \
  --placeholder dune-fake-{rand} \
  --value DUNE_FAKE_SECRET_VALUE_FOR_SPIKE_ONLY
```
Help text states:
- custom secrets set an env var in the sandbox to a placeholder;
- outbound requests to the target host have the placeholder replaced with the real secret in request headers;
- the real secret should not enter the sandbox directly.
Observed:
- command saved a custom secret placeholder for target `httpbin.org`;
- CLI warned: "You may need to update environment variable DUNE_FAKE_API_KEY inside existing sandboxes";
- `sbx exec` in the already-running sandbox did not see `DUNE_FAKE_API_KEY`;
- after `sbx stop` and `sbx run`, `sbx exec` still did not see `DUNE_FAKE_API_KEY`;
- `sbx secret ls` showed the custom secret;
- `sbx secret ls --service custom` did not show it;
- `sbx secret ls --service httpbin.org` did not show it;
- `sbx secret rm` could not remove it using `custom`, `httpbin.org`, `DUNE_FAKE_API_KEY`, or likely combined identifiers;
- removing the sandbox did not remove the sandbox-scoped custom secret.
Current cleanup gap:
```text
sbx secret ls
```
continued to show the fake custom secret after sandbox removal:
```text
SCOPE                   TARGET        ENV
dune-sbx-spike2-shell   httpbin.org   DUNE_FAKE_API_KEY
```
The stored value is fake, but the lifecycle behavior matters.
Implication:
`sbx` custom secrets are promising but not mature enough to build Dune's first hard-cut release around without more testing or clarification. For now:
- do not depend on custom secrets for core Dune boot;
- use only fake secrets in further tests;
- verify built-in provider secrets separately from custom secrets;
- report or investigate custom-secret removal semantics before recommending Dune-managed secret lifecycle;
- avoid claiming "sandbox-scoped custom secrets are cleaned up with sandboxes" because this test showed they are not.
## Experiment 8: clone mode
Command:
```bash
sbx create --clone --name dune-sbx-spike2-clone shell /home/mitchell/Documents/Mycode/dune
```
Observed:
- `sbx` detected the Git repository.
- It created a host remote named `sandbox-dune-sbx-spike2-clone`.
- It exposed source at `/run/sandbox/source`.
- `pwd` was `/home/agent/workspace`.
- `/home/agent/workspace` was not a Git repository.
- `/run/sandbox/source` was a Git repository.
- `/run/sandbox/source` had both `origin` and the sandbox remote.
- The absolute host path `/home/mitchell/Documents/Mycode/dune` was also visible as a Git repository.
- Removing the sandbox removed the host sandbox remote.
Implication:
Clone mode is still not the immediate Dune default. It provides useful ingredients, but Dune would need to explicitly orchestrate a private checkout into the working directory and define writeback behavior. For the first hard-cut path, direct mount plus clear working directory handling is simpler.
## Workspace model recommendation
Given the observations, the first `sbx` Dune runtime should likely use direct mount and make the attached shell start in the real mounted repo path.
Recommended first shape:
```text
workspace host root: resolved by Dune
workspace sandbox path: same absolute path visible inside sbx
shell working dir: that absolute path
/workspace: optional compatibility symlink only if safe
```
Avoid relying on `/workspace` unless Dune creates it deliberately. The current Dune base image's `/workspace` convention is actively misleading under `sbx` because it exists but is not the mounted repo.
Clone mode can remain a later security/workflow improvement after direct-mount parity is complete.
## Template direction
The future Dune `sbx` template should combine:
- Docker-enabled behavior from Docker's `shell-docker` template;
- Dune's current batteries-included toolchain;
- runtime-safe service startup;
- explicit service health/logging conventions;
- direct-mount workspace behavior that does not pretend `/workspace` is the repo unless Dune makes it true.
Required template contents for parity:
- zsh and shell defaults;
- Rally;
- Laps;
- selected agent CLIs;
- Go, Node, Python, uv, pnpm, common CLI tools;
- Playwright and browser dependencies;
- Redis;
- Mailpit;
- Postgres with runtime directory setup;
- Docker Engine and Docker Compose inside the sandbox.
Unknown:
- whether Dune can directly derive from Docker's internal `shell-docker` template image. It was usable by `sbx`, but not inspectable as a normal host Docker image in this probe.
## Command mapping for hard-cut Dune
Suggested command mapping:
```text
dune up
  validate sbx installed, daemon healthy, authenticated, minimum version
  ensure Dune sbx template exists or pull/load it
  create sandbox if missing
  start sandbox if stopped
  attach shell with working dir set to mounted repo path

dune down
  sbx stop <instance>

dune destroy / dune rm, if added
  sbx rm <instance>

dune rebuild
  rebuild or refresh the Dune sbx template
  recreate sandbox when template changes require it
  no Dockerfile.dune path

dune logs
  show Dune service logs from inside the sandbox
  include or offer sbx policy logs

dune ports, if added
  wrap sbx ports list/publish/unpublish

dune doctor
  sbx diagnose
  template availability
  sandbox status
  service health
  policy baseline
```
Profile mapping recommendation:
```text
instance name = dune-<workspace-slug>-<profile>
```
This keeps the existing mental model while moving persistence from Docker volumes to sandbox lifecycle.
## What this means for the refactor
The older `ref-*` plans were written for an uncertain backend future. The new target should be more decisive.
Keep from the old direction:
- separate resolution/planning from execution where it improves clarity;
- avoid app-level code spelling every raw runtime command;
- add command construction tests around the runtime execution layer.
Drop or reduce:
- dual-backend architecture as a long-lived goal;
- preserving `Dockerfile.dune`;
- designing abstractions around remote Docker or generic MicroVM targets;
- Pipelock as a runtime abstraction for the future path.
New refactor shape:
```text
app.go
  -> resolve workspace/profile/config
  -> build sbx-oriented environment intent
  -> execute through sbx runtime package

internal/dune/runtime/sbx
  -> validates sbx
  -> maps Dune instance names to sandboxes
  -> manages create/run/stop/rm/exec
  -> manages ports and policy-log access
  -> invokes service health checks inside sandbox
```
The old Docker Compose code can remain only until the `sbx` path is ready to cut over.
## Remaining gaps before hard cut
Highest confidence gaps to close:
1. Build a dedicated Dune `sbx` template with Docker/Compose and Dune's toolchain in one image.
2. Fix Postgres startup by creating runtime directories during service startup.
3. Add service health checks for Postgres, Redis, and Mailpit.
4. Define service log paths and `dune logs` behavior.
5. Verify Playwright can launch Chromium under the final template.
6. Verify nested Docker Compose projects can build images, create volumes, and publish ports.
7. Verify common package-manager traffic under a non-open policy baseline.
8. Decide the default Dune `sbx` network policy and how users inspect/modify it.
9. Clarify or fix custom-secret lifecycle, especially removal of custom secrets.
10. Decide whether first release uses direct mount only, with clone mode deferred.
11. Decide whether `/workspace` is a compatibility symlink, a deprecated convention, or removed from the mental model.
12. Define template rebuild/update behavior now that `Dockerfile.dune` is dropped.
## Recommended next spike
Build a dedicated minimal Dune `sbx` template prototype.
Suggested success criteria:
- sandbox starts from the Dune `sbx` template;
- attached shell starts in the mounted repository;
- Docker Engine works;
- `docker compose version` works;
- nested `docker compose up` works;
- Postgres, Redis, and Mailpit are healthy after fresh sandbox start;
- Playwright can run a minimal Chromium launch;
- `sbx ports` can expose a nested service to host;
- `sbx policy deny network --sandbox <name> example.com` blocks both shell and nested Docker traffic;
- `sbx policy log <name>` shows useful blocked and allowed records;
- fake custom-secret behavior is either understood and removable or explicitly excluded from first release scope.
## Commands run during this spike
Representative commands:
```bash
sbx version
sbx diagnose --output json
sbx policy ls
sbx secret set-custom --help
sbx policy deny network --help
sbx policy rm network --help
sbx create --name dune-sbx-spike2-shell shell /home/mitchell/Documents/Mycode/dune
sbx exec dune-sbx-spike2-shell bash -lc 'docker version'
sbx exec dune-sbx-spike2-shell bash -lc 'docker compose version'
sbx exec dune-sbx-spike2-shell bash -lc 'docker compose up -d'
sbx ports dune-sbx-spike2-shell --publish 18081 --json
sbx ports dune-sbx-spike2-shell --publish 18082 --json
sbx create --name dune-sbx-spike2-base --template ghcr.io/mitchell-wallace/dune-base:0.4.2 shell /home/mitchell/Documents/Mycode/dune
sbx exec dune-sbx-spike2-base bash -lc 'pg_isready -h 127.0.0.1 -p 5432'
sbx exec dune-sbx-spike2-base bash -lc 'sudo install -d -o agent -g agent -m 0755 /var/run/postgresql'
sbx policy deny network --sandbox dune-sbx-spike2-shell example.com
sbx policy log dune-sbx-spike2-shell --limit 10
sbx policy rm network --sandbox dune-sbx-spike2-shell --resource example.com
sbx secret set-custom dune-sbx-spike2-shell --host httpbin.org --env DUNE_FAKE_API_KEY --placeholder dune-fake-{rand} --value <fake value>
sbx secret ls
sbx create --clone --name dune-sbx-spike2-clone shell /home/mitchell/Documents/Mycode/dune
sbx rm dune-sbx-spike2-base
sbx rm dune-sbx-spike2-clone
sbx rm dune-sbx-spike2-shell
```
Cleanup status:
- all probe sandboxes were removed;
- clone-mode host git remote was removed;
- the fake custom secret remained listable after sandbox removal and could not be removed through tested `sbx secret rm` forms.
