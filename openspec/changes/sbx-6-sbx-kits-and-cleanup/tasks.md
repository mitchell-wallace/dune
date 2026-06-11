## 1. Kits as the customization layer

- [ ] 1.1 Decide the kit types Dune ships/recommends (mixin kits for the common case; agent kits only if needed) and document kits as the replacement for `Dockerfile.dune` (D1).
- [ ] 1.2 Decide and document where kit definitions live (recommended: per-repo, alongside `rally.toml`).
- [ ] 1.3 (Optional) Provide a Dune-recommended docs-domain kit or documented recipe (exact + specific-wildcard rules) addressing the `Balanced`-blocks-docs friction from `sbx-4` (D2).

## 2. Template refresh and versioning (kit-aware)

- [ ] 2.1 Document the template refresh/republish flow and keep the `VERSION` + template `IMAGE_VERSION` lockstep (from `sbx-2` D6) (D3).
- [ ] 2.2 Define how kits target/pin a template version so kit + template stay compatible.

## 3. Retire the legacy Docker Compose scaffolding and base image (parity-gated)

- [ ] 3.1 Confirm sbx-backend parity before removing anything (gate).
- [ ] 3.2 Remove leftover Compose rendering/helpers and golden/validation test fixtures not already deleted in `sbx-3`.
- [ ] 3.3 Remove the legacy `dune-base` Compose image build: root `Dockerfile`, the `container/base/` tree (after the template build no longer needs its assets, `sbx-2` D2), and the `image.yml` base build-and-push job.
- [ ] 3.4 Update AGENTS.md / version-bump guidance so the lockstep references the template's `IMAGE_VERSION` rather than `container/base/IMAGE_VERSION` (D4).

## 4. Stale local Docker artifact cleanup

- [ ] 4.1 Provide an opt-in, list-then-confirm cleanup story (a small `dune` helper and/or documented manual steps) for `dune-persist-<profile>` volumes, `dune-local-<slug>` images, and generated compose files; never remove non-Dune artifacts (D5).
- [ ] 4.2 Add user-facing migration notes for the one-time transition from the Docker Compose backend.

## 5. Verification and docs

- [ ] 5.1 If a cleanup helper is added, run `go build ./cmd/dune` and `go test ./...`; add tests asserting it only targets Dune-scoped artifacts and requires confirmation.
- [ ] 5.2 Verify the sbx path still builds and works after the legacy removals (template build, `dune up`, `dune doctor`).
- [ ] 5.3 Update architecture/README docs: kits are the customization layer; the legacy Compose backend and `dune-base` image are retired; the template is the sole runtime artifact.
