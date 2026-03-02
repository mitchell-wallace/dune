#!/usr/bin/env bash

# Agent CLI shortcuts
alias cc='claude --dangerously-skip-permissions'
alias cx='codex --dangerously-bypass-approvals-and-sandbox'
alias ge='gemini --model gemini-3.1-pro-preview --yolo'
alias add-omc='~/add-omc.sh'

_show_agent_startup_message() {
  [ -n "${SAND_STARTUP_MESSAGE_SHOWN:-}" ] && return 0
  export SAND_STARTUP_MESSAGE_SHOWN=1

  cat <<'EOF'
Sandbox aliases:
  cc      -> claude --dangerously-skip-permissions
  cx      -> codex --dangerously-bypass-approvals-and-sandbox
  ge      -> gemini --model gemini-3.1-pro-preview --yolo
  add-omc -> ~/add-omc.sh

Oh My Claudecode setup:
  Run: add-omc
EOF
}

case "$-" in
  *i*) _show_agent_startup_message ;;
esac
