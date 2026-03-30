## ADDED Requirements

### Requirement: Pipelock sidecar proxies all agent HTTP traffic
The agent container SHALL have no direct internet access. All HTTP and HTTPS traffic from the agent container SHALL be routed through the Pipelock sidecar via `http_proxy` and `https_proxy` environment variables pointing to `pipelock:8888`. The agent container SHALL connect only to an internal Docker network. The Pipelock container SHALL connect to both the internal network and an external network with internet access.

#### Scenario: Agent makes an HTTPS request to an allowed API
- **WHEN** a tool in the agent container makes an HTTPS request to `api.anthropic.com`
- **THEN** the request is proxied through Pipelock and reaches the destination successfully

#### Scenario: Agent attempts direct internet access bypassing proxy
- **WHEN** a tool in the agent container attempts a direct connection to an external host without using the proxy
- **THEN** the connection fails because the agent container has no route to the external network

#### Scenario: Pipelock container restarts
- **WHEN** the Pipelock container crashes or is killed
- **THEN** Docker Compose restarts it automatically via the `restart: unless-stopped` policy
- **THEN** agent HTTP traffic resumes once Pipelock is back

### Requirement: Pipelock runs in balanced mode with enforcement
Pipelock SHALL run with config fields `version: 1`, `mode: balanced`, `enforce: true`. Response scanning SHALL be enabled with `response_scanning.enabled: true` and `response_scanning.action: warn` (log, do not block). DLP SHALL use `dlp.include_defaults: true` to enable the 46 built-in secret detection patterns (covering Anthropic API keys, AWS access keys, GitHub tokens, and more) without hand-written regex.

#### Scenario: Agent request contains an API key in the body
- **WHEN** an outbound request body contains a string matching a DLP pattern (e.g. `sk-ant-`)
- **THEN** Pipelock logs a DLP warning and blocks the request

#### Scenario: Normal API request with authorization header
- **WHEN** an outbound request uses a standard `Authorization` header to authenticate with an API
- **THEN** Pipelock allows the request (authorization headers are not treated as exfiltration)

### Requirement: Core domains are allowlisted in Pipelock config
The Pipelock configuration SHALL include an `api_allowlist` with core domains that MUST NOT be blocked by heuristics. The allowlist SHALL use wildcard syntax where appropriate and include at minimum: `*.anthropic.com`, `*.openai.com`, `*.googleapis.com`, `accounts.google.com`, `oauth2.googleapis.com`, `chatgpt.com`, `registry.npmjs.org`, `pypi.org`, `files.pythonhosted.org`, `proxy.golang.org`, `crates.io`, `mcp.grep.app`, `mcp.context7.com`, `mcp.exa.ai`.

#### Scenario: Request to an allowlisted domain
- **WHEN** the agent makes a request to `registry.npmjs.org`
- **THEN** the request is allowed without heuristic evaluation

#### Scenario: Request to an unknown domain
- **WHEN** the agent makes a request to `example.com`
- **THEN** Pipelock evaluates the request using balanced-mode heuristics (may allow or block depending on content)

### Requirement: Known exfiltration targets are blocklisted
The Pipelock configuration SHALL blocklist known exfiltration targets via `fetch_proxy.monitoring.blocklist` including `*.pastebin.com`, `*.hastebin.com`, `*.transfer.sh`, `file.io`, `requestbin.net`, and similar paste/file-sharing services.

#### Scenario: Agent attempts to POST to a blocklisted domain
- **WHEN** the agent sends a POST request to `pastebin.com`
- **THEN** Pipelock blocks the request and logs the attempt

### Requirement: Pipelock config is globally managed
The Pipelock configuration file SHALL be stored at `~/.config/dune/pipelock.yaml`. On first run, dune SHALL generate the baseline config by running `docker run --rm ghcr.io/luckypipewrench/pipelock:<pinned-tag> generate config --preset balanced`, then apply customisations (api_allowlist, blocklist, logging) and write the result. The config file SHALL be mounted read-only into the Pipelock container. Pipelock supports hot-reload via file watcher, so config edits take effect without container restart.

#### Scenario: First run with no existing config
- **WHEN** a user runs `dune` for the first time and `~/.config/dune/pipelock.yaml` does not exist
- **THEN** dune generates the baseline from `pipelock generate config --preset balanced`, applies customisations, and writes the file

#### Scenario: User edits Pipelock config
- **WHEN** a user modifies `~/.config/dune/pipelock.yaml` and runs `dune down && dune up`
- **THEN** the Pipelock container uses the updated config

### Requirement: Pipelock logs to stdout in JSON format
Pipelock SHALL log all request activity to stdout in JSON format. Logs SHALL be viewable via `dune logs pipelock` (which maps to `docker compose logs pipelock`).

#### Scenario: Viewing proxy request logs
- **WHEN** a user runs `dune logs pipelock`
- **THEN** they see JSON-formatted log entries showing proxied requests, blocked requests, and DLP warnings

### Requirement: Rate limiting is enabled
Pipelock SHALL enforce rate limiting via `fetch_proxy.monitoring.max_requests_per_minute` to prevent runaway agents from making excessive requests. The default rate limit SHALL be set to a reasonable value for AI agent workloads (e.g., 60 requests per minute per domain).

#### Scenario: Agent exceeds rate limit
- **WHEN** the agent makes requests at a rate exceeding the configured limit
- **THEN** Pipelock temporarily throttles requests and logs a rate-limit warning
