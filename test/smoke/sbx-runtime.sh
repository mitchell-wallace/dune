#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=test/smoke/lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)/lib.sh"

smoke_init

BUILD_TEMPLATE=0
PROFILE="sbx-smoke"
SBX_IMAGE_VERSION="$(tr -d '\n' < "${REPO_ROOT}/container/sbx/IMAGE_VERSION")"
SBX_TEMPLATE_REF="ghcr.io/mitchell-wallace/dune-sbx:${SBX_IMAGE_VERSION}"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --build-template)
      BUILD_TEMPLATE=1
      shift
      ;;
    --profile)
      PROFILE="$2"
      shift 2
      ;;
    *)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
  esac
done

WORK_DIR="$(mktemp -d "${TMP_ROOT}/sbx-runtime-smoke.XXXXXX")"
FIXTURE_ROOT="${WORK_DIR}/sample-project"
XDG_CONFIG_HOME="${WORK_DIR}/xdg-config"
XDG_DATA_HOME="${WORK_DIR}/xdg-data"
PERSIST_HOST_PATH="${XDG_DATA_HOME}/dune/persist/${PROFILE}"
TZ_VALUE="Australia/Melbourne"
SANDBOX_NAME=""
SENTINEL_FILE="dune-sbx-smoke-sentinel.txt"
SENTINEL_VALUE="dune-sbx-smoke-${PROFILE}-$$"

cleanup() {
  if [ -n "${SANDBOX_NAME}" ]; then
    sbx rm --force "${SANDBOX_NAME}" >/dev/null 2>&1 || true
  fi
  rm -rf "${WORK_DIR}"
}
trap cleanup EXIT

sanitize_slug_base() {
  local value="$1"

  value="$(printf '%s' "${value}" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//')"
  if [ -z "${value}" ]; then
    value="workspace"
  fi
  printf '%s' "${value}"
}

workspace_slug() {
  local root="$1"
  local base
  local hash_prefix

  base="$(sanitize_slug_base "$(basename "${root}")")"
  hash_prefix="$(printf '%s' "${root}" | sha1sum | awk '{print substr($1, 1, 2)}')"
  printf '%s-%s' "${base}" "${hash_prefix}"
}

build_and_load_template() {
  local tar_path="${WORK_DIR}/dune-sbx-template.tar"

  docker build \
    -f "${REPO_ROOT}/container/sbx/Dockerfile.sbx" \
    -t "${SBX_TEMPLATE_REF}" \
    "${REPO_ROOT}"
  docker save "${SBX_TEMPLATE_REF}" -o "${tar_path}"
  sbx template load "${tar_path}"
}

run_dune() {
  (
    cd "${FIXTURE_ROOT}"
    XDG_CONFIG_HOME="${XDG_CONFIG_HOME}" \
    XDG_DATA_HOME="${XDG_DATA_HOME}" \
    TZ="${TZ_VALUE}" \
    "$@"
  )
}

run_dune_shell() {
  local command_file="$1"
  local command

  command="$(printf '%q ' "${DUNE_BIN}" up -p "${PROFILE}")"
  run_dune script -qec "${command}" /dev/null <"${command_file}"
}

assert_file_equals() {
  local path="$1"
  local expected="$2"
  local actual

  actual="$(tr -d '\r\n' < "${path}")"
  if [ "${actual}" != "${expected}" ]; then
    echo "${path} = ${actual}; want ${expected}" >&2
    exit 1
  fi
}

mkdir -p "${XDG_CONFIG_HOME}" "${XDG_DATA_HOME}"
cp -R "${REPO_ROOT}/test/fixtures/sample-project" "${FIXTURE_ROOT}"

(
  cd "${FIXTURE_ROOT}"
  git init >/dev/null
  git config user.name "Codex"
  git config user.email "codex@example.com"
  git add .
  git commit -m "fixture" >/dev/null
)

SANDBOX_NAME="dune-$(workspace_slug "${FIXTURE_ROOT}")-${PROFILE}"

if [ "${BUILD_TEMPLATE}" -eq 1 ]; then
  build_and_load_template
fi

DUNE_BIN="$("${REPO_ROOT}/scripts/build-dune.sh" --force --print-path)"

FIRST_COMMANDS="${WORK_DIR}/first-shell-commands"
cat >"${FIRST_COMMANDS}" <<EOF
pwd > .dune-smoke-pwd \\
  && git rev-parse --show-toplevel > .dune-smoke-toplevel \\
  && readlink /workspace > .dune-smoke-workspace-link \\
  && readlink /persist/agent > .dune-smoke-persist-link \\
  && readlink ~/.claude > .dune-smoke-claude-link \\
  && printf '%s\n' '${SENTINEL_VALUE}' > /persist/agent/${SENTINEL_FILE} \\
  && test -f /persist/agent/${SENTINEL_FILE}
exit \$?
EOF

run_dune_shell "${FIRST_COMMANDS}"

assert_file_equals "${FIXTURE_ROOT}/.dune-smoke-pwd" "${FIXTURE_ROOT}"
assert_file_equals "${FIXTURE_ROOT}/.dune-smoke-toplevel" "${FIXTURE_ROOT}"
assert_file_equals "${FIXTURE_ROOT}/.dune-smoke-workspace-link" "${FIXTURE_ROOT}"
assert_file_equals "${FIXTURE_ROOT}/.dune-smoke-persist-link" "${PERSIST_HOST_PATH}"

CLAUDE_LINK="$(tr -d '\r\n' < "${FIXTURE_ROOT}/.dune-smoke-claude-link")"
if [ "${CLAUDE_LINK}" != "${PERSIST_HOST_PATH}/.claude" ] && [ "${CLAUDE_LINK}" != "/persist/agent/.claude" ]; then
  echo "~/.claude links to ${CLAUDE_LINK}; want ${PERSIST_HOST_PATH}/.claude or /persist/agent/.claude" >&2
  exit 1
fi

sbx rm --force "${SANDBOX_NAME}"

SECOND_COMMANDS="${WORK_DIR}/second-shell-commands"
cat >"${SECOND_COMMANDS}" <<EOF
pwd > .dune-smoke-pwd-after \\
  && git rev-parse --show-toplevel > .dune-smoke-toplevel-after \\
  && test -f /persist/agent/${SENTINEL_FILE} \\
  && grep -qx '${SENTINEL_VALUE}' /persist/agent/${SENTINEL_FILE} \\
  && readlink ~/.claude > .dune-smoke-claude-link-after
exit \$?
EOF

run_dune_shell "${SECOND_COMMANDS}"

assert_file_equals "${FIXTURE_ROOT}/.dune-smoke-pwd-after" "${FIXTURE_ROOT}"
assert_file_equals "${FIXTURE_ROOT}/.dune-smoke-toplevel-after" "${FIXTURE_ROOT}"

echo "sbx runtime smoke passed for ${SANDBOX_NAME} using ${SBX_TEMPLATE_REF}"
