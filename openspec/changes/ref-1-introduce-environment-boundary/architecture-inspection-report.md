# Dune Architecture Inspection Report

---

## 1. Current Command Flow

| Command | Entry Point | Key Functions Called | Side Effects |
|---------|-------------|---------------------|--------------|
| `dune` (default) | `cmd/dune/main.go:12` → `dune.Run()` | `cli.Parse` → `workspace.Resolve` → `loadProfileStore` → `resolveProfile` → `validateDockerPrerequisites` → `ensurePipelockConfig` → `ensureComposeFile` → `ensureVolume` → `isAgentRunning` → `prepareAgentImage` → `composeUp` → `runStreaming exec` | All Docker calls + all filesystem writes |
| `dune up` | Same as default, command=CommandUp | Identical flow | Identical |
| `dune down` | `app.go:145` | `validateDockerPrerequisites` → `ensureComposeFile` → `runStreaming down` | `docker compose down` |
| `dune rebuild` | `app.go:155-171` | All `up` steps + `prepareAgentImage(noCache=true)` | `docker compose up -d --force-recreate` |
| `dune logs` | `app.go:146-154` | `validateDockerPrerequisites` → `ensureComposeFile` → `runStreaming logs -f` | `docker compose logs -f [service]` |
| `dune profile set` | `app.go:99-108` | `workspace.Resolve` → `loadProfileStore` → `saveProfileStore` | Writes `~/.config/dune/profiles.json` |
| `dune profile list` | `app.go:109-111` | `loadProfileStore` → `printProfileList` | None (read-only) |
| `dune version` | `app.go:96-98` | None | Prints version string |

---

## 2. Docker/Compose Call Sites

| File | Function | Command Shape | Capture? |
|------|----------|---------------|----------|
| `app.go:405` | `validateDockerPrerequisites` | `docker compose version` | Yes (`capture`) |
| `app.go:408` | `validateDockerPrerequisites` | `docker info` | Yes (`capture`) |
| `app.go:269` | `ensurePipelockConfig` | `docker run --rm <image> generate config --preset balanced` | Yes (`capture`) |
| `app.go:335` | `validateComposeFile` | `docker compose -f <path> -p <project> config` | Yes (`capture`) |
| `app.go:348` | `ensureVolume` | `docker volume create <name>` | Yes (`capture`) |
| `app.go:356` | `prepareAgentImage` | `docker pull <baseImage>` | Yes (`runStreaming`) |
| `app.go:367-372` | `prepareAgentImage` | `docker compose -f <path> -p <project> build [--no-cache] agent` | Yes (`runStreaming`) |
| `app.go:379` | `composeUp` | `docker compose -f <path> -p <project> up -d` | Yes (`capture`) |
| `app.go:380` | `composeUp` (on error) | `docker compose logs --tail 60` | Yes (`capture`) |
| `app.go:390` | `isAgentRunning` | `docker compose -f <path> -p <project> ps --status running --services agent` | Yes (`capture`) |
| `app.go:145,153,171,198` | `runStreaming` variants | `docker compose down/logs/up/exec` + `docker exec` | No (full TTY) |

---

## 3. File-System Side Effects

| Path | Read/Write | Functions |
|------|------------|------------|
| `~/.config/dune/profiles.json` | RW | `loadProfileStore`, `saveProfileStore` (app.go:449-481) |
| `~/.local/share/dune/projects/<slug>/compose.yaml` | W | `ensureComposeFile` (app.go:291-324) |
| `~/.config/dune/pipelock.yaml` | RW | `ensurePipelockConfig` (app.go:258-289) |
| `/tmp/dune/projects/<slug>/compose-*.yaml` | W (temp) | `ensureComposeFile` (app.go:300-307) |
| `<workspace>/Dockerfile.dune` | R | `fileExists` at app.go:128 |
| Docker volume `dune-persist-<profile>` | Create | `ensureVolume` (app.go:347-352) |

---

## 4. Current Planning Data

All computed in `app.go:118-136` (project struct construction):

- **WorkspaceRoot**: from `workspace.Resolve().Root`
- **WorkspaceSlug**: from `workspace.Resolve().Slug` (SHA1 hash of root + sanitized basename)
- **Profile**: from CLI arg, stored mapping, or "default"
- **ComposeProject**: `fmt.Sprintf("dune-%s-%s", ws.Slug, profile)`
- **ComposeDir**: `~/.local/share/dune/projects/<slug>`
- **ComposePath**: `~/.local/share/dune/projects/<slug>/compose.yaml`
- **PersistVolume**: `dune-persist-<profile>`
- **BaseImage**: `version.BaseImageRef()` (hardcoded in version package)
- **AgentImage**: `version.BaseImageRef()` unless `UseBuild`, then `dune-local-<slug>:latest`
- **UseBuild**: `fileExists(ws.Root + "/Dockerfile.dune")`
- **PipelockImage**: `ghcr.io/luckypipewrench/pipelock:2.0.0`
- **PipelockConfigPath**: `~/.config/dune/pipelock.yaml`
- **TZ**: `os.Getenv("TZ")` or "UTC"

