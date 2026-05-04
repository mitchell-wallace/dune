#!/usr/bin/env bash

smoke_init() {
  SMOKE_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[1]}")" && pwd -P)"
  REPO_ROOT="$(cd "${SMOKE_SCRIPT_DIR}/../.." && pwd -P)"
  IMAGE_VERSION="$(tr -d '\n' < "${REPO_ROOT}/container/base/IMAGE_VERSION")"
  IMAGE_REF="ghcr.io/mitchell-wallace/dune-base:${IMAGE_VERSION}"
  TMP_ROOT="${REPO_ROOT}/tmp"
  mkdir -p "${TMP_ROOT}"
}

parse_image_args() {
  BUILD_IMAGE="$1"
  shift

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
}

build_or_inspect_image() {
  if [ "${BUILD_IMAGE}" -eq 1 ]; then
    docker build \
      --build-arg BUILDKIT_INLINE_CACHE=1 \
      -t "${IMAGE_REF}" \
      "${REPO_ROOT}"
  else
    docker image inspect "${IMAGE_REF}" >/dev/null
  fi
}

ensure_image_available() {
  docker image inspect "${IMAGE_REF}" >/dev/null 2>&1 || docker build \
    --build-arg BUILDKIT_INLINE_CACHE=1 \
    -t "${IMAGE_REF}" \
    "${REPO_ROOT}" >/dev/null
}

assert_container_command() {
  local command="$1"

  docker exec "${CONTAINER_NAME}" bash -lc "${command}" >/dev/null
}

wait_for_container_command() {
  local command="$1"
  local remaining="${2:-30}"

  while [ "${remaining}" -gt 0 ]; do
    if docker exec "${CONTAINER_NAME}" bash -lc "${command}" >/dev/null 2>&1; then
      return 0
    fi
    remaining=$((remaining - 1))
    sleep 2
  done

  docker logs "${CONTAINER_NAME}" >&2 || true
  echo "Timed out waiting for container command: ${command}" >&2
  return 1
}

wait_for_compose_command() {
  local compose_path="$1"
  local compose_project="$2"
  local command="$3"
  local remaining="${4:-30}"

  while [ "${remaining}" -gt 0 ]; do
    if docker compose -f "${compose_path}" -p "${compose_project}" exec -T agent bash -lc "${command}" >/dev/null 2>&1; then
      return 0
    fi
    remaining=$((remaining - 1))
    sleep 2
  done

  docker compose -f "${compose_path}" -p "${compose_project}" logs >&2 || true
  echo "Timed out waiting for agent command: ${command}" >&2
  return 1
}
