## 1. Kits as the customization layer

- [ ] 1.0 Confirm the `sbx` kit surface against the installed binary before documenting kits as supported: record the verified kit subcommand(s)/YAML schema and the create-with-kit flow (`sbx kit --help` / `sbx create --help`); author one minimal mixin kit and smoke-confirm a sandbox built from the Dune template with that kit picks up its addition (declared env var/file present in the sandbox). If kits can't be applied as documented, fall back to docs-only recipes and do not claim a kit build path (D1).
- [ ] 1.1 Decide the kit types Dune ships/recommends (mixin kits for the common case; agent kits only if needed) and document kits as the replacement for `Dockerfile.dune` (D1).
- [ ] 1.2 Decide and document where kit definitions live (recommended: per-repo, alongside `rally.toml`).
- [ ] 1.3 Provide a Dune-recommended documented docs-domain kit recipe (exact + specific-wildcard rules, using the `sbx-4`-verified `sbx policy allow network --sandbox <instance> <domain>:443` / `'*.<domain>:443'` form, no `set-default`, no catch-alls) addressing the `Balanced`-blocks-docs friction from `sbx-4` (D2).
- [ ] 1.4 Smoke-verify the docs-domain recipe against the `sbx` binary: with the recipe applied, a fetch of a representative `Balanced`-blocked docs domain succeeds while an out-of-recipe domain still blocks (confirm via `sbx policy log <instance>`); confirm global policy is unchanged (D2).

## 2. Template refresh and versioning (kit-aware)

- [ ] 2.1 Document the template refresh/republish flow and keep the `VERSION` + template `IMAGE_VERSION` lockstep (from `sbx-2` D6), noting the live ref source is `version.SbxTemplateRef()` reading `container/sbx/IMAGE_VERSION` (`sbx-3` D10) (D3).
- [ ] 2.2 Define how kits target/pin a template version so kit + template stay compatible, using the version-targeting field from the D1-confirmed kit schema (or a documented convention against the published template tag if the schema offers none).

## 3. Retire the legacy Docker Compose scaffolding and base image (parity-gated)

- [ ] 3.1 Confirm sbx-backend parity before removing anything (gate).
- [ ] 3.2 Remove leftover Compose rendering/helpers and golden/validation test fixtures not already deleted in `sbx-3` D8 (and the `BaseImageRef()` path / any residual `pipelock` package); confirm `compose`, `docker compose`, `dune-base`, and `BaseImageRef` have no live references via grep.
- [ ] 3.3 Remove the legacy `dune-base` Compose image build: root `Dockerfile`, the `container/base/` tree (after the template build no longer needs its assets, `sbx-2` D2), and the `image.yml` base build-and-push job. Keep `version.SbxTemplateRef()` and its unit test intact (`sbx-3` D10).
- [ ] 3.4 Update AGENTS.md / version-bump guidance so the lockstep references the template's `IMAGE_VERSION` (`container/sbx/IMAGE_VERSION`) rather than `container/base/IMAGE_VERSION` (D4).
- [ ] 3.5 Verify after removal: `go build ./cmd/dune` and `go test ./...` pass, and a smoke run (`dune up` → attach → `dune doctor` → `dune down`) against the Dune sbx template works with no `container/base/` tree present (D4).

## 4. Stale local Docker artifact cleanup

- [ ] 4.1 Add an opt-in, list-then-confirm `dune cleanup docker` helper. Register it via the custom parser (`Command` constant + parse case in `internal/dune/cli/options.go`, `Run` case + help text in `internal/dune/app.go`), mirroring the `profile`/`down` wiring. Scope discovery to Dune's exact naming — `dune-persist-<profile>` volumes (`docker volume ls -q -f name=^dune-persist-`), `dune-local-<slug>:latest` images (`docker image ls -q dune-local-*`), and generated compose files at `<XDG_DATA_HOME or ~/.local/share>/dune/projects/<slug>/compose.yaml` (plus the unused `pipelock.yaml`); never remove non-Dune artifacts. Route `docker` calls through the `sbx-3` D5 runner seam (D5).
- [ ] 4.2 Add user-facing migration notes for the one-time transition from the Docker Compose backend.

## 5. Verification and docs

- [ ] 5.1 If a cleanup helper is added, run `go build ./cmd/dune` and `go test ./...`; add fakeRunner tests asserting it (a) selects only `dune-persist-*` / `dune-local-*` / Dune-owned compose paths, (b) never selects a sample of unrelated volumes/images, (c) removes nothing without confirmation (list-then-confirm), and (d) is a clean no-op when no artifacts exist.
- [ ] 5.2 Verify the sbx path still builds and works after the legacy removals (template build, `dune up`, `dune doctor`).
- [ ] 5.3 Update architecture/README docs: kits are the customization layer; the legacy Compose backend and `dune-base` image are retired; the template is the sole runtime artifact.
