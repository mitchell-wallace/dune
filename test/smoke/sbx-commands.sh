#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=test/smoke/lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)/lib.sh"

smoke_init

BUILD_TEMPLATE=0
PROFILE="sbx-commands-smoke"
DOCS_URL="https://docs.python.org/3/"
DOCS_HOST="docs.python.org:443"
REPORT_PATH="${REPO_ROOT}/tmp/sbx-commands-smoke.$(date -u +%Y%m%dT%H%M%SZ).log"

usage() {
  cat <<'EOF'
Usage: test/smoke/sbx-commands.sh [options]

Options:
  --build-template     Build and load the local dune-sbx template first
  --profile NAME       Dune profile to use for the temporary sandbox
  --docs-url URL       URL used to create an sbx policy-log record
  --report PATH        Write command/output evidence to PATH
EOF
}

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
    --docs-url)
      DOCS_URL="$2"
      shift 2
      ;;
    --report)
      REPORT_PATH="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

SBX_IMAGE_VERSION="$(tr -d '\n' < "${REPO_ROOT}/container/sbx/IMAGE_VERSION")"
SBX_TEMPLATE_REF="ghcr.io/mitchell-wallace/dune-sbx:${SBX_IMAGE_VERSION}"
WORK_DIR="$(mktemp -d "${TMP_ROOT}/sbx-commands-smoke.XXXXXX")"
FIXTURE_ROOT="${WORK_DIR}/sample-project"
XDG_CONFIG_HOME="${WORK_DIR}/xdg-config"
XDG_DATA_HOME="${WORK_DIR}/xdg-data"
PERSIST_HOST_PATH="${XDG_DATA_HOME}/dune/persist/${PROFILE}"
TZ_VALUE="Australia/Melbourne"
SANDBOX_NAME=""
SENTINEL_FILE="dune-sbx-commands-sentinel.txt"
SENTINEL_VALUE="dune-sbx-commands-${PROFILE}-$$"
REPORT_DIR="$(dirname "${REPORT_PATH}")"
mkdir -p "${REPORT_DIR}"
: >"${REPORT_PATH}"

log() {
  printf '%s\n' "$*" | tee -a "${REPORT_PATH}"
}

capture_report() {
  local output_path="$1"
  local status
  shift

  log ""
  log "$ $*"
  set +e
  "$@" >"${output_path}" 2>&1
  status=$?
  set -e
  sed 's/^/  /' "${output_path}" | tee -a "${REPORT_PATH}"
  log "[exit ${status}]"
  return "${status}"
}

run_report() {
  local output_path

  output_path="$(mktemp "${WORK_DIR}/command-output.XXXXXX")"
  capture_report "${output_path}" "$@"
  rm -f "${output_path}"
}

capture_report_stdin() {
  local output_path="$1"
  local input_path="$2"
  local status
  shift 2

  log ""
  log "$ $* < ${input_path}"
  set +e
  "$@" >"${output_path}" 2>&1 <"${input_path}"
  status=$?
  set -e
  sed 's/^/  /' "${output_path}" | tee -a "${REPORT_PATH}"
  log "[exit ${status}]"
  return "${status}"
}

cleanup() {
  if [ -n "${SANDBOX_NAME}" ]; then
    {
      printf '\n$ sbx rm --force %s\n' "${SANDBOX_NAME}"
      sbx rm --force "${SANDBOX_NAME}"
      printf '[cleanup exit 0]\n'
    } >>"${REPORT_PATH}" 2>&1 || {
      printf '[cleanup failed]\n' >>"${REPORT_PATH}"
    }
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

  run_report docker build \
    -f "${REPO_ROOT}/container/sbx/Dockerfile.sbx" \
    -t "${SBX_TEMPLATE_REF}" \
    "${REPO_ROOT}"
  run_report docker save "${SBX_TEMPLATE_REF}" -o "${tar_path}"
  run_report sbx template load "${tar_path}"
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
  local output_path="$2"
  local command

  command="$(printf '%q ' "${DUNE_BIN}" up -p "${PROFILE}")"
  capture_report_stdin "${output_path}" "${command_file}" run_dune script -qec "${command}" /dev/null
}

sandbox_exec() {
  local script="$1"

  run_report sbx exec "${SANDBOX_NAME}" bash -lc "${script}"
}

sandbox_is_listed() {
  sbx ls --json | jq -e --arg name "${SANDBOX_NAME}" '.sandboxes[]? | select(.name == $name)' >/dev/null
}

