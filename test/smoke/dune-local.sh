#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd -P)"
IMAGE_VERSION="$(tr -d '\n' < "${REPO_ROOT}/container/base/IMAGE_VERSION")"
IMAGE_REF="ghcr.io/mitchell-wallace/dune-base:${IMAGE_VERSION}"
TMP_ROOT="${REPO_ROOT}/tmp"
mkdir -p "${TMP_ROOT}"
WORK_DIR="$(mktemp -d "${TMP_ROOT}/dune-local-smoke.XXXXXX")"
FIXTURE_ROOT="${WORK_DIR}/sample-project"
HOME_DIR="${WORK_DIR}/home"
XDG_CONFIG_HOME="${WORK_DIR}/xdg-config"
XDG_DATA_HOME="${WORK_DIR}/xdg-data"
TZ_VALUE="Australia/Melbourne"

cleanup() {
  if [ -f "${COMPOSE_PATH:-}" ]; then
    docker compose -f "${COMPOSE_PATH}" -p "${COMPOSE_PROJECT:-}" down >/dev/null 2>&1 || true
  fi
  rm -rf "${WORK_DIR}"
}
trap cleanup EXIT

mkdir -p "${HOME_DIR}" "${XDG_CONFIG_HOME}" "${XDG_DATA_HOME}"
cp -R "${REPO_ROOT}/test/fixtures/sample-project" "${FIXTURE_ROOT}"

(
  cd "${FIXTURE_ROOT}"
  git init >/dev/null
  git config user.name "Codex"
  git config user.email "codex@example.com"
  git add .
  git commit -m "fixture" >/dev/null
)

docker image inspect "${IMAGE_REF}" >/dev/null 2>&1 || docker build --build-arg BUILDKIT_INLINE_CACHE=1 -t "${IMAGE_REF}" "${REPO_ROOT}" >/dev/null
DUNE_BIN="$("${REPO_ROOT}/scripts/build-dune.sh" --force --print-path)"

run_dune() {
  (
    cd "${FIXTURE_ROOT}"
    HOME="${HOME_DIR}" \
    XDG_CONFIG_HOME="${XDG_CONFIG_HOME}" \
    XDG_DATA_HOME="${XDG_DATA_HOME}" \
    TZ="${TZ_VALUE}" \
    "$@"
  )
}

run_dune_with_shell() {
  run_dune bash -lc "printf 'exit\n' | script -qec '${DUNE_BIN} $*' /dev/null"
}

run_dune_with_shell up

COMPOSE_PATH="$(find "${XDG_DATA_HOME}/dune/projects" -name compose.yaml -print -quit)"
if [ -z "${COMPOSE_PATH}" ]; then
  echo "Unable to find generated compose file" >&2
  exit 1
fi

COMPOSE_PROJECT="$(basename "$(dirname "${COMPOSE_PATH}")")"
COMPOSE_PROJECT="dune-${COMPOSE_PROJECT}-default"

wait_for_agent() {
  local command="$1"
  local remaining=30

  while [ "${remaining}" -gt 0 ]; do
    if docker compose -f "${COMPOSE_PATH}" -p "${COMPOSE_PROJECT}" exec -T agent bash -lc "${command}" >/dev/null 2>&1; then
      return 0
    fi
    remaining=$((remaining - 1))
    sleep 2
  done

  docker compose -f "${COMPOSE_PATH}" -p "${COMPOSE_PROJECT}" logs >&2 || true
  echo "Timed out waiting for agent command: ${command}" >&2
  return 1
}

wait_for_agent "pg_isready"

docker compose -f "${COMPOSE_PATH}" -p "${COMPOSE_PROJECT}" ps | grep -q agent
docker compose -f "${COMPOSE_PATH}" -p "${COMPOSE_PROJECT}" ps | grep -q pipelock

