## ADDED Requirements

### Requirement: Rally is published via GoReleaser to GitHub Releases
Rally SHALL be built and released using GoReleaser. Each tagged release SHALL produce cross-platform binaries for linux/amd64, linux/arm64, darwin/amd64, and darwin/arm64. Binaries SHALL be published as GitHub Release assets on the `mitchell-wallace/rally` repository. Archives SHALL be named `rally_<os>_<arch>.tar.gz`.

#### Scenario: New Rally release
- **WHEN** a version tag (e.g. `v1.0.0`) is pushed to the Rally repo
- **THEN** GitHub Actions runs GoReleaser
- **THEN** release assets include `rally_linux_amd64.tar.gz`, `rally_linux_arm64.tar.gz`, `rally_darwin_amd64.tar.gz`, `rally_darwin_arm64.tar.gz`, and checksums

### Requirement: Install script downloads latest release
An `install.sh` script SHALL be published as a release asset. The script SHALL detect the current OS and architecture, download the appropriate binary from the latest GitHub Release, and install it to `~/.local/bin/rally`. The script SHALL NOT require sudo.

#### Scenario: Installing Rally on Linux amd64
- **WHEN** a user runs `curl -fsSL <install-url> | sh` on a Linux amd64 machine
- **THEN** Rally is installed to `~/.local/bin/rally`
- **THEN** the script prints the installed version

#### Scenario: Installing Rally on macOS arm64
- **WHEN** a user runs the install script on macOS with Apple Silicon
- **THEN** Rally is installed to `~/.local/bin/rally` using the `darwin_arm64` binary

### Requirement: Rally version is embedded at build time
The Rally binary SHALL embed its version at build time via Go linker flags (`-X main.Version={{.Version}}`). Running `rally --version` SHALL print the embedded version. Development builds SHALL show version `dev`.

#### Scenario: Release build shows version
- **WHEN** a user runs `rally --version` with a release build
- **THEN** the output shows the release version (e.g. `rally v1.2.3`)

#### Scenario: Development build shows dev version
- **WHEN** a user runs `rally --version` with a locally compiled build
- **THEN** the output shows `rally dev`

### Requirement: rally update self-updates to latest release
Rally SHALL include a `rally update` subcommand that downloads the latest release binary from GitHub Releases and replaces the current binary at its install location (`~/.local/bin/rally`). The command SHALL NOT require sudo.

#### Scenario: Updating Rally to latest version
- **WHEN** a user runs `rally update` and a newer version is available
- **THEN** the new binary is downloaded and replaces the current one
- **THEN** the output confirms the update with old and new version numbers

#### Scenario: Rally is already up to date
- **WHEN** a user runs `rally update` and the current version matches the latest release
- **THEN** the output indicates Rally is already up to date

### Requirement: Background version check on startup
Rally SHALL check for newer versions in the background on startup by querying the GitHub Releases API. If a newer version is available, Rally SHALL print a one-line notice to stderr (e.g. `A new version of Rally is available: v1.3.0. Run 'rally update' to upgrade.`). The check SHALL be non-blocking and SHALL NOT delay Rally startup. The check SHALL be suppressible via `RALLY_NO_UPDATE_CHECK=1` environment variable.

#### Scenario: Newer version available
- **WHEN** Rally starts and a newer version exists on GitHub Releases
- **THEN** a one-line update notice is printed to stderr after the main command output

#### Scenario: Update check suppressed
- **WHEN** `RALLY_NO_UPDATE_CHECK=1` is set in the environment
- **THEN** no version check is performed and no update notice is printed

### Requirement: Rally code is extracted from the dune repo
All Rally source code (`cmd/rally`, `internal/rally`) SHALL be moved to the `mitchell-wallace/rally` GitHub repository. The `internal/contracts/rally` package SHALL be removed entirely — Rally is installed into the container like any other tool (via GitHub Releases) and dune has no build-time or runtime dependency on Rally types. The dune repo SHALL NOT contain Rally source code after extraction.

#### Scenario: Clean separation
- **WHEN** the extraction is complete
- **THEN** the dune repo contains no Rally Go source files and no `internal/contracts/rally` package
- **THEN** the rally repo builds independently with `go build ./cmd/rally`

### Requirement: Rally reads and writes configuration via rally.toml
Rally SHALL read model preferences (`claude_model`, `codex_model`, `gemini_model`, `opencode_model`) and beads configuration from `~/.config/rally/rally.toml`. Rally SHALL also write to this file — in particular, `rally init` SHALL write the `beads` setting to `~/.config/rally/rally.toml` (replacing the current behaviour of writing to `dune.toml`). This replaces the previous approach of receiving values via environment variables from `dune.toml`. The file format is TOML with the same key names previously used in `dune.toml`.

#### Scenario: Rally reads model config
- **WHEN** `~/.config/rally/rally.toml` contains `claude_model = "opus"`
- **THEN** Rally uses `opus` as the Claude model preference

#### Scenario: rally init writes config
- **WHEN** a user runs `rally init` and selects beads = true
- **THEN** `~/.config/rally/rally.toml` contains `beads = "true"`
- **THEN** `dune.toml` is NOT written to or created

#### Scenario: No rally.toml exists
- **WHEN** `~/.config/rally/rally.toml` does not exist
- **THEN** Rally uses default model settings

### Requirement: GoReleaser CI workflow
The Rally repo SHALL include a GitHub Actions workflow (`.github/workflows/release.yml`) that runs `goreleaser release --clean` on tag pushes matching `v*`. The workflow SHALL handle checksums, archives, and GitHub Release creation.

#### Scenario: Tag push triggers release
- **WHEN** a tag `v1.0.0` is pushed to the Rally repo
- **THEN** GitHub Actions builds binaries for all target platforms
- **THEN** a GitHub Release is created with all assets attached

### Requirement: Dune container installs Rally from releases
The dune base image Dockerfile SHALL install Rally by running the install script from the latest GitHub Release. This replaces the previous approach of syncing a host-built binary into the container.

#### Scenario: Container build installs Rally
- **WHEN** the base image is built
- **THEN** Rally is installed at `/home/agent/.local/bin/rally`
- **THEN** `rally --version` works inside the container
