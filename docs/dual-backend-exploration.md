# Dune Dual-Backend Exploration

- Status: active working document
- Owner: GPT-5.6-sol
- Started: 2026-07-11
- Decision horizon: after the post-migration host experiments
- Branches examined: `main` (classic Docker, read through `origin/main`) and `dunex` (sbx integration, current checkout)

## Purpose and maintenance contract

This is the durable research record for Dune's container-backend strategy. Future sessions should extend it rather than create parallel notes. Add dated findings to the research log, preserve failed experiments, and update the decision record only when new evidence changes a conclusion.

The immediate question is no longer whether to replace classic Dune with sbx. It is whether Dune should support two backends while giving an agent the same inner development contract on both:

```text
                          Dune environment contract
                workspace + profile + tools + docker compose
                                      |
                       +--------------+--------------+
                       |                             |
                classic Docker                    sbx microVM
          agent + nested-Docker sibling      dedicated kernel + dockerd
                       |                             |
               host Docker/kernel                 sbx daemon/KVM
```

This document separates three kinds of parity:

1. **Workflow parity:** the same project can use `docker compose` from the agent shell.
2. **Lifecycle parity:** start, stop, rebuild, persistence, ports, logs, and diagnostics behave predictably.
3. **Security parity:** the same isolation, egress enforcement, and host-managed secret boundary.

Workflow parity is realistic. Lifecycle parity is achievable behind a backend abstraction. Security parity is not: a privileged DinD container shares the host kernel, while sbx gives the sandbox a microVM kernel and an outside-the-guest policy plane. Dune must document that difference rather than flatten it into a single “sandboxed” claim.

## Executive recommendation

Adopt a **dual-backend direction** and stop treating the sbx line as a hard cutover. Keep classic Docker available as the low-friction backend, retain sbx as the stronger-isolation backend, and converge their inner project-service model on repo-owned Compose.

Do not ship privileged DinD in the interactive agent container. The first classic implementation candidate should be a **dedicated DinD sibling** controlled by an otherwise unprivileged agent container. It should be opt-in until its security wording, nested egress, port publication, storage cleanup, and 4 GB memory behavior pass the post-migration gates.

The recommended topology is:

```text
host Docker daemon
  |
  `-- Dune outer Compose project
      |-- agent
      |    - unprivileged outer container
      |    - Docker CLI and Compose plugin
      |    - DOCKER_HOST -> private DinD endpoint
      |    - workspace + profile persistence
      |
      |-- dind
      |    - dedicated nested daemon
      |    - privileged outer container (explicit risk boundary)
      |    - same workspace mounted at the same absolute inner path
      |    - workspace/profile-specific Docker data volume
      |
      `-- Pipelock / future credential broker
           - only component with an outer egress path
```

This is preferable to the alternatives for the initial spike:

| Option | Result | Position |
|---|---|---|
| Host Docker socket in the agent (“Docker-outside-of-Docker”) | Fast and storage-efficient, but grants effective host-root control, leaks host container namespace, and makes bind paths resolve on the host | Reject |
| Rootful `dockerd` inside the agent | Straightforward, but makes the interactive agent container privileged and expands compromise impact | Reject as the product topology |
| Dedicated rootful DinD sibling | Keeps direct privilege out of the agent and uses Docker's conventional DinD shape; still shares the host kernel | First spike candidate |
| Rootless DinD | Inner daemon runs as a non-root user, but Docker's supported rootless-in-rootful-Docker recipe still requires `--privileged`; cgroup limits require cgroup v2 and systemd delegation | Comparative spike, not assumed safer enough |
| Sysbox runtime for classic containers | Can support system-container nesting without ordinary `--privileged`, but adds a Linux host runtime/daemon dependency and its own compatibility surface | Optional hardening experiment, not the default prerequisite |
| sbx only | Stronger isolation and external policy plane, but current VM creation failed and startup/setup are higher-friction | Retain as a backend; do not hard-cut yet |

The dual-backend decision should be conditional, not sentimental: if the dedicated DinD sibling cannot enforce an acceptable network boundary, cannot expose ports coherently, or causes repeated memory/OOM failures on the 4 GB target, classic Dune should not promise general nested Compose. It may remain a lighter backend with an explicit capability difference while sbx is repaired.

## Evidence from this repository

### Classic Dune on `main`

The classic implementation is concentrated in `internal/dune/app.go` and `internal/dune/compose.yaml.tmpl` on `origin/main`:

