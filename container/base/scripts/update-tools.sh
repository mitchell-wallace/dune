#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=container/base/scripts/tooling-data.sh
source "${SCRIPT_DIR}/tooling-data.sh"

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

update_release_tool() {
  local name="$1" script="$2" version_env="$3" version="${4:-}"
  echo "Updating ${name}..."
  if [ -n "${version}" ]; then
    env "${version_env}=${version}" bash "${script}"
  else
    bash "${script}"
  fi
  echo "${name} updated"
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

  for entry in "${RELEASE_TOOLS[@]}"; do
    local key="${entry%%:*}" rest="${entry#*:}"
    local script="${rest%%:*}" version_env="${rest#*:}"
    if [ "${key}" = "${TOOL_NAME}" ]; then
      update_release_tool "${key}" "${script}" "${version_env}" "${TOOL_VERSION:-}"
      return 0
    fi
  done

  echo "Unknown tool: ${TOOL_NAME}" >&2
  usage 1
}

update_all() {
  for entry in "${NPM_TOOLS[@]}"; do
    local key="${entry%%:*}" pkg="${entry#*:}"
    update_npm_tool "${key}" "${pkg}" "latest"
  done
  for entry in "${RELEASE_TOOLS[@]}"; do
    local key="${entry%%:*}" rest="${entry#*:}"
    local script="${rest%%:*}" version_env="${rest#*:}"
    update_release_tool "${key}" "${script}" "${version_env}" ""
  done
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
