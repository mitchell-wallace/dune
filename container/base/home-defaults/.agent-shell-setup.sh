#!/usr/bin/env bash

alias cc='claude --dangerously-skip-permissions'
alias cx='codex --dangerously-bypass-approvals-and-sandbox'
alias op='OPENCODE_PERMISSION='"'"'{"*":"allow"}'"'"' opencode'
alias ut='update-tools'

if command -v gemini >/dev/null 2>&1; then
  alias ge='gemini --yolo'
fi

case ":${PATH}:" in
  *":${HOME}/.local/share/mise/shims:"*) ;;
  *) export PATH="${HOME}/.local/share/mise/shims:${PATH}" ;;
esac

case ":${PATH}:" in
  *":${HOME}/.local/bin:"*) ;;
  *) export PATH="${HOME}/.local/bin:${PATH}" ;;
esac
