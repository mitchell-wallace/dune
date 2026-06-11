## 1. Egress baseline

- [ ] 1.1 Define Dune's recommended non-`Open` egress baseline starting from `sbx`'s `Balanced` preset, and document it (D1).
- [ ] 1.2 Surface the *active* posture for an instance via `sbx policy ls` / `sbx policy log` rather than assuming it; ensure Dune never weakens egress to `Open` on the user's behalf.
- [ ] 1.3 Verify Dune's actual toolchain traffic (package managers: npm/pip/go/cargo; AI providers) works under the baseline; record any domains that must be added.

## 2. Sandbox-scoped rules and domain opening

- [ ] 2.1 Apply Dune-managed egress rules scoped to the workspace's sandbox (`sbx policy allow/deny network --sandbox <instance> ...`), not the global default policy (D2). Use the `--sandbox <name>` form (positional was rejected in the spike).
- [ ] 2.2 Provide a domain-opening affordance/guidance that adds the exact domain and, when needed, a specific wildcard (`example.org` + `*.example.org`), prefers exact + specific-wildcard over broad catch-alls, and takes effect immediately (D3).
- [ ] 2.3 Verify opening a domain unblocks it immediately on the running sandbox and that removing the rule re-blocks it.

## 3. Egress observability

- [ ] 3.1 Expose/wrap `sbx policy log <instance>` access for an instance's sandbox as the egress observability source replacing `dune logs pipelock` (D4). (Final `dune logs` composition is `sbx-5`.)

## 4. Secrets posture

- [ ] 4.1 Document and prefer service-identifier secrets (`sbx secret set`) for built-in agent / kit-declared identifiers and registry auth (`sbx secret set --registry`) (D5).
- [ ] 4.2 Document `sbx` custom secrets (`sbx secret set-custom`) as experimental and out of v1 lifecycle ownership (no working removal, not auto-injected, duplicate rows, survive sandbox removal); ensure no core Dune boot path depends on them.
- [ ] 4.3 Reaffirm no secrets are baked into the template; agent-provider creds use the profile-scoped `/persist` location until a service identifier is available (D5, `sbx-2` D7, `sbx-3` D3).

## 5. Complete Pipelock removal (sequenced last)

- [ ] 5.1 Confirm the egress baseline (tasks 1–2) is in place so `sbx` policy governs egress before removing Pipelock (D6 ordering).
- [ ] 5.2 Delete the `internal/dune/pipelock` package, `ensurePipelockConfig`, and `~/.config/dune/pipelock.yaml` generation.
- [ ] 5.3 Remove the proxy-env (`HTTP(S)_PROXY`) model and the `dune logs pipelock` surface (replaced by `sbx policy log`, task 3.1).
- [ ] 5.4 Remove any remaining Pipelock references left present-but-unused by `sbx-3`.

## 6. Tests and verification

- [ ] 6.1 Add tests for sandbox-scoped rule construction and the domain-opening rule shape (exact + wildcard), via the runner seam from `sbx-3`.
- [ ] 6.2 Smoke-verify in an ephemeral sandbox: baseline blocks a representative docs site; opening its exact + wildcard domains unblocks it; `sbx policy log` shows the records; nested Docker traffic is also governed.
- [ ] 6.3 Run `go build ./cmd/dune` and `go test ./...`; remove temporary sandboxes.

## 7. Documentation

- [ ] 7.1 Document the egress posture (baseline, sandbox-scoped rules, opening domains), the `sbx policy log` observability path, and the secrets posture. Note `dune logs pipelock` is gone and the final `dune logs` lands in `sbx-5`.
