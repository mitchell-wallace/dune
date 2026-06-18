## 1. Egress baseline

- [x] 1.1 Define Dune's non-`Open` egress baseline starting from `sbx`'s `Balanced` preset, and document it (D1).
- [x] 1.2 Surface the *active* posture for an instance via `sbx policy ls` / `sbx policy log`; ensure Dune never weakens egress to `Open`, never mutates global host policy, and fails/warns closed if a non-`Open` instance posture cannot be confirmed.
- [x] 1.4 ~~Gating: confirm whether a Balanced-equivalent preset can be applied with `--sandbox <instance>` scope~~ Resolved by spike 4: **no sandbox-scoped preset exists** — `sbx policy set-default` is global-only and `sbx policy profile` only lists remote-governance profiles. Dune takes the verify-only path (D1); sandbox-scoped control stays per-domain `allow/deny network --sandbox`.
- [x] 1.3 Verify Dune's actual toolchain traffic (package managers: npm/pip/go/cargo; AI providers) works under the baseline; record any domains that must be added.

## 2. Sandbox-scoped rules and domain opening

- [x] 2.1 Apply Dune-managed egress rules scoped to the workspace's sandbox (`sbx policy allow/deny network --sandbox <instance> ...`), not the global default policy (D2). Use the `--sandbox <name>` form (positional was rejected in the spike).
- [x] 2.2 Provide a domain-opening affordance/guidance that adds the exact domain and, when needed, a specific wildcard (`example.org` + `*.example.org`), prefers exact + specific-wildcard over broad catch-alls, and takes effect immediately (D3).
- [x] 2.3 Verify opening a domain unblocks it immediately on the running sandbox and that removing the rule re-blocks it.

## 3. Egress observability

- [x] 3.1 Expose/wrap `sbx policy log <instance>` access for an instance's sandbox as the egress observability source replacing `dune logs pipelock` (D4), constructed through the `sbx-3` D5 runner seam. (Final `dune logs` composition is `sbx-5`.)
- [x] 3.2 Use the structured `sbx policy log [SANDBOX] --json` form (confirmed by spike 4, with `--limit`/`--type`); pin the command shape and observed field names in fakeRunner tests (D4).

## 4. Secrets posture

- [ ] 4.1 Document and prefer service-identifier secrets (`sbx secret set`) for built-in agent / kit-declared identifiers and registry auth (`sbx secret set --registry`) (D5). Where Dune sets/removes a service secret itself, route it through the `sbx-3` D5 runner seam and pin the verified `set`/`ls`/`rm` shapes (D5) in fakeRunner tests; reconfirm via `sbx secret --help`.
- [ ] 4.2 Document `sbx` custom secrets (`sbx secret set-custom`) as experimental and out of v1 lifecycle ownership (no working removal, not auto-injected, duplicate rows, survive sandbox removal); ensure no core Dune boot path depends on them.
- [ ] 4.3 Reaffirm no secrets are baked into the template; agent-provider creds use the profile-scoped `/persist/agent` location until a service identifier is available (D5, `sbx-2` D7, `sbx-3` D3).

## 5. Complete Pipelock removal (sequenced last)

- [x] 5.1 Confirm the egress baseline (tasks 1–2) is in place so `sbx` policy governs egress before removing Pipelock (D6 ordering).
- [x] 5.2 Delete the `internal/dune/pipelock` package, `ensurePipelockConfig`, and `~/.config/dune/pipelock.yaml` generation.
- [x] 5.3 Remove the proxy-env (`HTTP(S)_PROXY`) model and the `dune logs pipelock` surface (replaced by `sbx policy log`, task 3.1).
- [x] 5.4 Remove any remaining Pipelock references left present-but-unused by `sbx-3`.

## 6. Tests and verification

- [x] 6.1 Add fakeRunner tests (via the `sbx-3` D5 seam) asserting the constructed argument shapes for: sandbox-scoped allow/deny/rm rules and the domain-opening rule shape (exact + wildcard, `<domain>:443`, removal via `--resource`); the `sbx policy log <instance> --limit <n>` invocation; and the service-secret `set`/`ls`/`rm` shapes (D2, D4, D5). These pin against silent `sbx` flag drift.
- [x] 6.2 Smoke-verify in an ephemeral sandbox: baseline blocks a representative docs site; opening its exact + wildcard domains unblocks it; `sbx policy log` shows the records; nested Docker traffic is also governed.
- [x] 6.3 Run `go build ./cmd/dune` and `go test ./...`; remove temporary sandboxes.

Verification evidence (2026-06-19): `just smoke-sbx-egress --report tmp/sbx-egress-smoke.20260618T170535Z.log` against `ghcr.io/mitchell-wallace/dune-sbx:0.4.8` passed the docs-site block/unblock/re-block, policy-log `forward`/`transparent`/`forward-bypass`, no-Pipelock, and cleanup assertions. Toolchain/provider policy findings recorded no non-docs blocked hosts; npm, pip, Go/GOPROXY, Go module lookup, OpenAI, Anthropic, and Gemini reached allowed hosts. The 0.4.8 template does not include a `cargo` binary, so cargo produced a template/toolchain parity finding rather than an sbx blocked-host finding.

## 7. Documentation

- [ ] 7.1 Document the egress posture (baseline, sandbox-scoped rules, opening domains), the `sbx policy log` observability path, and the secrets posture. Note `dune logs pipelock` is gone and the final `dune logs` lands in `sbx-5`.
