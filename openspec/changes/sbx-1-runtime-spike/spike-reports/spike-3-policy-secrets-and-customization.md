# Spike 3: sbx Policy, Secrets, Workspace, and Customization Clarifications
Date: 2026-06-10
Branch: `experiment/docker-sbx`
Status: targeted exploratory spike, not production implementation
## Purpose
This spike tightens details left unclear after `spike-2-sbx-hard-cut-parity-and-security.md` without retreading the template feasibility, nested Docker, Compose, ports, Postgres, Redis, or Mailpit probes.
The specific questions were:
- what the middle-ground `sbx` network preset allows;
- how library documentation sites behave under that preset;
- how easy it is to open custom domains;
- how much more can be learned about secrets lifecycle, especially custom secrets;
- whether a `/workspace` compatibility symlink is viable;
- how Docker's current docs frame templates and kits as the customization story.
## Documentation references checked
Authoritative docs consulted:
- Docker Sandboxes overview: https://docs.docker.com/ai/sandboxes/
- Get started: https://docs.docker.com/ai/sandboxes/get-started/
- Usage guide: https://docs.docker.com/ai/sandboxes/usage/
- Local policy: https://docs.docker.com/ai/sandboxes/governance/local/
- Policy concepts: https://docs.docker.com/ai/sandboxes/governance/concepts/
- Monitoring policies: https://docs.docker.com/ai/sandboxes/governance/monitoring/
- Security model: https://docs.docker.com/ai/sandboxes/security/
- Default security posture: https://docs.docker.com/ai/sandboxes/security/defaults/
- Credentials: https://docs.docker.com/ai/sandboxes/security/credentials/
- Customizing sandboxes: https://docs.docker.com/ai/sandboxes/customize/
- Templates: https://docs.docker.com/ai/sandboxes/customize/templates/
- Kits: https://docs.docker.com/ai/sandboxes/customize/kits/
The docs call the middle preset `Balanced`. It is described as default-deny plus a baseline allowlist covering AI provider APIs, package managers, code hosts, container registries, and common cloud services.
Docs also say the exact active rule set is visible through `sbx policy ls`; the installed CLI did not expose a read-only command to inspect the Balanced preset while another preset was active.
## Environment and cleanup
Observed host state before probing:
- `sbx` was installed at `/usr/bin/sbx`.
- `sbx` was authenticated and usable.
- `sbx ls --json` showed no active sandboxes.
- Current local policy before the spike was:
```text
PROVENANCE   APPLIES_TO   POLICY/RULE         TYPE      DECISION   RESOURCES
local        all          default-allow-all   network   allow      **
```
To capture the Balanced preset, local policy was temporarily reset and switched to Balanced. After the spike, local policy was reset back to Open / `allow-all`, restoring the original policy shape:
```text
PROVENANCE   APPLIES_TO   POLICY/RULE         TYPE      DECISION   RESOURCES
local        all          default-allow-all   network   allow      **
```
Temporary sandbox:
- `dune-sbx-spike3-policy`
- Created with `sbx create --name dune-sbx-spike3-policy shell /home/mitchell/Documents/Mycode/dune`
- Removed after probing with `sbx rm dune-sbx-spike3-policy`
Cleanup caveat:
- Fake custom secrets created during this spike remained listable after sandbox removal.
- This matches the custom-secret lifecycle concern from Spike 2 and is documented below.
## Balanced network policy allowlist
The installed CLI's Balanced preset created these local allow rules:
```text
default-ai-services
default-package-managers
default-code-and-containers
default-cloud-infrastructure
default-os-packages
default-cert-validation
```
High-level shape:
- AI and agent services: OpenAI, ChatGPT, Anthropic, Claude, Gemini, Perplexity, Cursor, Factory, WorkOS, `models.dev`, and related static/CDN domains.
- Package managers and language ecosystems: npm, Yarn, Bun, Python/PyPI, Go proxy/sum/pkg docs, Rust crates/rustup/static assets, RubyGems/Rails/Ruby, Maven, Gradle, NuGet, Packagist, Hex, CPAN, CocoaPods, Swift, Zig, Haskell, NodeSource, Java, Eclipse, Spring, Astral, Sigstore TUF CDN.
- Code and container infrastructure: GitHub, GitHub raw/usercontent/copilot, GitLab, Bitbucket, Docker Hub, Docker domains/CDNs, GHCR, GCR, MCR, Quay, public ECR, Kubernetes registries, Launchpad, SourceForge.
- Cloud/infrastructure/common web tooling: AWS, Google APIs/usercontent/static domains, Azure/Visual Studio, HashiCorp, Vercel, Supabase, Clerk, Figma, Fastly, jsDelivr, unpkg, SchemaStore, JSON Schema, Microsoft login/packages, Playwright Azure Edge, Mise, Prisma binaries, Cloudflare challenge.
- OS packages: Ubuntu, Debian, Alpine, Fedora, CentOS, Arch, LLVM apt, Packagecloud.
- Certificate validation: common OCSP/CRL/CA endpoints for DigiCert, Let's Encrypt, Google PKI, Microsoft PKI, Amazon Trust, Sectigo, GlobalSign, UserTrust, Comodo.
Representative exact entries observed:
```text
api.anthropic.com:443
api.openai.com via **.openai.com:443
chatgpt.com:443
claude.com:443
gemini.google.com:443
generativelanguage.googleapis.com:443
registry.npmjs.org:443
pypi.org:443
files.pythonhosted.org:443
pkg.go.dev:443
proxy.golang.org:443
sum.golang.org:443
crates.io:443
index.crates.io:443
static.crates.io:443
repo.maven.apache.org:443
github.com:443
**.githubusercontent.com:443
gitlab.com:443
ghcr.io:443
docker.io:443
**.docker.io:443
mcr.microsoft.com:443
quay.io:443
public.ecr.aws:443
playwright.azureedge.net:443
mise.run:443
mise-versions.jdx.dev:443
jsdelivr.net:443
unpkg.com:443
json.schemastore.org:443
archive.ubuntu.com:80
security.ubuntu.com:80
```
Important interpretation:
- Balanced is developer-infrastructure oriented, not general web browsing.
- It includes broad infrastructure wildcards such as `**.amazonaws.com:443`, `**.googleapis.com:443`, `**.googleusercontent.com:443`, `**.gstatic.com:443`, and `**.githubusercontent.com:443`.
- It does not generally include documentation sites just because they are popular or developer-relevant.
## Documentation-site probes under Balanced
The sandbox tested representative docs/library URLs with `curl -L -sS --max-time 12 -o /dev/null -w "%{http_code}"`.
Observed allowed:
```text
https://pkg.go.dev/std -> 200
```
Observed blocked by sbx policy with HTTP `403`:
```text
https://docs.python.org/3/
https://go.dev/doc/
https://doc.rust-lang.org/book/
https://docs.rs/serde/latest/serde/
https://react.dev/reference/react
https://vuejs.org/guide/introduction.html
https://svelte.dev/docs/svelte/overview
https://tailwindcss.com/docs/installation/using-vite
https://fastapi.tiangolo.com/
https://htmx.org/docs/
https://docs.djangoproject.com/en/stable/
https://expressjs.com/
https://laravel.com/docs
https://lodash.com/docs/
https://example.org/
```
Policy logs showed the blocked hosts with:
```text
No matching allow rule (default deny)
```
Implication:
- Balanced does not satisfy the common agent workflow of reading arbitrary library docs.
- Dune should expect users or kits to add explicit allow rules for project-specific docs.
- Popularity does not imply access: React, Vue, Svelte, Tailwind, Django, Laravel, Express, Lodash, FastAPI, docs.rs, docs.python.org, and go.dev docs were blocked in this probe.
- Some documentation attached to package infrastructure is available, such as `pkg.go.dev`, because it is in the package-manager allowlist.
## Opening custom domains
The installed CLI uses `--sandbox` for scoped policy rules:
```bash
sbx policy allow network --sandbox dune-sbx-spike3-policy example.org:443
```
The positional syntax shown in some docs/examples failed locally:
```text
ERROR: unexpected second argument "example.org:443": the sandbox name is no longer positional.
To scope this rule to a sandbox, use:
  sbx policy allow network --sandbox dune-sbx-spike3-policy example.org:443
```
Exact-domain behavior:
```text
after exact allow: https://example.org/ -> 200
after exact allow: https://www.example.org/ -> 403
```
Wildcard behavior after adding `*.example.org:443`:
```text
after wildcard allow: https://example.org/ -> 200
after wildcard allow: https://www.example.org/ -> 200
```
This matches the documented rule model:
- `example.org` and `*.example.org` do not cover each other.
- Specify both root and wildcard forms when both are needed.
- Port-specific rules work.
- Sandbox-scoped allow rules take effect immediately.
Recommendation for Dune:
- Provide a thin command or guidance around adding per-sandbox domains, because this is likely necessary for docs-heavy agent workflows.
- Do not imply that Balanced allows arbitrary docs.
- When adding docs access, prefer exact domains plus specific wildcard domains instead of broad catch-alls.
## Secrets lifecycle probes
All values used in this spike were fake.
### Normal service secret lifecycle
Command shape tested:
```bash
sbx secret set dune-sbx-spike3-policy dune-spike3-service -t DUNE_SPIKE3_FAKE_SERVICE_SECRET
sbx secret ls dune-sbx-spike3-policy
sbx secret rm dune-sbx-spike3-policy dune-spike3-service -f
```
Observed:
- service secret set succeeded;
- listing showed the service secret, masked;
- `sbx secret rm` deleted it;
- listing afterward no longer showed it.
Implication:
- Stored service secrets have a clean basic lifecycle.
- This supports using service secrets where a built-in agent or future kit declares a service identifier.
### Custom secret lifecycle
Command shape tested:
```bash
sbx secret set-custom dune-sbx-spike3-policy \
  --host httpbin.org \
  --env DUNE_SPIKE3_FAKE_API_KEY \
  --placeholder dune-spike3-placeholder \
  --value DUNE_SPIKE3_FAKE_SECRET_VALUE
```
Observed after setting:
```text
Saved custom secret placeholder "dune-spike3-placeholder" for target "httpbin.org" env "DUNE_SPIKE3_FAKE_API_KEY" in scope "dune-sbx-spike3-policy"
You may need to update environment variable DUNE_SPIKE3_FAKE_API_KEY inside existing sandboxes to "dune-spike3-placeholder" (the placeholder value).
Applied secret updates for sandbox "dune-sbx-spike3-policy"
```
Listing showed the custom secret under a separate `CUSTOM SECRETS` section.
Environment behavior:
- `printenv DUNE_SPIKE3_FAKE_API_KEY` produced no value in the running sandbox.
- After sandbox stop/start, `printenv DUNE_SPIKE3_FAKE_API_KEY` still produced no value.
- The CLI warning says the user may need to update the environment variable inside existing sandboxes; in this probe, sbx did not make the variable visible automatically.
Proxy substitution behavior:
- A request to `https://httpbin.org/headers` with header `X-Dune-Test: dune-spike3-placeholder` returned `X-Dune-Test: DUNE_SPIKE3_FAKE_SECRET_VALUE`.
- This proves the host-side proxy can replace a fake custom placeholder in outbound traffic to the configured host.
Removal behavior tested:
```bash
sbx secret rm dune-sbx-spike3-policy httpbin.org -f
sbx secret rm dune-sbx-spike3-policy DUNE_SPIKE3_FAKE_API_KEY -f
sbx secret rm dune-sbx-spike3-policy httpbin.org:DUNE_SPIKE3_FAKE_API_KEY -f
```
Observed:
- each command reported no matching service secret;
- each command exited successfully;
- none removed the custom secret.
Overwrite behavior:
- Setting another custom secret with the same scope, target, and env but a different placeholder/value created a second row.
- It did not overwrite the first row.
After sandbox removal:
```text
CUSTOM SECRETS
SCOPE                    TARGET        ENV                        PLACEHOLDER                  SECRET
dune-sbx-spike2-shell    httpbin.org   DUNE_FAKE_API_KEY          dune-fake-...                DUNE_F******...******ONLY
dune-sbx-spike3-policy   httpbin.org   DUNE_SPIKE3_FAKE_API_KEY   dune-spike3-placeholder      DUNE_S******...******ALUE
dune-sbx-spike3-policy   httpbin.org   DUNE_SPIKE3_FAKE_API_KEY   dune-spike3-placeholder-2    DUNE_S******...******UE_2
```
Implications:
- Custom secrets work for proxy replacement when the placeholder is deliberately sent to the target host.
- Custom secrets are not ready to be the first Dune-managed lifecycle primitive.
- The installed CLI does not expose a working removal path for custom secrets.
- Sandbox-scoped custom secrets are not removed when the sandbox is removed.
- Re-setting a custom secret for the same scope/target/env can accumulate multiple placeholders.
- Custom secrets are documented as experimental and hidden from normal help flows; Dune should treat them as future capability, not v1 hard-cut dependency.
Recommendation:
- Prefer service-identifier secrets through built-in agents or future kits.
- Avoid Dune-managed custom secrets until custom removal/update semantics are documented and tested.
- If custom secrets are used experimentally, Dune should warn that cleanup may require manual Docker/sbx intervention or a future sbx fix.
## `/workspace` compatibility symlink
Under a generic shell sandbox, `/workspace` was missing. The direct-mounted repository was available at the absolute host path.
Test:
```bash
sudo rm -f /workspace
sudo ln -s /home/mitchell/Documents/Mycode/dune /workspace
readlink -f /workspace
cd /workspace && pwd -P && git rev-parse --show-toplevel
```
Observed:
```text
/home/mitchell/Documents/Mycode/dune
/home/mitchell/Documents/Mycode/dune
/home/mitchell/Documents/Mycode/dune
```
Implication:
- A `/workspace` symlink is viable as a short-term compatibility layer when using direct mount.
- Dune can still attach shells in the real mounted repo path while also providing `/workspace` for legacy assumptions.
- The template should create the symlink deliberately; relying on image `WORKDIR /workspace` is not enough and can be actively misleading if `/workspace` is not the mounted repo.
## Templates and kits direction from docs
The docs support the emerging Dune model:
- Templates are reusable sandbox images with packages, tools, and large dependencies baked in.
- Templates can extend Docker's built-in sandbox template variants.
- Each built-in variant has a `-docker` form that includes a full Docker Engine inside the sandbox.
- `sbx template load` can import a tar into the sandbox runtime's image store.
- `sbx template save` can capture a configured sandbox, but docs warn that saving a sandbox captures the whole filesystem, including manually stored secrets.
- For non-Docker Hub registries such as GHCR, template pulls may require `sbx secret set --registry`.
- Kits are experimental YAML artifacts that can layer commands, environment variables, files, network rules, credentials, and memory on top of templates.
- Agent kits can define a full agent, including `agent.image`.
- Mixin kits can extend an existing agent.
- Kit network rules and credential wiring are a good conceptual fit for future per-project/team customization.
- Docs explicitly say templates and kits work together: a heavy template provides the base environment, and thin kits layer config, credentials, network rules, and runtime behavior.
Dune interpretation:
- The Dune sbx template becomes the new durable equivalent of today's base image / practical Dockerfile layer.
- `Dockerfile.dune` should not be carried forward as the first customization model.
- Kits are the natural future customization story once the Dune template/runtime is stable.
- Dune should avoid baking secrets into saved templates and should document that templates are not a secret boundary.
## Updated recommendations after Spike 3
1. Proceed with drafting implementation plans.
2. Treat the dedicated Dune sbx template as the new base runtime artifact.
3. Use a `/workspace` symlink as a compatibility bridge while making the real mounted repo path the canonical path.
4. Do not over-invest in Postgres/Redis/Mailpit health checks as first-class in-template services if the product direction is to move those app dependencies toward project-owned Docker Compose.
5. Keep `dune logs` focused on sbx lifecycle/policy logs plus Dune-owned setup/runtime logs; project service logs can increasingly belong to project Compose.
6. Use Balanced as a reasonable starting point only if users accept that arbitrary documentation sites are blocked.
7. Add a Dune affordance or clear docs for opening project-specific custom domains, especially documentation sites.
8. Prefer service secrets and kit-declared service identifiers over custom secrets.
9. Treat custom secrets as experimental: useful capability, proven proxy substitution, but unproven/poor lifecycle.
10. Keep kits as the planned customization story after hard-cut parity, especially for network rules, credentials, files, and per-team/project additions.
## Remaining unknowns
- Whether Docker can provide a non-destructive way to inspect the Balanced preset without switching the active local policy.
- Whether custom-secret removal has an undocumented command form or is currently missing/buggy in `sbx v0.32.0`.
- Whether future sbx releases will automatically inject custom secret placeholders into sandbox envs, or whether users/kits must always wire those env vars themselves.
- Whether broad Balanced wildcards such as `**.amazonaws.com`, `**.googleapis.com`, and `**.githubusercontent.com` are acceptable for Dune's recommended default posture.
- How Dune should surface domain-opening UX: direct `sbx policy allow` guidance, `dune policy allow`, kit-authored domain rules, or some combination.
- How Dune should version and refresh its sbx template, especially when kits later become the customization layer.
## Commands run during this spike
Representative commands:
```bash
sbx policy --help
sbx policy set-default --help
sbx policy profile --help
sbx policy ls
sbx policy reset --force
sbx create --name dune-sbx-spike3-policy shell /home/mitchell/Documents/Mycode/dune
sbx exec dune-sbx-spike3-policy bash -lc 'curl ...'
sbx policy allow network --sandbox dune-sbx-spike3-policy example.org:443
sbx policy allow network --sandbox dune-sbx-spike3-policy '*.example.org:443'
sbx policy rm network --sandbox dune-sbx-spike3-policy --resource example.org:443
sbx policy rm network --sandbox dune-sbx-spike3-policy --resource '*.example.org:443'
sbx policy allow network --sandbox dune-sbx-spike3-policy httpbin.org:443
sbx secret set-custom dune-sbx-spike3-policy --host httpbin.org --env DUNE_SPIKE3_FAKE_API_KEY --placeholder dune-spike3-placeholder --value <fake value>
sbx secret ls dune-sbx-spike3-policy
sbx exec dune-sbx-spike3-policy bash -lc 'printenv DUNE_SPIKE3_FAKE_API_KEY || true'
sbx exec dune-sbx-spike3-policy bash -lc 'curl -H "X-Dune-Test: dune-spike3-placeholder" https://httpbin.org/headers'
sbx secret rm dune-sbx-spike3-policy httpbin.org -f
sbx secret rm dune-sbx-spike3-policy DUNE_SPIKE3_FAKE_API_KEY -f
sbx secret set dune-sbx-spike3-policy dune-spike3-service -t <fake value>
sbx secret rm dune-sbx-spike3-policy dune-spike3-service -f
sbx exec dune-sbx-spike3-policy bash -lc 'sudo ln -s /home/mitchell/Documents/Mycode/dune /workspace'
sbx rm dune-sbx-spike3-policy
sbx policy reset --force
sbx policy set-default allow-all
```
Final cleanup status:
- temporary sandbox removed;
- local policy restored to `allow-all`;
- fake custom secrets remained listable after sandbox removal because no tested `sbx secret rm` form removed them.
