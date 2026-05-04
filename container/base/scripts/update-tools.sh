#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: update-tools [--all | TOOL [VERSION] | TOOL@VERSION]

Tools: claude, codex, opencode, gemini, rally, laps

Examples:
  update-tools --all
  update-tools claude
  update-tools codex 0.125.0
  update-tools codex@0.125.0
EOF
  exit "${1:-0}"
}

NPM_TOOLS=(
  "claude:@anthropic-ai/claude-code"
  "codex:@openai/codex"
  "opencode:opencode-ai"
  "gemini:@google/gemini-cli"
)

run_privileged() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    "$@"
  fi
}

parse_tool_arg() {
  local arg="$1"
  if [[ "${arg}" == *@* ]]; then
    TOOL_NAME="${arg%@*}"
    TOOL_VERSION="${arg#*@}"
  else
    TOOL_NAME="${arg}"
    TOOL_VERSION="${2:-}"
  fi

  if [ -z "${TOOL_NAME}" ] || [ -z "${TOOL_VERSION:-latest}" ]; then
    usage 1
  fi
}

update_npm_tool() {
  local name="$1" pkg="$2" version="${3:-latest}"
  echo "Updating ${name} (${pkg}@${version})..."
  run_privileged npm install -g "${pkg}@${version}"
  echo "${name} updated"
}

update_rally() {
  local version="${1:-}"
  echo "Updating rally..."
  if [ -n "${version}" ]; then
    RALLY_VERSION="${version}" bash /usr/local/bin/install-rally.sh
  else
    bash /usr/local/bin/install-rally.sh
  fi
  echo "rally updated"
}

update_laps() {
  local version="${1:-}"
  echo "Updating laps..."
  if [ -n "${version}" ]; then
    LAPS_VERSION="${version}" bash /usr/local/bin/install-laps.sh
  else
    bash /usr/local/bin/install-laps.sh
  fi
  echo "laps updated"
}

update_single() {
  parse_tool_arg "$@"

  for entry in "${NPM_TOOLS[@]}"; do
    local key="${entry%%:*}" pkg="${entry#*:}"
    if [ "${key}" = "${TOOL_NAME}" ]; then
      update_npm_tool "${key}" "${pkg}" "${TOOL_VERSION:-latest}"
      return 0
    fi
  done

  case "${TOOL_NAME}" in
    rally) update_rally "${TOOL_VERSION:-}" ;;
    laps)  update_laps "${TOOL_VERSION:-}" ;;
    *)     echo "Unknown tool: ${TOOL_NAME}" >&2; usage 1 ;;
  esac
}

update_all() {
  for entry in "${NPM_TOOLS[@]}"; do
    local key="${entry%%:*}" pkg="${entry#*:}"
    update_npm_tool "${key}" "${pkg}" "latest"
  done
  update_rally ""
  update_laps ""
}

if [ "$#" -eq 0 ]; then
  usage
fi

case "$1" in
  --all)
    update_all
    ;;
  -h|--help)
    usage
    ;;
  *)
    update_single "$@"
    ;;
esac