---

## 5. Test Seams

| Test | Fake Strategy | Brittleness |
|------|---------------|-------------|
| `TestRenderComposeFileGolden` | Golden file at `testdata/compose.golden.yaml` | String equality is fragile; missing schema validation |
| `TestRunUsesSampleProjectFixtureForDockerfileWorkflow` | Shell shim at `$PATH/docker` writing to log file | Doesn't capture exit codes, signals, PTY behavior |
| `TestPrepareAgentImageReportsProgress` | Shell shim | Same shell shim fragility |
| `TestEnsurePipelockConfigReconcilesExistingConfig` | Shell shim (expects NO docker calls) | Relies on specific call count detection |
| `TestRenderComposeFilePassesDockerComposeConfig` | Real docker required | Skips if docker missing |
| `workspace_test.go` | Temp dirs + git init | Depends on git being available |
| `pipelock_test.go` | Reads real YAML from `testdata/balanced-2.0.0.yaml` | Fixture dependency |

**Fixture paths used**: `test/fixtures/sample-project/Dockerfile.dune`, `pipelock/testdata/balanced-2.0.0.yaml`

---

## 6. Suggested Extraction Map

### project/workspace resolution

- `workspace.Resolve(input)` → `workspace/workspace.go:18`
- `workspace.ResolveRoot(directory)` → `workspace/workspace.go:48` (shells to `git rev-parse --show-toplevel`)
- `workspace.Slug(root)` → `workspace/workspace.go:72` (SHA1-based)

### plan builder

- `project` struct construction → `app.go:118-136`
- `resolveProfile(opts, ws.Root, store)` → `app.go:204-215`
- `renderComposeFile(proj)` → `app.go:326-332`
- `ensureComposeFile(ctx, proj)` → `app.go:291-324`
- Volume name derivation: `dune-persist-<profile>`

### dockercompose backend

- `validateDockerPrerequisites(ctx)` → `app.go:401-412`
- `ensurePipelockConfig(ctx, path)` → `app.go:258-289`
- `validateComposeFile(ctx, proj, path)` → `app.go:334-345`
- `ensureVolume(ctx, name)` → `app.go:347-352`
- `prepareAgentImage(ctx, proj, noCache, stdout, stderr)` → `app.go:354-376`
- `composeUp(ctx, proj, stderr)` → `app.go:378-387`
- `isAgentRunning(ctx, proj)` → `app.go:389-399`
- `runStreaming(ctx, dir, stdout, stderr, name, args...)` → `app.go:437-447`
- `capture(ctx, dir, name, args...)` → `app.go:424-435`
- `composeArgs(proj, args...)` → `app.go:414-417`

### command runner

- `cli.Parse(argv)` → `cli/options.go:32-53`
- Profile persistence: `loadProfileStore`, `saveProfileStore` → `app.go:449-481`

### diagnostics/errors

- No existing dedicated diagnostics module — error wrapping is ad-hoc throughout (`fmt.Errorf("...: %w", err)`)

---

## 7. Risks

| Behavior | Location | Risk |
|----------|----------|------|
| **Profile persistence** | `app.go:89,104` — JSON file at `~/.config/dune/profiles.json` | No file locking; concurrent writes could corrupt |
| **Slug stability** | `workspace/workspace.go:72-79` — SHA1 of absolute path | Moving project changes slug, orphaning compose.yaml |
| **Compose path stability** | `app.go:123-124` — depends on slug + XDG_DATA_HOME | If XDG_DATA_HOME changes, old compose.yaml orphaned |
| **Pipelock config generation** | `app.go:258-289` — merges baseline + customizations | YAML deep merge is subtle; losing existing user config |
| **Local Dockerfile.dune builds** | `app.go:354-376` — `docker compose build` | No build caching strategy; slow rebuilds |
| **Shell attach** | `app.go:198` — `docker exec agent zsh` | Hardcoded `zsh`; broken if zsh not in image |
| **Logs streaming** | `app.go:153` — `docker compose logs -f` | Blocks indefinitely; no timeout |
| **Docker prerequisite checks** | `app.go:401-412` | Three separate docker invocations before any work |