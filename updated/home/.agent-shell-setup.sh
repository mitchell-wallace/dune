#!/usr/bin/env bash

# Agent CLI shortcuts
alias cc='claude --dangerously-skip-permissions'
alias cx='codex --dangerously-bypass-approvals-and-sandbox'
alias ge='gemini --model gemini-3.1-pro-preview --yolo'
alias op='opencode --yolo'

# Ensure mise shims are available in interactive shells. Keep this path even
# before shims exist so newly installed runtimes work immediately.
case ":${PATH}:" in
  *":${HOME}/.local/share/mise/shims:"*) ;;
  *) export PATH="${HOME}/.local/share/mise/shims:${PATH}" ;;
esac

_show_agent_startup_message() {
  [ -n "${SAND_STARTUP_MESSAGE_SHOWN:-}" ] && return 0
  export SAND_STARTUP_MESSAGE_SHOWN=1

  local mode profile ws_mode
  mode="$(printf '%s' "${SAND_SECURITY_MODE:-std}" | tr '[:upper:]' '[:lower:]')"
  profile="$(printf '%s' "${SAND_PROFILE:-0}" | tr '[:upper:]' '[:lower:]')"
  ws_mode="$(printf '%s' "${SAND_WORKSPACE_MODE:-mount}" | tr '[:upper:]' '[:lower:]')"

  cat <<EOF
Sandbox profile: ${profile}
Security mode: ${mode}
Workspace mode: ${ws_mode}

Sandbox aliases:
  cc      -> claude --dangerously-skip-permissions
  cx      -> codex --dangerously-bypass-approvals-and-sandbox
  ge      -> gemini --model gemini-3.1-pro-preview --yolo
  op      -> opencode --yolo
EOF

  if [ "$mode" != "strict" ]; then
    cat <<'EOF'
Addons:
  addons           -> list addons, install status, helper commands
  addons add-omc   -> install Oh My Claudecode
  addons boost-cli -> install optional CLI boost tools (fd/rg/bat/tree/eza/micro)
  addons add-postgres -> install local PostgreSQL + pg-local helper
  addons add-redis -> install local Redis + redis-local helper
  addons add-playwright -> install Playwright CLI + browsers for e2e tests
  addons add-pnpm/add-turbo/add-wrangler -> JS/edge CLIs
  addons add-mailpit/add-minio/add-meilisearch -> local service binaries (mp-local helper for Mailpit)
  addons add-python-uv/add-go/add-rust/add-dotnet/add-java/add-bun/add-deno -> language runtimes via mise
EOF
  fi
}

case "$-" in
  *i*) _show_agent_startup_message ;;
esac
