## ADDED Requirements

### Requirement: Profiles use string names
Profiles SHALL be identified by string names (e.g. `default`, `work`, `personal`) instead of single-character IDs. Profile names SHALL be validated as non-empty strings containing only lowercase alphanumeric characters and hyphens.

#### Scenario: Valid profile name
- **WHEN** a user runs `dune --profile my-project`
- **THEN** dune uses the profile named `my-project`

#### Scenario: Invalid profile name
- **WHEN** a user runs `dune --profile "My Project!"`
- **THEN** dune exits with an error indicating the profile name is invalid

### Requirement: Default profile is named "default"
When no profile is specified via `--profile`/`-p` and no directory mapping exists, dune SHALL use the profile named `default`.

#### Scenario: No profile specified and no mapping
- **WHEN** a user runs `dune` in a directory with no profile mapping configured
- **THEN** dune uses the `default` profile

### Requirement: Profile selection via CLI flag
The dune CLI SHALL accept `--profile <name>` or `-p <name>` on all commands that operate on a container (up, down, rebuild, logs). The flag SHALL override any stored directory-to-profile mapping.

#### Scenario: Explicit profile flag
- **WHEN** a user runs `dune --profile work`
- **THEN** dune uses the `work` profile regardless of any stored mapping for the current directory

### Requirement: Directory-to-profile mapping is stored centrally
The dune CLI SHALL store a mapping of directory paths to profile names in `~/.config/dune/profiles.json`. The `dune profile set <name>` command SHALL update this mapping for the current directory. The `--profile` flag SHALL take precedence over stored mappings.

#### Scenario: Setting a profile for a directory
- **WHEN** a user runs `dune profile set work` in `/home/user/projects/myapp`
- **THEN** `~/.config/dune/profiles.json` records that `/home/user/projects/myapp` maps to profile `work`
- **THEN** subsequent `dune` commands in that directory use the `work` profile

#### Scenario: CLI flag overrides stored mapping
- **WHEN** a directory is mapped to profile `work`
- **WHEN** the user runs `dune --profile personal` in that directory
- **THEN** dune uses the `personal` profile for this invocation (the stored mapping is not changed)

### Requirement: Profile list shows all mappings
The `dune profile list` command SHALL display all stored directory-to-profile mappings and indicate which profile would be used for the current directory.

#### Scenario: Listing profiles
- **WHEN** a user runs `dune profile list`
- **THEN** the output shows all directory-to-profile mappings
- **THEN** the current directory's effective profile is highlighted

### Requirement: Each profile has its own Docker volume
Each profile SHALL have a dedicated named Docker volume `dune-persist-<profile>` that is mounted at `/persist/agent` in the agent container. Specific credential and config paths are symlinked from the agent's home directory into the persistent volume (see base-image spec for the symlink list). Different profiles SHALL NOT share persist volume state.

#### Scenario: Profile isolation
- **WHEN** a user has credentials configured under profile `work`
- **WHEN** the user switches to profile `personal`
- **THEN** the `personal` profile's container has a separate persist volume without the `work` profile's credentials

### Requirement: Each profile has its own compose project
Each profile-directory combination SHALL use a distinct Docker Compose project name (`dune-<slug>-<profile>`) to ensure containers for different profiles do not collide.

#### Scenario: Multiple profiles for the same directory
- **WHEN** a user runs `dune --profile work` in `/projects/myapp`
- **WHEN** the user runs `dune --profile test` in `/projects/myapp` (in another terminal)
- **THEN** two independent container sets are running with different compose project names
