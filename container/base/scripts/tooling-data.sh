#!/usr/bin/env bash

# shellcheck disable=SC2034
NPM_TOOLS=(
  "claude:@anthropic-ai/claude-code"
  "codex:@openai/codex"
  "opencode:opencode-ai"
  "gemini:@google/gemini-cli"
)

# shellcheck disable=SC2034
RELEASE_TOOLS=(
  "rally:/usr/local/bin/install-rally.sh:RALLY_VERSION"
  "laps:/usr/local/bin/install-laps.sh:LAPS_VERSION"
)
