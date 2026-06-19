# sbx Network and Secrets Posture

This doc covers Dune's **egress** and **secrets** posture under the sbx backend:
the non-`Open` egress baseline, sandbox-scoped rule application and the
domain-opening affordance, egress observability via `sbx policy log`, and the
secrets lifecycle (design `sbx-4` D1–D6). It is the sbx-world replacement for
the old Pipelock sidecar model.

It does **not** cover what the template contains or how it is published — those
live in [Dune sbx Template — Contents, Persistence, and Workspace](./sbx-template.md)
and [Distribution, Versioning, and Registry Access](./sbx-template-distribution.md)
(where the *template is not a secret boundary* point, `sbx-2` D7, already lives).
The final `dune logs` / `dune doctor` composition that *consumes* the policy log
and the posture check lands in `sbx-5-sbx-lifecycle-and-doctor`; this doc covers
the data sources and the posture sbx-4 establishes.

## Egress posture (D1)

The sandbox does not bypass `sbx` mediation. Outbound traffic from both the
shell and nested Docker is governed by the `sbx` sandbox policy layer, not by a
host-side proxy.

Dune's egress baseline starts from `sbx`'s **`Balanced`** preset: default-deny
plus a developer-infrastructure allowlist. The smoke-verified allowlist groups
cover:

- **AI providers** — `api.anthropic.com`, `api.openai.com`,
  `generativelanguage.googleapis.com`, and related hosts.
- **Package managers / module proxies** — `registry.npmjs.org`, `pypi.org` /
  `files.pythonhosted.org`, `proxy.golang.org` / `sum.golang.org`,
  `registry-1.docker.io`, and the other managers in the preset.
- **Code hosts and container registries** — `github.com`, `gitlab.com`,
  `ghcr.io`, `docker.io`, `gcr.io`, `quay.io`, `public.ecr.aws`, and related
  hosts.
- **Common cloud, OS packages, and cert-validation** hosts.