- The host CLI requires a reachable host Docker daemon and Compose plugin.
- It renders a two-service outer Compose project: `agent` plus `pipelock`.
- The workspace is bind-mounted to `/workspace`.
- Profile state is a host-Docker named volume, `dune-persist-<profile>`, mounted at `/persist/agent` and wired into the agent home by `setup-persist.sh`.
- A repo-level `Dockerfile.dune` selects a custom outer agent image.
- The agent is attached with `docker compose exec ... agent zsh`.
- Postgres, Redis, and Mailpit are installed in the base image and started under s6. They are environment-owned services, not project-owned Compose services.
- Pipelock is on both an internal and external outer network. The agent is only on the internal network and receives HTTP(S) proxy variables.
- There is currently no Docker CLI/daemon contract inside the agent.

This explains the present project divergence: an application may use `localhost` services in classic Dune but a repo Compose stack in dunex.

### Dunex/sbx on the current branch

The current `dunex` branch is not a stub. `internal/dune/runtime/sbx` implements validation, create/start/attach/stop/remove, status, ports, logs, doctor, policy rules, and a forward-looking service-secret wrapper:

- `Validate` requires `sbx` on `PATH`, every `sbx diagnose --output json` check to pass, and version `v0.32.0` or newer.
- `Ensure` creates `dune-<workspace>-<profile>` from the Dune sbx template, passing the workspace and profile-persist host directory as positional mounts.
- The sbx template inherits Docker Engine, Compose, and the `com.docker.sandboxes.start-docker=true` label. The sbx runtime starts its nested daemon.
- The workspace and persist directory are virtiofs mounts at their absolute host paths. `/workspace` is a compatibility symlink refreshed by a login hook.
- The template uses `tini`, not s6 or systemd. Postgres/Redis/Mailpit servers are deliberately absent; projects own those services through Compose.
- The sbx policy layer observes both shell (`forward`) and nested-container (`transparent`) egress.
- `sbx secret` support exists in the backend but no core Dune boot path currently sets a service secret. Agent CLI credentials still live in profile persistence under `/persist/agent`.

The distinction in the last point matters: “sbx keeps secrets outside the container” is true for sbx-managed service/registry secrets and the control plane, but not yet for all Dune credentials. Current agent-provider credentials are guest-readable persisted files in both backends. The untracked `sbx-7-host-credential-injection` draft is investigating that remaining gap and is not modified by this work.

### Environment observation on 2026-07-11

Read-only checks inside the current classic Dune container found:

- 3.8 GiB total RAM, approximately 2.4 GiB available at observation time;
- 2.0 GiB swap, with approximately 512 MiB already in use;
- unified cgroup v2 (`0::/`; controllers include CPU, memory, I/O, and PIDs);
- no `/dev/kvm` in the container and user `agent` only in group `agent`;
- Linux `6.17.0-1018-azure`.

These are observations from inside the container, not proof of host `/dev/kvm` state or host group membership. They justify host-side collection after breakout and rule out trying sbx from this container today.

## What classic DinD requires

### Outer topology and daemon lifecycle

The agent image needs the Docker CLI and Compose plugin, but the daemon should live in a dedicated sibling. The outer Compose project needs:

- a pinned DinD image/engine version compatible with the CLI;
- a private daemon endpoint (prefer mutually authenticated TLS or a Unix socket volume over unauthenticated TCP);
- a daemon health check and an agent dependency on daemon readiness;
- an explicit lifecycle supervisor and bounded startup timeout;
- daemon logs available through `dune logs docker` or the eventual backend-neutral equivalent;
- a unique Docker data volume per workspace **and** profile, not the profile-wide credential volume;
- the resolved workspace mounted into the DinD sibling at the same path the inner Compose client uses;
- explicit resource limits at the outer container boundary.

Do not put `/var/lib/docker` in the outer container writable layer. `dune down` currently runs outer `compose down`, which removes containers; without a separate volume every down would discard inner images, build cache, volumes, and service data. Do not share one daemon data directory across simultaneously running Dune workspaces. The initial naming candidate is `dune-docker-<workspace-slug>-<profile>`.

