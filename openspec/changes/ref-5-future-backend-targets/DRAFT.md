```md
## Proposal: Document Future Backend Targets

## Summary

Document and assess Dune’s plausible future backend targets after the environment plan and Docker Compose backend boundaries exist.

This change should not implement a backend. It should produce a clear architectural assessment of where Dune could go next: continued local Docker hardening, remote Docker on a VPS/Raspberry Pi, local MicroVMs, or remote MicroVMs.

The output should preserve strategy for future planning contexts without prematurely committing the codebase to one backend model.

## Depends On

```text
introduce-environment-plan-boundary
extract-docker-compose-backend
````

## Problem

Dune currently uses local Docker Compose. That is the right first backend, but likely not the only useful backend shape.

Future possibilities include:

* local Docker Compose;
* remote Docker Compose on a VPS;
* remote Docker Compose on a Raspberry Pi or homelab box;
* local MicroVM;
* remote MicroVM;
* possibly hybrid remote environment management plus local shell/IDE connection.

These targets have different implications for filesystem sync, persistence, networking, Pipelock placement, shell attach, image/build strategy, latency, and security boundaries.

Without a documented assessment, near-term refactors may accidentally overfit to local Docker or prematurely design for MicroVMs.

## Objectives

This change should:

* document the candidate backend targets;
* compare their requirements and tradeoffs;
* identify which abstractions are already sufficient;
* identify where the current model is still local-Docker-shaped;
* recommend the next backend investigation path;
* produce concrete questions for future implementation proposals;
* avoid code changes unless minor docs links are needed.

## Non-Goals

This change should not:

* implement remote Docker;
* implement MicroVM support;
* add backend selection;
* add SSH management;
* add filesystem sync;
* add VM image building;
* change Docker Compose backend behaviour;
* change the base image;
* change Pipelock integration.

## Candidate Backend Targets

### 1. Local Docker Compose

Current backend.

Strengths:

* simplest implementation;
* fast local iteration after image exists;
* strong compatibility with current Compose model;
* easiest to debug;
* best smoke-test baseline.

Weaknesses:

* depends on local Docker daemon;
* host resource usage can be high;
* not ideal for weaker laptops;
* Docker-specific isolation model;
* base image rebuild loop can be slow.

### 2. Remote Docker Compose

Dune manages containers on a VPS, Raspberry Pi, or other remote machine.

Likely shape:

```text
host Dune CLI
  -> SSH/control connection
  -> remote Docker Compose project
  -> remote persistent volume/state
  -> shell attach over SSH/docker exec
```

Key questions:

* How is the repo made available remotely?

  * git clone?
  * rsync?
  * bind mount from remote path?
  * remote worktree management?
* Where does profile persistence live?
* Where does Pipelock run?
* How are ports forwarded?
* How does shell attach behave?
* How are credentials/auth separated between host and remote?
* How are logs retrieved?

Likely value:

* offload heavy containers from laptop;
* use always-on dev boxes;
* support Raspberry Pi/homelab workflows;
* preserve Docker compatibility.

### 3. Local MicroVM

Dune launches a local VM-like isolated environment.

Likely shape:

```text
host Dune CLI
  -> VM manager
  -> guest image/disk
  -> workspace mount or sync
  -> persistent guest state
  -> network policy path
  -> shell attach
```

Key questions:

* Which VM substrate?
* How is the base image converted or rebuilt?
* Is the environment still container-like inside the VM?
* Does Pipelock run inside guest, outside guest, or as host gateway?
* How does workspace mount work?

  * virtiofs?
  * 9p?
  * sync?
* How are ports and logs exposed?

Likely value:

* stronger isolation boundary than Docker;
* better fit for adversarial agent threat models;
* possible cleaner network containment.

Main concern:

* significantly more implementation complexity than remote Docker.

### 4. Remote MicroVM

Dune controls VM-like isolated environments on another host.

Likely value:

* strongest isolation/offload direction;
* useful for hosted or semi-hosted Dune later.

Main concern:

* combines remote management complexity with MicroVM complexity;
* probably not the next implementation step unless product goals change.

## Assessment Dimensions

The assessment should compare targets across:

```text
implementation complexity
security boundary
local resource usage
startup latency
workspace filesystem strategy
profile persistence strategy
Pipelock/egress strategy
shell attach strategy
port forwarding strategy
image/build strategy
testability
CI feasibility
user configuration burden
```

## Current Abstraction Fit

Assess whether these concepts are sufficient:

```text
EnvironmentPlan
EnvironmentSpec
PersistenceSpec
EgressSpec
BackendTarget
Docker Compose backend
diagnostics/check model
```

Identify where names or APIs still overfit to Docker, such as:

```text
volume naming
compose project identity
compose path assumptions
docker exec assumptions
sidecar assumptions
local filesystem mount assumptions
image build assumptions
```

## Expected Output

Create an architecture document, likely:

```text
docs/architecture/future-backend-targets.md
```

or an OpenSpec concept document:

```text
openspec/changes/assess-future-backend-targets/
  proposal.md
  design.md
  findings.md
```

The document should include:

```text
recommended next backend to investigate
non-recommended paths for now
backend comparison table
open design questions
risks to preserve in current abstractions
suggested future proposal sequence
```

## Likely Recommendation Bias

The expected recommendation is probably:

```text
1. Continue hardening local Docker Compose.
2. Investigate remote Docker Compose before MicroVMs.
3. Keep MicroVM compatibility as a design constraint, not the next implementation.
```

Reasoning:

* remote Docker preserves most of the current Docker backend model;
* it directly addresses local resource constraints;
* it is useful for VPS/Raspberry Pi workflows;
* it exercises the backend target abstraction without requiring VM image architecture;
* MicroVMs remain strategically important but have more unknowns.

The assessment should verify or challenge this recommendation rather than assume it.

## Acceptance Criteria

This change is complete when:

* future backend targets are documented;
* local Docker, remote Docker, local MicroVM, and remote MicroVM are compared;
* key design questions are listed;
* current abstraction gaps are identified;
* a recommended next investigation path is stated;
* no backend implementation is added;
* no user-facing behaviour changes.

## Risk Areas

### Premature commitment

Do not choose a MicroVM stack or remote orchestration implementation in this proposal.

### Too much abstraction pressure

Do not force current code to support hypothetical backends before the assessment is complete.

### Ignoring remote Docker

Remote Docker may be the more practical next backend than MicroVMs and should be assessed seriously.

### Treating documentation as final design

This should guide future proposals, not replace implementation design.

### Losing Dune’s product focus

Backend targets should be evaluated against Dune’s core value: a persistent, isolated, tool-rich AI coding environment entered with one memorable command.

```
```