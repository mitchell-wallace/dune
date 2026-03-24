#!/usr/bin/env bash

# Agent CLI shortcuts
alias cc='claude --dangerously-skip-permissions'
alias cx='codex --dangerously-bypass-approvals-and-sandbox'
if command -v gemini >/dev/null 2>&1; then
  alias ge='gemini --model gemini-3.1-pro-preview --yolo'
fi
if command -v opencode >/dev/null 2>&1; then
  alias op='opencode --yolo'
fi

# Ensure mise shims are available in interactive shells. Keep this path even
# before shims exist so newly installed runtimes work immediately.
case ":${PATH}:" in
  *":${HOME}/.local/share/mise/shims:"*) ;;
  *) export PATH="${HOME}/.local/share/mise/shims:${PATH}" ;;
esac

_show_agent_startup_message() {
  [ -n "${DUNE_STARTUP_MESSAGE_SHOWN:-}" ] && return 0
  export DUNE_STARTUP_MESSAGE_SHOWN=1

  local mode profile ws_mode
  mode="$(printf '%s' "${DUNE_SECURITY_MODE:-std}" | tr '[:upper:]' '[:lower:]')"
  profile="$(printf '%s' "${DUNE_PROFILE:-0}" | tr '[:upper:]' '[:lower:]')"
  ws_mode="$(printf '%s' "${DUNE_WORKSPACE_MODE:-mount}" | tr '[:upper:]' '[:lower:]')"

  cat <<EOF
Sandbox profile: ${profile}
Security mode: ${mode}
Workspace mode: ${ws_mode}

Sandbox aliases:
  cc      -> claude --dangerously-skip-permissions
  cx      -> codex --dangerously-bypass-approvals-and-sandbox
EOF

  if command -v gemini >/dev/null 2>&1; then
    printf '%s\n' "  ge      -> gemini --model gemini-3.1-pro-preview --yolo"
  fi
  if command -v opencode >/dev/null 2>&1; then
    printf '%s\n' "  op      -> opencode --yolo"
  fi

  if [ "$mode" != "strict" ]; then
    cat <<'EOF'
Gear:
  gear                   -> list installed gear, available gear, and helper commands
  gear install add-omc   -> install Oh My Claudecode
  gear install boost-cli -> install optional CLI boost tools (fd/rg/bat/tree/eza/micro)
  gear install add-postgres -> install local PostgreSQL + pg-local helper
  gear install add-redis -> install local Redis + redis-local helper
  gear install add-playwright -> install Playwright CLI + browsers for e2e tests
  gear install add-pnpm/add-turbo/add-wrangler -> JS/edge CLIs
  gear install add-mailpit/add-minio/add-meilisearch -> local service binaries (mp-local helper for Mailpit)
  gear install add-python-uv/add-go/add-rust/add-dotnet/add-java/add-bun/add-deno -> language runtimes via mise
EOF
    if ! command -v gemini >/dev/null 2>&1; then
      printf '%s\n' "  gear install add-gemini -> install Gemini CLI with persisted config/auth storage"
    fi
    if ! command -v opencode >/dev/null 2>&1; then
      printf '%s\n' "  gear install add-opencode -> install OpenCode CLI with persisted config/auth storage"
    fi
  fi
}

case "$-" in
  *i*) _show_agent_startup_message ;;
esac
