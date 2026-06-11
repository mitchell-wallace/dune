## Why

Under the Docker Compose backend, the `agent` container reaches external HTTP(S) services only through the Pipelock sidecar, which provides outbound egress policy and observability (`dune logs pipelock`). `sbx-3-sbx-runtime-backend` stops *starting* Pipelock for the sbx path but deliberately leaves the `internal/dune/pipelock` package, the generated `~/.config/dune/pipelock.yaml`, and the proxy-env model physically present-but-unused, so that egress is never left unmediated between changes. This change closes that loop: it defines Dune's sbx **network** and **secrets** posture so the `sbx` policy layer owns egress, and then completes Pipelock removal.

The spikes established the direction. Spike 2 verified that `sbx` network policy enforces egress for both direct shell traffic (`forward`) and nested Docker traffic (`transparent`), that deny rules take precedence over the allow-all default, and that `sbx policy log <sandbox>` records blocked and allowed requests with useful detail. Spike 3 characterised the middle `Balanced` preset: default-deny plus a developer-infrastructure allowlist (AI providers, package managers, code hosts, container registries, common cloud, OS packages, cert validation) that deliberately **blocks arbitrary documentation sites** (React, Vue, Django, docs.python.org, etc. all returned 403). It also confirmed that opening a domain is immediate and sandbox-scoped (`sbx policy allow network --sandbox <name> <domain>`), that exact and wildcard forms do not cover each other (`example.org` vs `*.example.org`), and that the production default should not be `Open`.

This proposal scopes the **egress posture and secrets posture, plus Pipelock removal**. It assumes the sbx backend from `sbx-3`. It does not define the final `dune logs`/`dune doctor` composition (that is `sbx-5`); it only exposes the `sbx policy log` data source those commands consume.

## What Changes

- **NEW** A defined, non-`Open` default egress baseline for Dune sbx workspaces, starting from `sbx`'s `Balanced` preset (default-deny + developer-infrastructure allowlist). Dune makes the active posture **explicit and inspectable**, never weakens it to `Open`, and fails/warns closed if a non-`Open` instance posture cannot be confirmed without mutating global host policy.
- **NEW** A way to apply Dune's egress rules **scoped to the workspace's sandbox** (`sbx policy allow/deny network --sandbox <instance> ...`) so Dune-managed rules do not clobber the user's global `sbx` default policy/profile.
- **NEW** A domain-opening affordance and/or clear guidance for opening project-specific domains (especially docs sites), which adds **both** the exact domain and a specific wildcard when both are needed, prefers exact + specific-wildcard rules over broad catch-alls, and takes effect immediately on the running sandbox. (The exact command surface is coordinated with the `dune` command set finalised in `sbx-5`.)
- **NEW** Egress observability via `sbx policy log <instance>` as the first-class source that replaces `dune logs pipelock`. This change exposes/wraps that access; the final `dune logs` composition lands in `sbx-5`.
- **NEW** A secrets posture: prefer **service-identifier secrets** (`sbx secret set`, clean set/list/remove lifecycle observed in spike 3) and built-in agent / future kit-declared identifiers; treat `sbx` **custom secrets** (`sbx secret set-custom`) as experimental and **out of v1 lifecycle ownership** (observed gaps: no working removal, not auto-injected into the sandbox env, can accumulate duplicate rows, survive sandbox removal). Reaffirm that no secrets are baked into the Dune template (`sbx-2` D7).
- **REMOVE** Complete Pipelock removal for the sbx backend: delete the `internal/dune/pipelock` package, the generated `~/.config/dune/pipelock.yaml`, the proxy-env (`HTTP(S)_PROXY`) model, and the `dune logs pipelock` surface — sequenced **after** the sbx egress baseline above is in place so egress is mediated by `sbx` policy throughout.

### Non-goals (explicitly deferred)

- The final `dune logs` / `dune doctor` composition and the broader command mapping (`sbx-5-sbx-lifecycle-and-doctor`).
- Kit-authored network rules / credential wiring as the durable customization layer (`sbx-6-sbx-kits-and-cleanup`).
- The Dune sbx template and the CLI runtime backend themselves (`sbx-2`, `sbx-3`).
- Any reliance on `sbx` custom secrets for core Dune boot (explicitly excluded from v1).

## Capabilities

### New Capabilities

- `sbx-network-policy`: Dune's sbx egress posture — a defined non-`Open` baseline, sandbox-scoped rule application, a domain-opening affordance, and `sbx policy log` observability — with the sandbox never bypassing `sbx` mediation.
- `sbx-secrets`: Dune's sbx secrets posture — prefer service-identifier secrets, treat custom secrets as experimental/out-of-lifecycle, and keep secrets out of the template.

### Removed Capabilities

- `network-proxy` (Pipelock): retired for the sbx backend. There is no promoted `network-proxy` main spec under `openspec/specs/` to emit a `REMOVED` delta against, so the retirement is captured as `ADDED` requirements in the `sbx-network-policy` spec (egress is provided solely by `sbx` policy; Pipelock and its proxy-env model are gone).

## Impact

- **Egress**: Outbound traffic from the sandbox shell and nested Docker is governed by `sbx` policy with a defined, inspectable, non-`Open` baseline. Pipelock is gone.
- **Architecture/code**: `internal/dune/pipelock` and `pipelock.yaml` generation are deleted; the proxy-env model and `dune logs pipelock` are removed. Any remaining references from `sbx-3` are cleaned up.
- **Secrets**: Service-identifier secrets are the supported path; custom secrets are documented as experimental and not Dune-managed in v1.
- **User-facing behavior**: Some traffic that “just worked” under Open/Pipelock (notably arbitrary docs sites) is denied by default; the domain-opening affordance/guidance is how users re-enable specific domains. Verify package-manager and agent-provider traffic under the baseline before shipping it.
- **Sequencing**: Depends on `sbx-3` (the backend must own the lifecycle before Pipelock is removed). Feeds `sbx-5` (`dune logs`/`dune doctor` consume the policy baseline and `sbx policy log`).

## Depends On

`sbx-3-sbx-runtime-backend` (the sbx backend must exist and own the sandbox lifecycle/egress before Pipelock is removed). Builds on the network behavior proven against `sbx-2-dune-sbx-template`.