assert_sandbox_absent() {
  local list_path="$1"

  if jq -e --arg name "${SANDBOX_NAME}" '.sandboxes[]? | select(.name == $name)' "${list_path}" >/dev/null; then
    log "Sandbox ${SANDBOX_NAME} is still listed"
    return 1
  fi
}

assert_sandbox_status() {
  local list_path="$1"
  local want_status="$2"

  jq -e --arg name "${SANDBOX_NAME}" --arg status "${want_status}" \
    '.sandboxes[]? | select(.name == $name and (.status | ascii_downcase) == $status)' "${list_path}" >/dev/null
}

assert_doctor_passes_core_checks() {
  local doctor_path="$1"

  jq -e '
    ([.checks[] | select(.id == "sbx.path" and .status == "pass")] | length == 1) and
    ([.checks[] | select(.id == "sbx.diagnose" and .status == "pass")] | length == 1) and
    ([.checks[] | select(.id == "sbx.version" and .status == "pass")] | length == 1) and
    ([.checks[] | select(.id == "sandbox.status" and .status == "pass")] | length == 1)
  ' "${doctor_path}" >/dev/null
}

wait_for_policy_log_record() {
  local policy_path="${WORK_DIR}/policy-log.json"

  for _ in $(seq 1 12); do
    if capture_report "${policy_path}" sbx policy log "${SANDBOX_NAME}" --json --limit 100; then
      if jq -e --arg host "${DOCS_HOST}" '.blocked_hosts[]? | select(.host == $host)' "${policy_path}" >/dev/null; then
        log "Observed blocked sbx policy-log record for ${DOCS_HOST}"
        return 0
      fi
    fi
    sleep 2
  done

  log "Missing blocked sbx policy-log record for ${DOCS_HOST}"
  return 1
}

assert_file_equals() {
  local path="$1"
  local expected="$2"
  local actual

  actual="$(tr -d '\r\n' < "${path}")"
  if [ "${actual}" != "${expected}" ]; then
    log "${path} = ${actual}; want ${expected}"
    return 1
  fi
}

for required in sbx jq script; do
  if ! command -v "${required}" >/dev/null 2>&1; then
    log "Missing required command on host: ${required}"
    exit 1
  fi
done
if [ "${BUILD_TEMPLATE}" -eq 1 ] && ! command -v docker >/dev/null 2>&1; then
  log "Missing required command on host: docker"
  exit 1
fi

log "# sbx command smoke"
log "report: ${REPORT_PATH}"
log "template: ${SBX_TEMPLATE_REF}"
log "profile: ${PROFILE}"
log "docs url: ${DOCS_URL}"

run_report sbx diagnose || {
  log "sbx diagnose failed. This smoke requires an installed, authenticated, healthy sbx daemon."
  exit 1
}
run_report sbx rm --help
run_report sbx ports --help

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

if sandbox_is_listed; then
  run_report sbx rm --force "${SANDBOX_NAME}"
fi

if [ "${BUILD_TEMPLATE}" -eq 1 ]; then
  build_and_load_template
fi

DUNE_BIN="$("${REPO_ROOT}/scripts/build-dune.sh" --force --print-path)"

log ""
log "## dune doctor is non-mutating for an absent sandbox"
ABSENT_BEFORE="${WORK_DIR}/sbx-ls-absent-before.json"
ABSENT_AFTER="${WORK_DIR}/sbx-ls-absent-after.json"
DOCTOR_ABSENT="${WORK_DIR}/doctor-absent.json"
capture_report "${ABSENT_BEFORE}" sbx ls --json
assert_sandbox_absent "${ABSENT_BEFORE}"
capture_report "${DOCTOR_ABSENT}" run_dune "${DUNE_BIN}" doctor --json -p "${PROFILE}"
assert_doctor_passes_core_checks "${DOCTOR_ABSENT}"
capture_report "${ABSENT_AFTER}" sbx ls --json
cmp "${ABSENT_BEFORE}" "${ABSENT_AFTER}" >/dev/null

log ""
log "## dune up creates the sandbox and writes profile-scoped persisted state"
FIRST_COMMANDS="${WORK_DIR}/first-shell-commands"
FIRST_UP_OUTPUT="${WORK_DIR}/first-up.out"
cat >"${FIRST_COMMANDS}" <<EOF
pwd > .dune-commands-pwd
printf '%s\n' '${SENTINEL_VALUE}' > /persist/agent/${SENTINEL_FILE}
test -f /persist/agent/${SENTINEL_FILE}
exit \$?
EOF
run_dune_shell "${FIRST_COMMANDS}" "${FIRST_UP_OUTPUT}"
assert_file_equals "${FIXTURE_ROOT}/.dune-commands-pwd" "${FIXTURE_ROOT}"
test -f "${PERSIST_HOST_PATH}/${SENTINEL_FILE}"
assert_file_equals "${PERSIST_HOST_PATH}/${SENTINEL_FILE}" "${SENTINEL_VALUE}"

