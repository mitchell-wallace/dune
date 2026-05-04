#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd -P)"
IMAGE_VERSION="$(tr -d '\n' < "${REPO_ROOT}/container/base/IMAGE_VERSION")"
IMAGE_REF="ghcr.io/mitchell-wallace/dune-base:${IMAGE_VERSION}"
BUILD_IMAGE=0

while [ "$#" -gt 0 ]; do
  case "$1" in
    --build)
      BUILD_IMAGE=1
      shift
      ;;
    --image)
      IMAGE_REF="$2"
      shift 2
      ;;
    --skip-build)
      BUILD_IMAGE=0
      shift
      ;;
    *)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
  esac
done

CONTAINER_NAME="dune-tool-updates-$$"

cleanup() {
  docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

assert_container_command() {
  local command="$1"

  docker exec "${CONTAINER_NAME}" bash -lc "${command}" >/dev/null
}

update_npm_tool() {
  local tool="$1" version="$2" verify_command="$3"

  assert_container_command "update-tools ${tool} ${version}"
  assert_container_command "${verify_command} | grep -q ${version}"
  assert_container_command "update-tools ${tool}"
  assert_container_command "! ${verify_command} | grep -q ${version}"
}

if [ "${BUILD_IMAGE}" -eq 1 ]; then
  docker build \
    --build-arg BUILDKIT_INLINE_CACHE=1 \
    -t "${IMAGE_REF}" \
    "${REPO_ROOT}"
else
  docker image inspect "${IMAGE_REF}" >/dev/null
fi

docker run -d --rm \
  --name "${CONTAINER_NAME}" \
  --entrypoint sleep \
  "${IMAGE_REF}" \
  infinity >/dev/null

assert_container_command "whoami | grep -qx agent"
assert_container_command "tre --version"
assert_container_command "ping -c1 127.0.0.1"
assert_container_command "openspec --version"
assert_container_command "laps version"
assert_container_command "gemini --version"
assert_container_command "update-tools --help"

update_npm_tool "claude" "2.1.120" "claude --version"
update_npm_tool "codex" "0.125.0" "codex --version"
update_npm_tool "opencode" "1.14.28" "opencode --version"
update_npm_tool "gemini" "0.39.1" "gemini --version"

assert_container_command "update-tools rally 0.3.0"
assert_container_command "rally version | grep -q 0.3.0"
assert_container_command "update-tools rally"

assert_container_command "update-tools laps 0.4.4"
assert_container_command "laps version | grep -q 0.4.4"
assert_container_command "update-tools laps"
