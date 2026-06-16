#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=test/smoke/lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)/lib.sh"

smoke_init
parse_image_args 1 "$@"

WORK_DIR="$(mktemp -d "${TMP_ROOT}/base-image-smoke.XXXXXX")"
CONTAINER_NAME="dune-base-smoke-$$"
PERSIST_EMPTY="${WORK_DIR}/persist-empty"
PERSIST_PRESEEDED="${WORK_DIR}/persist-preseeded"
FIXTURE_IMAGE="dune-sample-project-smoke:$$"

cleanup() {
  docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
  if [ -d "${WORK_DIR}" ]; then
    docker run --rm \
      -v "${WORK_DIR}:/work" \
      --entrypoint /bin/sh \
      alpine:3.22 \
      -c "chown -R $(id -u):$(id -g) /work" >/dev/null 2>&1 || true
  fi
  rm -rf "${WORK_DIR}"
}
trap cleanup EXIT

mkdir -p "${PERSIST_EMPTY}" "${PERSIST_PRESEEDED}"

start_container() {
  local persist_dir="$1"
  local timezone="${2:-UTC}"

  docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
  docker run -d --rm \
    --name "${CONTAINER_NAME}" \
    -e "TZ=${timezone}" \
    -v "${persist_dir}:/persist/agent" \
    "${IMAGE_REF}" >/dev/null

  wait_for_container_command "pg_isready"
  wait_for_container_command "redis-cli ping | grep -qx PONG"
  wait_for_container_command "timeout 5 bash -lc ': >/dev/tcp/127.0.0.1/8025'"
  wait_for_container_command "timeout 5 bash -lc ': >/dev/tcp/127.0.0.1/1025'"
}

build_or_inspect_image

start_container "${PERSIST_EMPTY}" "Australia/Melbourne"

assert_container_command "whoami | grep -qx agent"
assert_container_command "echo \"\$SHELL\" | grep -qx /bin/zsh"
assert_container_command "sudo whoami | grep -qx root"

assert_container_command "node --version"
assert_container_command "go version"
assert_container_command "python --version"
assert_container_command "uv --version"
assert_container_command "pnpm --version"
assert_container_command "turbo --version"
assert_container_command "mise --version"
assert_container_command "rally version"
assert_container_command "jq --version"
assert_container_command "rg --version"
assert_container_command "tmux -V"
assert_container_command "fd --version"
assert_container_command "bat --version"
assert_container_command "eza --version"
assert_container_command "delta --version"
assert_container_command "gitui --version"
assert_container_command "tre --version"
assert_container_command "ping -c1 127.0.0.1"

assert_container_command "claude --version"
assert_container_command "codex --version"
assert_container_command "gemini --version"
assert_container_command "laps version"
assert_container_command "openspec --version"
assert_container_command "opencode --version"
assert_container_command "agy --version"
assert_container_command "thenn version"
assert_container_command "update-tools --help"
assert_container_command "test ! -e /usr/local/bin/br"
assert_container_command "test ! -e /home/agent/.claude/skills/bd-to-br-migration"
assert_container_command "test ! -e /home/agent/.codex/skills/bd-to-br-migration"

POSTGRES_PID_BEFORE="$(docker exec "${CONTAINER_NAME}" bash -lc "pgrep -xo postgres")"
docker exec "${CONTAINER_NAME}" bash -lc "kill -9 ${POSTGRES_PID_BEFORE}" >/dev/null
wait_for_container_command "pg_isready"
POSTGRES_PID_AFTER="$(docker exec "${CONTAINER_NAME}" bash -lc "pgrep -xo postgres")"
if [ "${POSTGRES_PID_BEFORE}" = "${POSTGRES_PID_AFTER}" ]; then
  echo "Postgres PID did not change after forced restart" >&2
  exit 1
fi

# .zshrc and .p10k.zsh are image-owned: not seeded into the persist volume, and
# linked straight to the image defaults so version bumps always reach the
# container instead of being shadowed by a stale persisted copy.
test ! -e "${PERSIST_EMPTY}/.zshrc"
test ! -e "${PERSIST_EMPTY}/.p10k.zsh"
assert_container_command 'readlink -f /home/agent/.zshrc | grep -qx /opt/home-defaults/.zshrc'
assert_container_command 'readlink -f /home/agent/.p10k.zsh | grep -qx /opt/home-defaults/.p10k.zsh'
assert_container_command 'readlink -f /home/agent/.codex | grep -qx /persist/agent/.codex'

printf 'custom zshrc\n' > "${PERSIST_PRESEEDED}/.zshrc"
printf 'custom p10k\n' > "${PERSIST_PRESEEDED}/.p10k.zsh"
mkdir -p "${PERSIST_PRESEEDED}/.codex"
mkdir -p "${PERSIST_PRESEEDED}/.claude/skills/bd-to-br-migration"
mkdir -p "${PERSIST_PRESEEDED}/.codex/skills/bd-to-br-migration"
start_container "${PERSIST_PRESEEDED}" "UTC"

# A stale .zshrc/.p10k.zsh in an older profile volume must NOT shadow the image
# copy: the home links resolve to the image defaults, so the custom persisted
# content is ignored (this is the regression that left containers on an old
# prompt palette after an image bump).
assert_container_command 'readlink -f /home/agent/.zshrc | grep -qx /opt/home-defaults/.zshrc'
assert_container_command 'readlink -f /home/agent/.p10k.zsh | grep -qx /opt/home-defaults/.p10k.zsh'
assert_container_command "! grep -qx 'custom zshrc' /home/agent/.zshrc"
assert_container_command "! grep -qx 'custom p10k' /home/agent/.p10k.zsh"
assert_container_command "test ! -e /home/agent/.claude/skills/bd-to-br-migration"
assert_container_command "test ! -e /home/agent/.codex/skills/bd-to-br-migration"

docker build \
  --build-arg "BASE_IMAGE=${IMAGE_REF}" \
  -f "${REPO_ROOT}/test/fixtures/sample-project/Dockerfile.dune" \
  -t "${FIXTURE_IMAGE}" \
  "${REPO_ROOT}/test/fixtures/sample-project"

docker run --rm --entrypoint /bin/bash "${FIXTURE_IMAGE}" -lc \
  "test -f /opt/sample-project/message.txt && grep -qx 'hello from sample-project' /opt/sample-project/message.txt"