log ""
log "## Generate sbx policy-log evidence"
sandbox_exec "curl -sSL -o /tmp/dune-commands-docs-body -w '%{http_code}\n' --max-time 30 '${DOCS_URL}' || true"
wait_for_policy_log_record

log ""
log "## dune logs includes Dune-owned logs and sbx policy records"
LOGS_OUTPUT="${WORK_DIR}/dune-logs.out"
capture_report "${LOGS_OUTPUT}" run_dune "${DUNE_BIN}" logs -p "${PROFILE}"
grep -Eq 'dune host lifecycle log|dune sandbox logs|setup-persist' "${LOGS_OUTPUT}"
grep -Fq "== sbx policy log ==" "${LOGS_OUTPUT}"
grep -Fq "blocked host=${DOCS_HOST}" "${LOGS_OUTPUT}"
if capture_report "${WORK_DIR}/logs-pipelock.out" run_dune "${DUNE_BIN}" logs -p "${PROFILE}" pipelock; then
  log "dunex logs pipelock unexpectedly succeeded"
  exit 1
fi
grep -Fq "dunex logs pipelock is not available" "${WORK_DIR}/logs-pipelock.out"

log ""
log "## dune ports lists through sbx ports and surfaces publish guidance"
PORTS_DIRECT="${WORK_DIR}/sbx-ports.out"
PORTS_DUNE="${WORK_DIR}/dune-ports.out"
PORTS_PUBLISH="${WORK_DIR}/dune-ports-publish.out"
capture_report "${PORTS_DIRECT}" sbx ports "${SANDBOX_NAME}"
capture_report "${PORTS_DUNE}" run_dune "${DUNE_BIN}" ports -p "${PROFILE}"
capture_report "${PORTS_PUBLISH}" run_dune "${DUNE_BIN}" ports -p "${PROFILE}" --publish 8080
grep -Fq "bind dev servers to all sandbox interfaces" "${PORTS_PUBLISH}"

log ""
log "## dune doctor is non-mutating for a stopped sandbox"
run_report run_dune "${DUNE_BIN}" down -p "${PROFILE}"
STOPPED_BEFORE="${WORK_DIR}/sbx-ls-stopped-before.json"
STOPPED_AFTER="${WORK_DIR}/sbx-ls-stopped-after.json"
DOCTOR_STOPPED="${WORK_DIR}/doctor-stopped.json"
capture_report "${STOPPED_BEFORE}" sbx ls --json
assert_sandbox_status "${STOPPED_BEFORE}" "stopped"
capture_report "${DOCTOR_STOPPED}" run_dune "${DUNE_BIN}" doctor --json -p "${PROFILE}"
assert_doctor_passes_core_checks "${DOCTOR_STOPPED}"
capture_report "${STOPPED_AFTER}" sbx ls --json
cmp "${STOPPED_BEFORE}" "${STOPPED_AFTER}" >/dev/null

log ""
log "## dune destroy removes the sandbox and dune up recreates it with persisted state intact"
run_report run_dune "${DUNE_BIN}" destroy --force -p "${PROFILE}"
DESTROY_AFTER="${WORK_DIR}/sbx-ls-after-destroy.json"
capture_report "${DESTROY_AFTER}" sbx ls --json
assert_sandbox_absent "${DESTROY_AFTER}"

SECOND_COMMANDS="${WORK_DIR}/second-shell-commands"
SECOND_UP_OUTPUT="${WORK_DIR}/second-up.out"
cat >"${SECOND_COMMANDS}" <<EOF
test -f /persist/agent/${SENTINEL_FILE}
grep -qx '${SENTINEL_VALUE}' /persist/agent/${SENTINEL_FILE}
pwd > .dune-commands-pwd-after
exit \$?
EOF
run_dune_shell "${SECOND_COMMANDS}" "${SECOND_UP_OUTPUT}"
assert_file_equals "${FIXTURE_ROOT}/.dune-commands-pwd-after" "${FIXTURE_ROOT}"
test -f "${PERSIST_HOST_PATH}/${SENTINEL_FILE}"
assert_file_equals "${PERSIST_HOST_PATH}/${SENTINEL_FILE}" "${SENTINEL_VALUE}"

log ""
log "sbx command smoke passed for ${SANDBOX_NAME} using ${SBX_TEMPLATE_REF}"
