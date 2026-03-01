

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

Build with:
npx @devcontainers/cli up --workspace-folder . --config updated/devcontainer.json