wait_for_agent "test \"\$(curl -sS -o /dev/null -w '%{http_code}' --max-time 20 https://api.anthropic.com)\" != '000'"
if docker compose -f "${COMPOSE_PATH}" -p "${COMPOSE_PROJECT}" exec -T agent bash -lc \
  "env -u http_proxy -u HTTP_PROXY -u https_proxy -u HTTPS_PROXY -u no_proxy -u NO_PROXY curl -fsSI --max-time 10 https://api.anthropic.com >/dev/null"; then
  echo "Agent unexpectedly reached the internet without proxy settings" >&2
  exit 1
fi

LOG_FILE="${WORK_DIR}/pipelock.log"
status=0
run_dune timeout 10s "${DUNE_BIN}" logs pipelock >"${LOG_FILE}" 2>&1 || status=$?
if [ "${status}" -ne 0 ] && [ "${status}" -ne 124 ]; then
  cat "${LOG_FILE}" >&2
  exit "${status}"
fi
grep -q '{' "${LOG_FILE}"

docker compose -f "${COMPOSE_PATH}" -p "${COMPOSE_PROJECT}" exec -T agent bash -lc "printf 'persisted=true\n' > ~/.gitconfig"
run_dune "${DUNE_BIN}" down
run_dune_with_shell up
wait_for_agent "grep -qx 'persisted=true' ~/.gitconfig"
run_dune "${DUNE_BIN}" rebuild
wait_for_agent "grep -qx 'persisted=true' ~/.gitconfig"

run_dune "${DUNE_BIN}" down
run_dune_with_shell -p work
WORK_COMPOSE_PATH="$(find "${XDG_DATA_HOME}/dune/projects" -name compose.yaml -print -quit)"
WORK_COMPOSE_PROJECT="dune-$(basename "$(dirname "${WORK_COMPOSE_PATH}")")-work"
docker compose -f "${WORK_COMPOSE_PATH}" -p "${WORK_COMPOSE_PROJECT}" exec -T agent bash -lc "printf 'work-profile\n' > ~/.gitconfig"
run_dune "${DUNE_BIN}" down -p work

run_dune_with_shell -p personal
PERSONAL_COMPOSE_PATH="$(find "${XDG_DATA_HOME}/dune/projects" -name compose.yaml -print -quit)"
PERSONAL_COMPOSE_PROJECT="dune-$(basename "$(dirname "${PERSONAL_COMPOSE_PATH}")")-personal"
COMPOSE_PATH="${PERSONAL_COMPOSE_PATH}"
COMPOSE_PROJECT="${PERSONAL_COMPOSE_PROJECT}"
if docker compose -f "${PERSONAL_COMPOSE_PATH}" -p "${PERSONAL_COMPOSE_PROJECT}" exec -T agent bash -lc "grep -qx 'work-profile' ~/.gitconfig"; then
  echo "Profile volumes are not isolated" >&2
  exit 1
fi

HOST_ZONE="$(TZ="${TZ_VALUE}" date +%Z)"
docker compose -f "${PERSONAL_COMPOSE_PATH}" -p "${PERSONAL_COMPOSE_PROJECT}" exec -T agent bash -lc "date +%Z" | grep -qx "${HOST_ZONE}"
docker compose -f "${PERSONAL_COMPOSE_PATH}" -p "${PERSONAL_COMPOSE_PROJECT}" exec -T agent bash -lc "node --version"
docker compose -f "${PERSONAL_COMPOSE_PATH}" -p "${PERSONAL_COMPOSE_PROJECT}" exec -T agent bash -lc "go version"
docker compose -f "${PERSONAL_COMPOSE_PATH}" -p "${PERSONAL_COMPOSE_PROJECT}" exec -T agent bash -lc "python --version"
docker compose -f "${PERSONAL_COMPOSE_PATH}" -p "${PERSONAL_COMPOSE_PROJECT}" exec -T agent bash -lc "rustc --version"
docker compose -f "${PERSONAL_COMPOSE_PATH}" -p "${PERSONAL_COMPOSE_PROJECT}" exec -T agent bash -lc "uv --version"