The volume must also be measured for disk and inode growth. Nested image stores duplicate layers already present in host Docker and require an explicit prune/retention story. Docker recommends checking the chosen storage driver and backing filesystem with `docker info`; write-heavy data belongs in volumes rather than container writable layers. See [Docker storage-driver selection](https://docs.docker.com/engine/storage/drivers/select-storage-driver/) and [OverlayFS behavior and limitations](https://docs.docker.com/engine/storage/drivers/overlayfs-driver/).

### Privilege model

Conventional rootful DinD requires a privileged outer daemon container. Docker documents that `--privileged` grants all capabilities, access to all devices, and relaxes LSM restrictions; it should be treated as a host-kernel attack surface, not a cosmetic flag. See [Docker runtime privilege](https://docs.docker.com/engine/containers/run/#runtime-privilege-and-linux-capabilities).

A sibling narrows direct exposure: the agent receives control of the nested Docker API but is not itself an outer privileged process. It does **not** make the design equivalent to a VM. A malicious or compromised inner workload still attacks a privileged daemon/container sharing the host kernel.

Rootless DinD is worth measuring for UID behavior and daemon containment, but it does not remove the outer privileged requirement in Docker's documented nested recipe. Docker states that the `docker:<version>-dind-rootless` container still needs `--privileged` to disable seccomp, AppArmor, and mount masks. See [Rootless Docker-in-Docker](https://docs.docker.com/engine/security/rootless/tips/#rootless-docker-in-docker).

Sysbox changes the trust tradeoff by providing “system containers” intended for nested Docker without an ordinary privileged container. It also changes the installation promise: the host must install and maintain an alternate OCI runtime and its services, and filesystem/kernel/runtime compatibility becomes part of Dune support. It should be tested only after the stock-Docker baseline, and accepted only if its operational reliability is clearly better than privileged DinD on Dune's supported hosts.

### Cgroup v2 and resource control

The observed kernel exposes unified cgroup v2, which is the necessary modern substrate, but the decisive questions are delegation and what the **host Docker daemon** gives the DinD outer container.

For rootful DinD, set hard memory/PID/CPU limits on the outer DinD sibling and verify that inner limits neither escape nor silently fail. The outer limit is the safety boundary; inner Compose limits are convenience/fairness controls.

For rootless DinD, Docker documents that `--memory`, `--cpus`, and `--pids-limit` are supported only with cgroup v2 and systemd, and that only some controllers are commonly delegated. The current Dune image uses s6 and the sbx template uses tini, so rootless resource enforcement must be demonstrated, not inferred. See [Rootless resource limits](https://docs.docker.com/engine/security/rootless/tips/#limiting-resources).

Collect inside each candidate daemon:

```sh
docker info --format '{{json .}}'
docker info | sed -n '/Storage Driver/,+8p; /Cgroup/,+4p; /Rootless/,+2p'
cat /proc/self/cgroup
cat /sys/fs/cgroup/cgroup.controllers
cat /sys/fs/cgroup/cgroup.subtree_control 2>/dev/null || true
```

Then run an inner container with memory/PID limits and confirm the effective cgroup files, not just a successful CLI exit.

### Workspace mounts and UID behavior

An inner daemon resolves bind sources in the DinD sibling's mount namespace. Therefore the sibling must see the workspace at the same absolute path used by the agent/Compose client. This is straightforward when the outer CLI mounts the workspace into both siblings; it is not safe to assume `/workspace` is always the only path because dunex deliberately preserves the real host path and uses `/workspace` as compatibility.

Test all of the following:

- source edits are visible host → agent → inner container and back;
- bind mounts of files and directories work from a nested Compose file;
- generated files have acceptable host ownership under rootful and rootless candidates;
- Git worktrees, symlinks, spaces in paths, and repositories outside `$HOME` work;
- file watching and Playwright/browser workloads do not regress materially;
- no inner workload can bind arbitrary host paths that were not mounted into the DinD sibling.

### Networking and ports

Nested Compose does not automatically make an inner published port reachable on the physical host. `-p 3000:3000` binds in the DinD sibling's network namespace; the outer Dune project still needs a host-to-sibling publication for that port.

Candidate solutions, in preference order for testing:

1. A `dune ports` capability that records requested host→environment ports and recreates/updates the outer topology when required.
2. A documented, bounded outer port range forwarded to the DinD sibling, with collision handling.
3. A host-side forwarding helper managed by Dune.

Avoid `network_mode: host` as the default: it is Linux-specific, weakens network isolation, creates port collisions, and conflicts with the existing Pipelock network model.

For egress, simply setting proxy variables in the agent is insufficient. Inner Compose services do not reliably inherit them, and malicious workloads can unset them. The promising classic topology is:

- agent and DinD siblings attach only to an outer `internal: true` network;
- Pipelock/credential broker is the only service attached to both internal and external networks;
- dockerd pull/build proxy settings point at Pipelock;
- Docker client default proxy settings seed ordinary inner containers;
- tests prove an inner container has no direct route when proxy variables are absent;
- tests prove allowed and blocked requests appear in the policy log.

The privileged DinD sibling can manipulate its own network namespace, so this still requires an adversarial test. If it can bypass the outer internal-network boundary, classic cannot claim enforced egress parity. sbx already demonstrated policy observation for both shell and nested Docker traffic in spike 4.

### Secrets and authentication

Separate the secret problem into three categories:

| Category | Classic today | sbx/dunex today | Dual-backend target |
|---|---|---|---|
| Agent CLI login state (Codex, Claude, gh, etc.) | Files in profile volume, readable by agent | Files in host-backed profile persistence, readable in guest | Preserve UX initially; investigate host credential broker/injection in the sbx-7 line |
| Project runtime secrets | Repo/application convention | Repo/application convention; experimental custom sbx secrets are not lifecycle-managed | Compose secrets or project-native secret manager, with explicit per-service grants |
| Infrastructure/service credentials and registry auth | Host Docker/Pipelock configuration | sbx service/registry secrets outside guest control plane | Backend-owned broker/injection where possible; never bake into images or generated files |

Compose secrets are better than environment variables because they are mounted only into explicitly granted services, but they are not equivalent to an outside-the-guest secret broker: the secret source exists on the outer host/filesystem and a controlling agent can often reconfigure its own nested stack. See [Docker Compose secrets](https://docs.docker.com/compose/how-tos/use-secrets/).

Never expose the host Docker socket as a credential convenience. Docker warns that access to the daemon API can grant root access on the host; see the [dockerd reference](https://docs.docker.com/reference/cli/dockerd/).

## Backend mismatch inventory

This inventory records what an agent or operator can observe. “Align” means Dune should provide a shared contract; “document” means the difference is inherent or too costly to hide.

| Surface | Classic Dune today | Dunex/sbx today | Direction |
|---|---|---|---|
| Host prerequisite | Docker Engine + Compose | sbx CLI, login, daemon, KVM/nested virtualization, supported version | Document; doctor per backend |
| Environment creation | Outer two-container Compose project | sbx microVM from template | Document |
| Isolation boundary | Containers share host kernel | Dedicated microVM kernel | Document prominently |
| Inner Docker | Absent | Auto-started daemon + Compose | Align via dedicated DinD sibling if gates pass |
| Project dependencies | Image-owned s6 Postgres/Redis/Mailpit | Project-owned Compose | Align on project-owned Compose; remove/disable bundled servers in the parity image |
| PID 1 / service manager | s6-overlay | tini; sbx runtime starts Docker | Do not promise systemd; provide backend-owned daemon readiness/logging |
| `systemctl` / user services | Absent | Absent in current template | Align documentation; future thenn design must not assume systemd |
| Workspace path | Bind at `/workspace` | Real absolute host path plus `/workspace` symlink | Expose stable `$DUNE_WORKSPACE`; keep `/workspace` compatibility |
| Additional mounts | Outer Compose-controlled | Extra sbx positional workspaces, virtiofs | Backend capability/config, not identical flags |
| Profile persistence | Docker named volume shared by profile | Host directory shared by profile and mounted into VM | Align semantics; provide migration/export, not storage identity |
| Inner Docker persistence | N/A | Sandbox-local until sandbox removal | Classic needs workspace/profile daemon volume; document different rebuild behavior or normalize it |
| Image customization | `Dockerfile.dune` builds outer image | Dune sbx template; kits planned, experimental | Define backend-neutral “environment extension” capability or clearly backend-specific config |
| Daemon startup | Host Docker assumed running | sbx daemon plus sandbox dockerd startup | Auto-start sbx daemon; health/readiness state machine on both |
| Stop/down | Removes classic outer containers | Stops and retains sandbox | Normalize user meaning or add explicit `destroy`; current semantics differ materially |
| Rebuild | Rebuilds outer image; profile volume survives | Removes/recreates sandbox/template; profile dir survives | Align outcome and name backend-specific work discarded |
| Ports | Ordinary outer Compose publication (currently no generic command) | `sbx ports` maps host→sandbox interface | Add capability-based `dune ports`; nested classic needs a second forwarding layer |
| DNS/service discovery | Outer Compose names; bundled services on localhost | Inner Compose names; services in nested daemon | Align on repo Compose; document host access aliases |
| Egress enforcement | Pipelock via agent proxy env; no nested workloads today | sbx policy covers shell + nested Docker | Classic nested enforcement is a release gate, not assumed parity |
| Egress logs | `dune logs pipelock` stream | `dunex logs` includes `sbx policy log` records | Normalize a policy-log command/schema where feasible |
| SSH/non-HTTP egress | Generally unavailable through HTTP proxy | Governed/limited by sbx policy capabilities | Capability report and docs |
| Agent credentials | Guest-readable profile files | Guest-readable profile files | Same current limitation; continue sbx-7 research |
| Service/registry secrets | Host/Pipelock/Docker conventions | sbx control-plane API available | Backend capability; do not claim equivalence |
| Host Docker visibility | Outer containers visible in host `docker ps` | Inner containers absent from host `docker ps` | Document |
| Inner Docker visibility | Proposed inner containers visible only to nested daemon | Same | Align if DinD lands |
| Logs | Outer Compose services | Dune logs + sbx policy; project logs via inner Compose | Define environment logs vs project logs consistently |
| Resource limits | Host Docker cgroups | sbx VM allocation plus inner Docker | Surface effective limits; backend-specific enforcement |
| Memory overhead | Outer containers + proposed dockerd/cache | VM memory + daemon/cache | Measure; no guessed comparison |
| Disk/cache | Host image store; proposed duplicate nested store | sbx template and private daemon store | Measure cold/warm size, prune behavior, rebuild loss |
| Startup latency | Host Docker/outer Compose; likely lower warm path | daemon + microVM + template; observed friction | Measure p50/p95 cold/warm |
| File I/O/watchers | Native Docker bind | virtiofs mount | Measure representative repos |
| Architecture/platform | Docker-supported hosts | Linux requirements include KVM; nested virtualization in VMs | Document support matrix |
| Failure diagnostics | Basic prerequisite errors and recent Compose logs | Stable diagnostic codes + doctor | Port diagnostic model to shared frontend/backend checks |
| Offline behavior | Local host images may suffice | Separate sbx image store; local template requires load | Document/cache test |
| Timezone/hostname/kernel | Container shares host kernel; TZ injected | Guest kernel/hostname; TZ injected | Document only if tools observe it |
| Privileged workloads inside project | Depends on outer design; high risk in DinD | Privileged inside VM remains VM-contained | Explicit capability and warning |
| GPU/USB/KVM devices | Docker device passthrough possible | sbx support-dependent | Capability matrix, no false parity |

## Concrete dunex fix: start the sbx daemon

Current `Validate` calls `sbx diagnose --output json` and fails on any non-passing check. It does not explicitly start the daemon. Even though current Docker documentation says the daemon normally auto-starts on the first command, the reported VM required an explicit detached start in practice.

Add a bounded, idempotent daemon-start step before final readiness validation:

```sh
sbx daemon start > /dev/null 2>&1 &!
```

The implementation should not blindly hide failures:

1. Check whether the daemon is already healthy.
2. If the failure is specifically daemon-unavailable, launch it detached once.
3. Poll readiness with a short bounded backoff.
4. Re-run structured `sbx diagnose --output json`.
5. Preserve daemon-start/diagnose stderr and command details under `--verbose` and in lifecycle logs.
6. Do not retry authentication, KVM, version, storage, or template failures as if they were daemon startup races.

Add unit tests for already-running, successful start, start timeout, and non-daemon diagnose failure. Add a host smoke test that kills/stops only the sbx daemon, invokes `dunex up`, and confirms auto-recovery without disturbing existing sandboxes. Reconfirm `sbx daemon start --help` on the post-migration installed version before pinning arguments.

## `sbx.create_failed` post-breakout runbook

The reported error is only Dune's stable wrapper:

```text
sbx.create_failed: create sbx sandbox dune-group-1-01-default failed
```

The underlying stderr is required. Do not reset sbx state or recreate the production sandbox before collecting it.

### 1. Capture identity, version, virtualization, and storage

Run on the **host**, not inside Dune:

```sh
date -u
uname -a
cat /etc/os-release
id
groups
command -v sbx
sbx version
sbx daemon --help
sbx create --help
systemd-detect-virt || true
lscpu | sed -n '/Architecture/p;/Virtualization/p;/Hypervisor/p'
lsmod | grep -E '^kvm(_intel|_amd)?' || true
ls -l /dev/kvm
stat -c '%A %a %U:%G %t:%T %n' /dev/kvm
getfacl /dev/kvm 2>/dev/null || true
test -r /dev/kvm && echo kvm-readable || echo kvm-not-readable
test -w /dev/kvm && echo kvm-writable || echo kvm-not-writable
df -hT "$HOME" "${XDG_STATE_HOME:-$HOME/.local/state}" "${XDG_CACHE_HOME:-$HOME/.cache}"
df -i "$HOME" "${XDG_STATE_HOME:-$HOME/.local/state}" "${XDG_CACHE_HOME:-$HOME/.cache}"
```

On Ubuntu, also run `kvm-ok` if installed. If the host is itself a VM, verify that the provider exposes nested virtualization. Docker's current Linux requirements say sbx cannot start without KVM and that the user must be in the `kvm` group; group changes require a new login or `newgrp kvm`. See [Docker Sandboxes prerequisites](https://docs.docker.com/ai/sandboxes/get-started/#prerequisites).

### 2. Capture diagnostics before and after explicit daemon start

```sh
sbx diagnose
sbx diagnose --output json | tee /tmp/sbx-diagnose-before.json
sbx diagnose --output github-issue > /tmp/sbx-diagnose-before.md
ps -ef | grep -E '[s]bx|[s]andboxd'

sbx daemon start > /tmp/sbx-daemon-start.out 2> /tmp/sbx-daemon-start.err &
daemon_pid=$!
printf 'daemon pid=%s\n' "$daemon_pid"

sbx diagnose --output json | tee /tmp/sbx-diagnose-after.json
sbx ls --json | tee /tmp/sbx-ls.json
```

If `sbx diagnose` recommends a login, check status and authenticate normally; never place a token in the runbook output. Compare CLI and daemon versions for drift.

### 3. Collect daemon and kernel evidence

```sh
journalctl --user --since '-30 min' --no-pager | grep -iE 'sbx|sandbox|kvm' > /tmp/sbx-user-journal.txt || true
sudo journalctl -k --since '-30 min' --no-pager | grep -iE 'kvm|oom|out of memory' > /tmp/sbx-kernel-journal.txt || true
find "${XDG_STATE_HOME:-$HOME/.local/state}/sandboxes" -maxdepth 3 -type f -printf '%TY-%Tm-%Td %TT %p\n' 2>/dev/null | sort > /tmp/sbx-state-files.txt
find "${XDG_CACHE_HOME:-$HOME/.cache}/sandboxes" -maxdepth 3 -type f -printf '%TY-%Tm-%Td %TT %p\n' 2>/dev/null | sort > /tmp/sbx-cache-files.txt
```

Inspect the newest files listed rather than assuming a log filename. Redact tokens, registry credentials, workspace names if sensitive, and user paths before sharing. `sbx diagnose --upload` can create a support bundle/diagnostic ID, but upload only with Mitchell's explicit approval because it sends host diagnostics externally. Docker's troubleshooting guide recommends diagnose first and warns that reset deletes sandbox state: [Docker Sandboxes troubleshooting](https://docs.docker.com/ai/sandboxes/troubleshooting/).

### 4. Reproduce once with full Dune detail

```sh
dunex doctor --json --verbose | tee /tmp/dunex-doctor.json
dunex up --verbose 2>&1 | tee /tmp/dunex-up-verbose.txt
```

Extract the exact `sbx create ...` command and raw stderr from verbose output. Before running that command directly, replace the name with a unique disposable diagnostic name and use an empty temporary workspace so it cannot collide with the existing instance. Record `free -h`, `vmstat 1`, and kernel OOM messages during the attempt.

Classify the result in this order:

1. daemon not running or CLI/daemon version mismatch;
2. authentication/session failure;
3. missing KVM module/device, wrong `/dev/kvm` ownership/mode, missing `kvm` group, or nested virtualization disabled;
4. insufficient memory/swap or OOM kill;
5. disk/inode exhaustion or corrupt/unwritable XDG state;
6. template pull/auth/reference failure (may map to `template.unavailable` if output matches);
7. stale name/state or sbx database problem;
8. reproducible sbx defect—generate a redacted support bundle and file upstream.

Do **not** run `sbx reset` or delete `~/.local/{state,share,cache}/sandboxes` until evidence is saved and Mitchell approves destructive recovery.

## Phased plan

### Phase 0 — Research and decision framing (this session)

- [x] Inspect classic `origin/main` lifecycle, Compose topology, persistence, bundled services, and proxy model.
- [x] Inspect the landed dunex sbx backend, template, lifecycle, persistence, egress, secrets, and diagnostics.
- [x] Record the mismatch inventory and candidate DinD architectures.
- [x] Prepare the host failure runbook and post-migration experiment protocol.
- [ ] Have the Claude crew chief and Mitchell review the recommendation/open questions.

No Docker/sbx fleet mutations or heavy builds belong in this phase.

### Phase 1 — Preserve both backends in the architecture

Reframe the existing `ref-1`/`ref-2` environment-boundary work around real dual-backend needs before implementation:

- define one resolved environment spec plus backend capabilities;
- restore/extract classic Docker behind a backend interface rather than merging two command graphs into `app.go`;
- keep backend-specific operations (sbx policy/secrets, outer Compose rendering) behind capability checks;
- normalize lifecycle vocabulary (`down`, `destroy`, `rebuild`) and state-loss descriptions;
- add backend selection precedence (CLI flag, workspace config, user default) only after Mitchell answers the open questions;
- retain stable diagnostic codes with backend-specific detail.

Do not revive the planned `sbx-6` destruction of classic assets while the dual-backend direction is active. Supersede or rewrite the hard-cut cleanup artifacts before implementation.

### Phase 2 — Post-migration low-impact host characterization

Run the sbx failure runbook first. Record host Docker/sbx versions, cgroup delegation, backing filesystem, Docker storage driver, KVM state, baseline memory, and current fleet resource use. Repair only the sbx daemon auto-start path and proven host prerequisites; avoid broad resets.

Exit criteria:

- `sbx.create_failed` has a root cause or a complete upstream-quality evidence bundle;
- the host has a trustworthy idle-memory baseline;
- no experiment overlaps another heavy test run.

### Phase 3 — Ephemeral DinD candidate spikes

Use disposable outer Compose projects and unique volumes. Do not modify the running Dune fleet.

Test sequentially:

1. dedicated rootful DinD sibling on stock Docker;
2. rootless DinD sibling on stock Docker;
3. Sysbox only if the first two leave a security/reliability gap worth the host dependency.

For each candidate test:

- daemon cold/warm startup and health recovery;
- `docker compose up/down/build/logs/exec` from the agent;
- workspace bind mounts, ownership, file watches, Git worktrees;
- project Postgres/Redis/Mailpit stack readiness and persistence;
- host→inner port reachability and collision handling;
- inner egress allow/deny with proxy variables present and deliberately absent;
- nested privileged-container behavior;
- outer stop/recreate and daemon-data survival;
- disk/inode growth and prune behavior;
- cgroup memory/PID enforcement;
- concurrent lightweight agent + service use, only after single-candidate stability.

### Phase 4 — 4 GB memory and performance protocol

Measure; do not estimate. Run one backend/candidate at a time, on an otherwise quiet host, with the same workspace and pinned images. Separate cold-cache and warm-cache runs and do at least three repetitions before quoting a typical value.

Before each run:

```sh
date -u
free -b
swapon --show --bytes
vmstat 1
docker stats --no-stream
cat /proc/pressure/memory
```

Keep `vmstat 1` and pressure sampling in a background logger during the scenario. Record at least:

- `MemAvailable`, swap used, `si`/`so`, run queue, and I/O wait;
- per-outer-container current/peak memory and PIDs from cgroup files or `docker stats`;
- sbx/VM process RSS/PSS on the host;
- `memory.events` (`oom`, `oom_kill`, `high`, `max`) before and after;
- kernel OOM logs;
- elapsed time for environment create/start/attach, Compose up, service-ready, representative build/test, stop, and warm restart;
- disk bytes and inodes for outer and nested Docker stores.

Suggested scenarios:

1. host idle baseline for five minutes;
2. environment shell idle for five minutes;
3. nested daemon idle;
4. Postgres + Redis + Mailpit Compose idle;
5. representative application boot;
6. representative build/test;
7. warm stop/start and rebuild;
8. two environments only if every single-environment gate passes.

Safety gates for this 4 GB host:

- abort a workload if `MemAvailable` remains below 512 MiB for 30 seconds;
- abort on any new `oom_kill`, repeated allocation stalls, or sustained swap-in/swap-out with loss of interactivity;
- do not start a second environment while swap is actively thrashing;
- cool down to a stable baseline before the next candidate;
- keep raw time series with the result so a peak number is not divorced from pressure behavior.

The 512 MiB threshold is a conservative initial guardrail, not a product requirement; Mitchell should confirm whether a different operational reserve is needed.

### Phase 5 — Decision gate and implementation proposal

Choose the classic inner-Docker mode only after evidence. The minimum ship gates are:

- no host Docker socket in the agent;
- no direct outer privilege on the interactive agent;
- reliable cold/warm daemon startup and actionable diagnostics;
- project Compose parity for the representative stack;
- coherent host port workflow;
- no direct nested egress bypass under the documented security mode;
- profile and project data survive the documented lifecycle;
- no OOM kills or unacceptable sustained pressure in the agreed 4 GB scenario;
- explicit backend isolation/security wording;
- cleanup, versioning, and upgrade behavior defined.

Then create/update OpenSpec changes for the environment boundary, classic DinD backend, shared lifecycle/capabilities, and sbx daemon recovery. Implementation and substantial testing begin only after the environment migration and review.

## Batched questions for Mitchell

### Product contract

1. Should classic Docker remain the default backend, should existing users keep their current default while new installs choose, or should backend selection be per-project only?
2. Is nested Compose a mandatory capability for both backends, or may classic remain a documented lighter fallback if the security/memory gates fail?
3. Is a trusted-local-development warning sufficient for a privileged DinD sibling, or is sharing the host kernel with any privileged outer container a release blocker?
4. Which host platforms must classic nested Docker support initially: Linux Engine only, or Docker Desktop on macOS/Windows too?

### Lifecycle and compatibility

5. Should `down` retain inner Docker service data/cache on both backends, and should `destroy` remove it while retaining profile credentials?
6. Must existing `Dockerfile.dune` customization remain supported, or can dual-backend projects move to a new backend-neutral extension/config model?
7. Do existing bundled Postgres/Redis/Mailpit services need a compatibility window, or can the parity release require each repository to add Compose definitions immediately?
8. What is the expected host-port UX: explicit `dune ports`, a configured range, or automatic discovery from project Compose?

### Security and credentials

9. Is Pipelock's HTTP-only model still the desired classic policy plane, and must nested non-HTTP egress be blocked by default?
10. Should the sbx-7 host-credential work become a shared credential-broker design for both backends, or is guest-readable profile auth acceptable on classic?
11. May diagnostics bundles be uploaded to Docker support when a failure recurs, after local redaction review?

### Resource policy and rollout

12. What memory reserve should Dune protect on the 4 GB VM? This document proposes aborting below 512 MiB available for 30 seconds as an initial safety gate.
13. What representative repository/test stack should define acceptance beyond Postgres/Redis/Mailpit?
14. Should sbx and classic be allowed to run concurrently on this VM, or should Dune enforce/warn about a one-heavy-environment-at-a-time policy?

## Decision record

| Date | Decision | Confidence | Evidence needed to change it |
|---|---|---|---|
| 2026-07-11 | Explore dual backends; do not proceed with an sbx hard cut | Medium-high | sbx reliability and friction become clearly superior with no material classic use case |
| 2026-07-11 | Align on project-owned Compose rather than bundled system services | High | a representative project cannot operate reliably with Compose on one backend |
| 2026-07-11 | Do not mount the host Docker socket into the agent | High | none anticipated; it violates the host isolation objective |
| 2026-07-11 | Spike a dedicated DinD sibling before in-agent dockerd or Sysbox | Medium | port/egress/security tests fail, or Sysbox proves substantially safer and equally operable |
| 2026-07-11 | Treat security parity as impossible across container and microVM backends | High | a different isolation substrate replaces privileged classic DinD |

## Research log

### 2026-07-11 — repository and documentation pass

- Read classic lifecycle/template from `origin/main` without checking out or modifying `main`.
- Read the landed sbx runtime, template, spike reports, architecture docs, and active OpenSpec inventory on `dunex`.
- Observed current-container memory/cgroup/KVM visibility with read-only commands; no heavy experiments run.
- Consulted current Docker documentation for privilege, rootless DinD, cgroup limits, storage drivers, Compose secrets, and sbx troubleshooting/prerequisites.
- Preserved the pre-existing untracked `openspec/changes/sbx-7-host-credential-injection/` directory untouched.
- Next session starts with crew-chief/Mitchell review, then the post-breakout runbook.
