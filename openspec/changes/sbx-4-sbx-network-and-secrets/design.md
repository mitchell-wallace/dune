## Context

The Docker Compose backend mediates egress through a Pipelock sidecar: `app.go` generates `~/.config/dune/pipelock.yaml`, runs Pipelock as a second Compose service, injects `HTTP(S)_PROXY` into the agent, and exposes logs via `dune logs pipelock`. `sbx-3-sbx-runtime-backend` stops starting Pipelock for the sbx path but intentionally leaves the `internal/dune/pipelock` package, the generated config, and the proxy-env model present-but-unused so egress is never unmediated mid-migration.

The spikes proved `sbx`'s own network layer can take over. Spike 2: a scoped `sbx policy deny network --sandbox <name> example.com` blocked both the shell (`forward`) and a nested Docker container (`transparent`); deny rules beat the allow-all default; `sbx policy log <name>` recorded the events. Spike 3: the `Balanced` preset is default-deny plus a developer-infrastructure allowlist and **blocks arbitrary docs sites**; opening a domain is immediate and sandbox-scoped; exact vs wildcard forms are independent; custom secrets work for proxy substitution but have a broken lifecycle.

This change defines Dune's sbx egress and secrets posture and removes Pipelock. Scope excludes the final `dune logs`/`dune doctor` composition (`sbx-5`) and kit-authored rules (`sbx-6`).

## Goals / Non-Goals

**Goals**
- A defined, non-`Open`, inspectable default egress baseline for Dune sbx workspaces.
- Apply Dune-managed egress rules scoped to the workspace's sandbox, without clobbering the user's global `sbx` policy.
- A domain-opening affordance/guidance that handles exact + specific-wildcard correctly and takes effect immediately.
- Expose `sbx policy log <instance>` as the egress observability source replacing `dune logs pipelock`.
- A secrets posture favouring service-identifier secrets; custom secrets explicitly experimental/out-of-lifecycle.
- Complete, correctly sequenced Pipelock removal.

**Non-Goals**
- Final `dune logs`/`dune doctor` composition and command mapping (`sbx-5`).
- Kit-authored network rules/credentials as the durable customization layer (`sbx-6`).
- Any reliance on custom secrets for core Dune boot.

## Decisions

### D1: Default baseline is non-`Open` and explicit, starting from `Balanced`
Dune's posture starts from `sbx`'s `Balanced` preset (default-deny + developer-infrastructure allowlist). Dune SHALL NOT silently weaken egress to `Open`, and it SHALL NOT mutate the user's global `sbx` default policy/profile. On sandbox preparation, Dune must make the instance posture inspectable and non-`Open`: prefer applying a sandbox-scoped Balanced-equivalent baseline when `sbx` supports it; otherwise verify the active posture is already non-`Open` and return an actionable warning/failure if it cannot be confirmed.

**The sandbox-scoped baseline mechanism is unverified and is a gating verification point.** The spikes only switched the **global** default policy to `Balanced` (`sbx policy set-default` / `sbx policy reset`) and applied **per-domain** sandbox-scoped allow/deny rules (`--sandbox <name>`); they never applied a *Balanced-equivalent preset scoped to a single sandbox*. Before relying on it, implementation MUST confirm against the installed `sbx` (inspect `sbx policy --help` and `sbx policy set-default --help` / `sbx policy profile --help`) whether a Balanced preset can be applied with `--sandbox <instance>` scope without touching global policy, and record the exact command. If no sandbox-scoped preset mechanism exists, Dune MUST NOT mutate global policy to obtain one; instead it falls back to the **verify-only** path below.

**Boot (`up`) behavior vs. read-only report.** This change fixes two distinct behaviors so they do not contradict each other or `sbx-5`:
- At sandbox preparation (`up`/`Ensure`), if Dune can neither apply a sandbox-scoped non-`Open` baseline nor confirm the active posture is already non-`Open`, it MUST warn closed with an actionable message (how to set a non-`Open` posture) and MUST NOT silently apply or assume `Open`. It MUST NOT escalate to a hard failure solely because confirmation was unavailable, since the global default may already be non-`Open`; it fails hard only when it positively observes an `Open` posture it was asked to operate under.
- The read-only `dune doctor` egress check (`sbx-5`) reports the same posture as `warn` (not `fail`) when it cannot be confirmed. sbx-4 exposes the inspection (D4 / `sbx policy ls`); sbx-5 composes the check.

### D2: Dune-managed rules are sandbox-scoped, not global
Where Dune applies egress rules itself (baseline assertion when supported, opening a domain, or adding a deny), it uses sandbox-scoped rules (`sbx policy allow/deny network --sandbox <instance> ...`), which spike 3 showed take effect immediately. This avoids mutating the user's global `sbx` default policy/profile. If a sandbox-scoped baseline cannot be applied, Dune verifies the active posture instead of assuming it. Note the CLI surface uses `--sandbox <name>` (the positional form was rejected in spike 3).

`<instance>` is the same `dune-<slug>-<profile>` sandbox name `sbx-3` D3 maps the Dune instance to; policy/secret commands target that exact name. These commands are constructed through the `sbx-3` D5 command-runner seam, so their argument construction is asserted in fakeRunner tests rather than only at runtime. Verified rule shapes from the spikes (pin these in fakeRunner tests; reconfirm via `sbx policy --help` before use because flags may drift across versions):

```text
sbx policy allow network --sandbox <instance> <domain>:443     # spike 3
sbx policy deny  network --sandbox <instance> <domain>         # spike 2
sbx policy rm    network --sandbox <instance> --resource <domain>:443   # spike 3 (removal uses --resource)
sbx policy log   <instance> --limit <n>                         # spike 2
```

