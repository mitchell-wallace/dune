#!/usr/bin/env bash

# shellcheck disable=SC2034
NPM_TOOLS=(
  "claude:@anthropic-ai/claude-code"
  "codex:@openai/codex"
  "opencode:opencode-ai"
)

# shellcheck disable=SC2034
RELEASE_TOOLS=(
  "rally:/usr/local/bin/install-rally.sh:RALLY_VERSION"
  "laps:/usr/local/bin/install-laps.sh:LAPS_VERSION"
  "agy:/usr/local/bin/install-agy.sh:AGY_VERSION"
  "thenn:/usr/local/bin/install-thenn.sh:THENN_VERSION:self"
)