`Balanced` deliberately **blocks arbitrary documentation sites** (e.g.
`docs.python.org`, React, Vue, Django all return 403). That is expected: the
default is not `Open`. Use the [domain-opening affordance](#opening-a-domain-d3)
to re-enable a specific docs site.

Dune makes the active posture explicit and inspectable, and:

- **Never weakens egress to `Open`.** If Dune positively observes an `Open`
  (allow-all) posture for the instance, `up` **fails hard** with an actionable
  message.
- **Never mutates global `sbx` policy.** Dune does not call
  `sbx policy set-default` (which is global-only) or otherwise change the user's
  global default policy/profile.

### Verify-only at boot (no sandbox-scoped preset)

There is **no sandbox-scoped preset** in `sbx`: `sbx policy set-default` is
global-only (no `--sandbox` flag), and `sbx policy profile` only lists
remote-governance profiles selected at create time. Because Dune MUST NOT mutate
global policy, the baseline behaviour at boot is **verify-only**.

On `up`/`Ensure`, after the sandbox is created, `dune` inspects the instance's
active network policy with `sbx policy ls <instance> --type network` and
classifies it without writing any rule:

| Observed posture | `up` behaviour |
| --- | --- |
| Non-`Open` (a deny rule, or an allowlist with no allow-all) | proceed silently |
| `Open` (an allow-all `*` / `**` rule) | **fail hard** — refuse to operate under `Open` |
| Unconfirmable (no parseable rows, unknown decision, `policy ls` error) | **warn closed** to stderr and proceed |

The warn-closed (not fail) path for *unconfirmable* posture is deliberate: the
host's global default may already be non-`Open`, so a transient inspection
failure must not block a sandbox whose posture is actually fine. Dune fails only
when it positively sees `Open`. The warning tells the user exactly how to set a
non-`Open` default and how to open project-specific domains.

To set a non-`Open` default on the host (recommended before the first
`dune up`):

```sh
sbx policy set-default balanced        # recommended developer baseline
# or, stricter:
sbx policy set-default deny-all
```

## Sandbox-scoped rules and opening a domain (D2 / D3)

Where Dune applies an egress rule itself (opening a project domain, or adding a
deny), it uses **sandbox-scoped** rules so the user's global policy is never
touched. The CLI surface is the `--sandbox <name>` form (the positional form was
rejected during the spikes):

```text
sbx policy allow network --sandbox <instance> <domain>:443
sbx policy deny  network --sandbox <instance> <domain>
sbx policy rm    network --sandbox <instance> --resource <domain>:443
```

`<instance>` is the same `dune-<slug>-<profile>` sandbox name the sbx-3 backend
maps the Dune instance to; `sbx policy ls <instance>` shows the active rules.
Sandbox-scoped rules take effect **immediately** on the running sandbox — no
recreate needed (smoke-verified).

### Opening a domain (D3)

`Balanced` blocks common docs sites, so opening a project-specific domain is a
normal, frequent operation. The rule shape Dune uses:

- Add **both** the exact domain and a specific subdomain wildcard. `example.org`
  and `*.example.org` are **independent** — neither covers the other.
- Prefer **exact + specific-wildcard** rules over broad catch-alls (`**`, `*`,
  `**.amazonaws.com`). Dune's domain-opening helper rejects broad catch-alls.
- Pin the HTTPS port (`:443`) on allow rules; removal uses `--resource` with the
  same token that was allowed.

For example, to open `docs.python.org` for the current workspace's sandbox and
remove it again:

```sh
INSTANCE="dune-<workspace-slug>-<profile>"   # the sbx sandbox name dune uses

# Open: exact + specific wildcard, immediate effect
sbx policy allow network --sandbox "$INSTANCE" docs.python.org:443
sbx policy allow network --sandbox "$INSTANCE" *.docs.python.org:443

# Remove (note: removal uses --resource with the same token)
sbx policy rm network --sandbox "$INSTANCE" --resource docs.python.org:443
sbx policy rm network --sandbox "$INSTANCE" --resource *.docs.python.org:443
```

> The concrete `dune`-level wrapper for opening a domain is coordinated with the
> command set finalised in `sbx-5`. The behaviour and rule shape above are fixed
> by `sbx-4`; the `sbx policy allow` commands work today.

## Egress observability via `sbx policy log` (D4)

`sbx policy log <instance>` is the first-class egress observability source. It
records both blocked and allowed network events for the sandbox:

```sh
sbx policy log <instance>                 # human-readable, all sandboxes or one
sbx policy log <instance> --json --limit 200   # structured form Dune consumes
```

Each record carries the host, the sandbox it belongs to, the matching rule, the
reason, and a `proxy_type` that distinguishes how the traffic reached the sbx
proxy:

- **`forward`** — direct shell traffic (proxied/inspected).
- **`transparent`** — nested-container traffic (e.g. a `docker run` inside the
  sandbox). Nested Docker egress is governed the same way shell egress is.
- **`forward-bypass`** — an explicitly *allowed* HTTPS host skips the MITM
  forward proxy, so sbx records it as `forward-bypass` rather than `forward`.

A blocked docs-site request therefore shows up as a `blocked_hosts` record via
`forward` (shell) and `transparent` (nested Docker); once allowed, it shows up as
an `allowed_hosts` record via `forward-bypass`.

### Pipelock is gone

This observability source **replaces `dune logs pipelock`**. The old Pipelock
egress sidecar and its proxy-env model are fully removed for the sbx backend
(`sbx-4` D6): the `internal/dune/pipelock` package, the generated
`~/.config/dune/pipelock.yaml`, the `HTTP(S)_PROXY`-into-a-Dune-proxy model, and
the `dune logs pipelock` surface are all gone. There is no window where egress
was both un-proxied and un-policed — `sbx` policy governed egress before
Pipelock was removed.

> The sandbox still advertises a proxy to processes via the standard
> `HTTP_PROXY` / `HTTPS_PROXY` env vars, but that endpoint
> (`http://gateway.docker.internal:3128`) is **sbx's own policy-enforcing
> gateway**, not Pipelock. That is expected; what is gone is Dune's separate
> Pipelock proxy.

`dune logs` composes Dune-owned setup/runtime logs from the host lifecycle log
and the sandbox's `/var/log/dune/` directory, then appends records from
`sbx policy log <instance>`. App-dependency service logs are project-owned and
come from `docker compose logs` inside the sandbox.

## Secrets posture (D5)

### Prefer service-identifier secrets

Dune prefers **service-identifier secrets** (`sbx secret set <instance>
<service> ...`), which have a clean set/list/remove lifecycle (verified in the
spikes). Use these where a built-in agent or a future kit declares a service
identifier, and for registry pull auth (`sbx secret set --registry`, see
[Distribution, Versioning, and Registry Access](./sbx-template-distribution.md)).

The verified lifecycle shapes (the scope is the `dune-<slug>-<profile>` sandbox
name; reconfirm against `sbx secret --help` before use because flags may drift):

```text
sbx secret set <instance> <service> -t <token>   # set (positional SANDBOX then SERVICE, -t carries the value)
sbx secret ls  <instance>                        # list (masked, human-readable)
sbx secret rm  <instance> <service> -f           # remove (-f skips the confirm prompt)
```

No core Dune boot path sets a service secret in v1 — the backend exposes and
tests this surface, but it is **not wired into `up`/`Ensure`** yet. It is
forward-looking plumbing for built-in agent identifiers and kit-declared
services.

### Custom secrets are experimental and out-of-lifecycle

`sbx secret set-custom` is **experimental and not Dune-managed in v1**. The
spikes found its lifecycle is broken: no working removal, not auto-injected into
the sandbox env, accumulates duplicate rows on re-set, and survives sandbox
removal. Dune does **not** depend on custom secrets for boot, and the boot path
guards against `set-custom`. If you use custom secrets experimentally, expect
that cleanup may require manual `sbx`/Docker intervention or a future `sbx` fix.

### No secrets in the template; agent creds live in `/persist/agent`

Reaffirming `sbx-2` D7 (see
[The template is not a secret boundary](./sbx-template-distribution.md#the-template-is-not-a-secret-boundary-d7)):
the Dune sbx template is built from source only and is **not** a secret boundary
— no secrets are baked in. Credentials and tokens are injected at runtime
(`sbx secret set`) or supplied by the profile-persist volume, never embedded in
the published image.

Until a built-in agent or kit declares a service identifier suitable for
`sbx secret` injection, agent-provider credentials use the profile-scoped
persist location under `/persist/agent` (`sbx-3` D3 — the same re-homed
`setup-persist` hook that seeds `~/.claude`, `~/.codex`, `~/.config/gh`, etc.,
documented in [Dune sbx Template — Contents, Persistence, and
Workspace](./sbx-template.md)).

## Toolchain parity under the baseline

The sbx egress smoke (`test/smoke/sbx-egress.sh`) verified Dune's actual
toolchain against the `Balanced` baseline (template
`ghcr.io/mitchell-wallace/dune-sbx:0.4.8`): npm (`registry.npmjs.org`), pip
(`pypi.org`), Go module lookup and `GOPROXY` (`proxy.golang.org`,
`sum.golang.org`), and the AI providers (OpenAI, Anthropic, Gemini) all reached
allowed hosts, and **no non-docs toolchain/provider hosts were blocked**. The
only `Balanced`-allowlisted package ecosystem without a working toolchain in the
0.4.8 template is **cargo/Rust**: `registry.crates.io` is allowed by the
baseline, but the template does not yet ship a `cargo` binary. That is a
template/toolchain parity finding (tracked for a later template build), not an
egress-blocked-host finding — no domain needs opening for cargo to work.

## What lands later

- **`sbx-5-sbx-lifecycle-and-doctor`** — `dune doctor` (which reports the
  verified posture as `warn`, not `fail`, when it cannot be confirmed), the
  diagnostics taxonomy, and the final `dune logs` composition that reads
  `/var/log/dune/` and surfaces the `sbx policy log` source.
- **`sbx-6-sbx-kits-and-cleanup`** — kits as the per-workspace customization
  layer (kit-authored `network.allowedDomains`/`deniedDomains` and credential
  wiring), and retirement of the legacy Compose base image + `container/base/`
  tree.