### D3: Domain-opening affordance
Opening a project domain is common (docs sites are blocked under `Balanced`). Dune provides a thin affordance and/or explicit guidance that:
- adds **both** the exact domain and a specific wildcard when both are needed (`example.org` and `*.example.org` are independent);
- prefers exact + specific-wildcard rules over broad catch-alls (`**.amazonaws.com` etc.);
- is sandbox-scoped (D2) and effective immediately.
The concrete command name (e.g. a `dune` wrapper vs. documented `sbx policy allow`) is coordinated with the command set finalised in `sbx-5`; this change fixes the *behavior and rule shape*, not necessarily a new top-level verb.

### D4: Egress observability via `sbx policy log`
`sbx policy log <instance>` is the first-class egress observability source, replacing `dune logs pipelock`. This change ensures the access is available/wrapped for the instance's sandbox through the `sbx-3` D5 runner seam; the final `dune logs` composition (policy log + Dune-owned setup/runtime logs) is `sbx-5`.

The spikes verified `sbx policy log <instance> --limit <n>` records both `forward` (direct shell) and `transparent` (nested Docker) traffic, plus `forward-bypass` for allowed registry traffic. The exact invocation/parse shape used by the wrapper (whether a `--json` form exists, and the field names) is **unverified** and MUST be confirmed against `sbx policy log --help` and pinned in fakeRunner tests before the wrapper relies on parsed fields; if no structured form exists, the wrapper passes the raw output through and `sbx-5` formats it.

### D5: Secrets posture
- **Prefer service-identifier secrets** (`sbx secret set <scope> <service> ...`): spike 3 showed a clean set/list/remove lifecycle. Use these where a built-in agent or a future kit declares a service identifier, and for registry auth (`sbx secret set --registry`, see `sbx-2`). Where Dune itself sets/removes a service secret it goes through the `sbx-3` D5 runner seam, so the argument shape is asserted in fakeRunner tests. Verified lifecycle shapes from spike 3 (the scope is the `dune-<slug>-<profile>` sandbox name; reconfirm via `sbx secret --help` before use):

```text
sbx secret set <instance> <service> -t <ENV_VAR>   # spike 3 (set)
sbx secret ls  <instance>                            # spike 3 (list, masked)
sbx secret rm  <instance> <service> -f               # spike 3 (remove)
```
- **Custom secrets are experimental and out of v1 lifecycle ownership.** Spike 2/3 found `sbx secret set-custom` has no working removal, is not auto-injected into the sandbox env, accumulates duplicate rows on re-set, and survives sandbox removal. Dune does not depend on custom secrets for boot in v1; if offered experimentally, Dune warns that cleanup may require manual `sbx`/Docker intervention or a future `sbx` fix.
- **No secrets in the template** (reaffirms `sbx-2` D7): the template is built from source, never via `sbx template save`, and is not a secret boundary.
- Agent-provider credentials continue to use persisted config under the profile-scoped `/persist` location (`sbx-3` D3) unless/until a built-in agent or kit declares a service identifier suitable for `sbx secret` injection.

### D6: Pipelock removal, sequenced after the baseline
Delete the `internal/dune/pipelock` package, `ensurePipelockConfig` and the `~/.config/dune/pipelock.yaml` generation, the proxy-env (`HTTP(S)_PROXY`) model, and the `dune logs pipelock` surface (its replacement is D4). Ordering: removal happens only after D1–D2 establish that `sbx` policy governs egress for the sbx path, so there is no window where egress is both un-proxied and un-policed. (Pipelock remains for as long as the legacy Compose backend exists; the legacy backend itself is retired in `sbx-6`.)

## Risks / Trade-offs

- **Docs-site friction.** `Balanced` blocks common docs sites; agent workflows that read library docs will hit 403s until domains are opened. Mitigation: the D3 affordance + clear guidance; consider a Dune-recommended docs allowlist (tracked for kits in `sbx-6`).
- **Broad-wildcard acceptability.** `Balanced` itself includes broad wildcards (`**.amazonaws.com`, `**.googleapis.com`, `**.githubusercontent.com`). Whether Dune endorses these in its recommended posture is an open question.
- **Package-manager / provider traffic under a non-open baseline.** Must be verified before defaulting (npm/pip/go/cargo, AI providers) — most are in `Balanced`, but confirm against Dune's actual toolchain.
- **Custom-secret immaturity.** Building UX on custom secrets now would inherit a broken lifecycle. Mitigation: exclude from v1.
- **Global-policy clobbering.** Setting a global default profile could override the user's own policy. Mitigation: never mutate global policy; use sandbox-scoped rules and fail/warn closed when a non-`Open` posture cannot be confirmed.
- **Removal ordering.** Removing Pipelock before the baseline is established would leave egress unmediated. Mitigation: enforce the D6 ordering.

## Migration Plan

1. Land after `sbx-3` (the sbx backend owns the lifecycle).
2. Establish the egress baseline expectation (D1) and sandbox-scoped rule application + domain-opening affordance (D2, D3); wire `sbx policy log` access (D4).
3. Define and document the secrets posture (D5).
4. Only then remove Pipelock and the proxy-env model and replace `dune logs pipelock` (D6).
5. `sbx-5` composes the final `dune logs`/`dune doctor` on top of D4/D1.

## Open Questions

- Whether broad `Balanced` wildcards (`**.amazonaws.com`, `**.googleapis.com`, `**.githubusercontent.com`) are acceptable in Dune's recommended default posture.
- Domain-opening UX: a `dune`-level wrapper vs. documented `sbx policy allow` guidance vs. kit-authored domain rules (final form with `sbx-5`/`sbx-6`).
- How package-manager and agent-provider traffic behaves under the non-open baseline against Dune's actual toolchain (verify before shipping).
- Whether any custom-secret usage is offered experimentally, with explicit manual-cleanup warnings.
