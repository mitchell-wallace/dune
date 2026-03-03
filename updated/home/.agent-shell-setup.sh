#!/usr/bin/env bash

# Agent CLI shortcuts
alias cc='claude --dangerously-skip-permissions'
alias cx='codex --dangerously-bypass-approvals-and-sandbox'
alias ge='gemini --model gemini-3.1-pro-preview --yolo'

_show_agent_startup_message() {
  [ -n "${SAND_STARTUP_MESSAGE_SHOWN:-}" ] && return 0
  export SAND_STARTUP_MESSAGE_SHOWN=1

  local mode profile
  mode="$(printf '%s' "${SAND_SECURITY_MODE:-std}" | tr '[:upper:]' '[:lower:]')"
  profile="$(printf '%s' "${SAND_PROFILE:-0}" | tr '[:upper:]' '[:lower:]')"

  cat <<EOF
Sandbox profile: ${profile}
Security mode: ${mode}

Sandbox aliases:
  cc      -> claude --dangerously-skip-permissions
  cx      -> codex --dangerously-bypass-approvals-and-sandbox
  ge      -> gemini --model gemini-3.1-pro-preview --yolo
EOF

  if [ "$mode" != "strict" ]; then
    cat <<'EOF'
Addons:
  addons           -> list addons, install status, helper commands
  addons add-omc   -> install Oh My Claudecode
  addons boost-cli -> install optional CLI boost tools (fd/rg/bat/tree/eza/micro)
  addons add-postgres -> install local PostgreSQL + pg-local helper
  addons add-redis -> install local Redis + redis-local helper
EOF
  fi
}

case "$-" in
  *i*) _show_agent_startup_message ;;
esac
