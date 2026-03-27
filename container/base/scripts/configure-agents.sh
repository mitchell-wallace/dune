#!/usr/bin/env bash
set -euo pipefail

if command -v claude >/dev/null 2>&1; then
  claude mcp add --transport http grep-app https://mcp.grep.app || true
  claude mcp add --transport http context7 https://mcp.context7.com/mcp || true
  claude mcp add --transport http exa-ai 'https://mcp.exa.ai/mcp?tools=web_search_exa' || true
fi

if command -v gh >/dev/null 2>&1; then
  gh auth setup-git || true
fi
