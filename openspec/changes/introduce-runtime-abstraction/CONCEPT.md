Q: what are some other areas we can improve the reliability of the codebase? give me some suggestions, and recommend an improvement route. note that at some point I might want to support some form of MicroVM-based containers as an alternative to Docker, but not commited to that yet, just something to condsider with regards to the codebase structure and APIs. Could also draft an openspec proposal for that to document it formally, even if it just sits as a proposal for a while.



NOTE: The below may be better broken into multiple changes

---

Here’s where I’d improve reliability next, in roughly the order I’d do it.

**Recommended Route**
I’d do a “runtime abstraction + reliability guardrails” track before any MicroVM work. Not implementing MicroVMs yet, just shaping the code so Docker is one runtime backend instead of being smeared through app logic.

1. **Introduce a runtime interface around Docker**
   Right now `internal/dune/app.go` directly shells out to `docker`, `docker compose`, image pulls, volume creation, service checks, logs, etc. I’d extract this behind something like:

   ```go
   type Runtime interface {
     Validate(ctx context.Context) error
     EnsureVolume(ctx context.Context, name string) error
     ImageExists(ctx context.Context, ref string) bool
     PullImage(ctx context.Context, ref string) error
     RenderOrValidateCompose(ctx context.Context, project Project) error
     Up(ctx context.Context, project Project) error
     Down(ctx context.Context, project Project) error
     ExecShell(ctx context.Context, project Project) error
     Logs(ctx context.Context, project Project, service string) error
   }
   ```

   Keep Docker as the only implementation for now. This makes future MicroVM support a contained design problem instead of a rewrite.

2. **Split project planning from runtime execution**
   The CLI currently resolves workspace/profile/config, renders compose, validates Docker, pulls images, creates volumes, and starts containers in one flow. I’d split:
   - “plan the environment” into a pure/testable project model
   - “execute the plan” into runtime backend code

   That would make failures easier to test and would clarify what a MicroVM backend must support.

3. **Add command-runner test infrastructure**
   The current tests use a shell-script Docker shim in one large test. That works, but it’s hard to expand. I’d add a Go `CommandRunner` abstraction and fake runner so tests can assert exact runtime operations without writing temp shell scripts.

4. **Strengthen failure-mode tests**
   Add tests for:
   - Docker daemon unavailable
   - compose validation failure with stderr details
   - pull failure with local-image fallback
   - pull failure without local fallback
   - corrupt `profiles.json`
   - invalid/stale pipelock config
   - `Dockerfile.dune` build failure
   - compose `up` failure includes recent logs

5. **Formalize runtime/backend OpenSpec proposal**
   I’d definitely document the MicroVM thought now, even if it sits. The proposal should not commit to Firecracker/Cloud Hypervisor/Lima/Finch/etc. yet. It should define the host/runtime boundary and the portability goals.

**Other Good Reliability Improvements**
- Add a `dune doctor` command:
  Checks Docker/Compose, image availability, write access to config/data dirs, Pipelock config validity, base image version, and profile mapping health.

- Add structured errors:
  User-facing failures should have clear recovery hints. Example: “Docker daemon unavailable” vs raw `exit status 1`.

- Validate generated compose through a pure snapshot plus runtime validation:
  Keep golden tests, but also add table tests for profiles, custom workspace names, missing `Dockerfile.dune`, and timezone behavior.

- Make smoke tests more observable:
  The scripts are quiet by default, which is nice until a long npm update is running. Add optional `--verbose` and phase logging like “pinning gemini 0.39.1”.

- Add CI paths for smoke tiers:
  You already added base/tool smoke in image CI. Later, separate “fast PR” vs “container PR” jobs would avoid always paying full image cost.

**MicroVM-Aware Design Notes**
The key thing is not “support MicroVMs” yet. It’s to avoid assuming forever that:
- there is a Docker daemon
- Compose YAML is the universal runtime plan
- named Docker volumes are the only persistence model
- `docker exec` is the only shell-entry mechanism
- service networking is Compose-network based

So I’d introduce internal concepts like:
- `RuntimeBackend`
- `WorkspaceMount`
- `PersistStore`
- `AgentInstance`
- `ProxyInstance`
- `RuntimePlan`

Docker Compose can remain the first renderer/executor. A future MicroVM backend could map the same plan to VM disk images, bind mounts/virtiofs, port forwarding, and process startup.

**My Recommendation**
Next change: create an OpenSpec proposal called something like `runtime-backend-abstraction`.

Scope it to:
- extract Docker operations behind an interface
- keep behavior identical
- improve tests with a fake runtime/command runner
- explicitly document future MicroVM compatibility goals as non-goals for the first implementation

That gives you immediate reliability payoff and sets up the future architecture without prematurely choosing a MicroVM stack.