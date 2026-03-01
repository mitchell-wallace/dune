

## `./base`

from https://github.com/anthropics/claude-code/tree/main/.devcontainer

A container for running Claude Code in.

## `./updated`

Changes:
- node 22
- switch claude code to native installer
- add oh-my-claudecode
- add codex installer
- add gemini-cli installer
- switch devcontainer vscode to biome

How to use:

Run `install-sand-alias.sh`, then from any folder run `sand` to (if needed) create a new container for that folder, and then start an interactive session

Build with:
```sh
npx @devcontainers/cli up --workspace-folder . --config updated/devcontainer.json
```

API keys for context7 and exa, if needed, are in warp drive

connect with container via interactive zsh session:

```sh
docker exec -it <container-name-or-id> zsh
```

e.g.
```sh
docker exec -it great_jones zsh
```



## key filepaths

*MCP CONFIG*
- Gemini settings path: ~/.gemini/settings.json
- Codex mcp path: ~/.codex/mcp-servers.toml
- Claude mcp path: ~/.claude/.mcp.json

*AUTH PERSISTENCE*
- Gemini auth path: ~/.gemini/google_accounts.json ~/.gemini/oauth_creds.json
- Codex auth path: ~/.codex/auth.json
- Claude auth path: ~/.claude/.credentials.json

*SHELL ALIASES*
- cc -> claude --dangerously-skip-permissions
- cx -> codex --dangerously-bypass-approvals-and-sandbox
- ge -> gemini --model gemini-3.1-pro-preview --yolo

## startup message

- describe shell aliases

