#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=test/smoke/lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)/lib.sh"

smoke_init

BUILD_TEMPLATE=0
PROFILE="sbx-egress-smoke"
DOCS_DOMAIN="docs.python.org"
DOCS_URL="https://${DOCS_DOMAIN}/3/"
NESTED_CURL_IMAGE="curlimages/curl:8.10.1"
REPORT_PATH="${REPO_ROOT}/tmp/sbx-egress-smoke.$(date -u +%Y%m%dT%H%M%SZ).log"
TOOLCHAIN_GAPS=0

usage() {
  cat <<'EOF'
Usage: test/smoke/sbx-egress.sh [options]

Options:
  --build-template          Build and load the local dune-sbx template first
  --profile NAME            Dune profile to use for the temporary sandbox
  --docs-domain DOMAIN      Documentation domain expected to be blocked by the baseline
  --docs-url URL            Full documentation URL to request
  --nested-image IMAGE      Nested-Docker curl image to use
  --report PATH             Write command/output evidence to PATH
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
    --docs-domain)
      DOCS_DOMAIN="$2"
      DOCS_URL="https://${DOCS_DOMAIN}/"
      shift 2
      ;;
    --docs-url)
      DOCS_URL="$2"
      shift 2
      ;;
    --nested-image)
      NESTED_CURL_IMAGE="$2"
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
WORK_DIR="$(mktemp -d "${TMP_ROOT}/sbx-egress-smoke.XXXXXX")"
FIXTURE_ROOT="${WORK_DIR}/sample-project"
XDG_CONFIG_HOME="${WORK_DIR}/xdg-config"
XDG_DATA_HOME="${WORK_DIR}/xdg-data"
TZ_VALUE="Australia/Melbourne"
SANDBOX_NAME=""
REPORT_DIR="$(dirname "${REPORT_PATH}")"
mkdir -p "${REPORT_DIR}"
: >"${REPORT_PATH}"

log() {
  printf '%s\n' "$*" | tee -a "${REPORT_PATH}"
}

run_report() {
  local output_path
  local status

  output_path="$(mktemp "${WORK_DIR}/command-output.XXXXXX")"
  log ""
  log "$ $*"
  set +e
  "$@" >"${output_path}" 2>&1
  status=$?
  set -e
  sed 's/^/  /' "${output_path}" | tee -a "${REPORT_PATH}"
  log "[exit ${status}]"
  rm -f "${output_path}"
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
  local command

  command="$(printf '%q ' "${DUNE_BIN}" up -p "${PROFILE}")"
  run_dune script -qec "${command}" /dev/null <"${command_file}"
}

boot_sandbox() {
  local commands="${WORK_DIR}/boot-commands"

  cat >"${commands}" <<'EOF'
pwd > .dune-egress-pwd
exit $?
EOF
  run_report run_dune_shell "${commands}"
}

sandbox_exec() {
  local script="$1"
  shift

  run_report sbx exec "$@" "${SANDBOX_NAME}" bash -lc "${script}"
}

docs_request_script() {
  cat <<'EOF'
set -euo pipefail
code="$(curl -sSL -o /tmp/dune-egress-docs-body -w "%{http_code}" --max-time 30 "${DOCS_URL}" || printf "curl_exit_%s" "$?")"
printf '%s\n' "${code}"
case "${EXPECT}" in
  blocked)
    test "${code}" = "403"
    ;;
  allowed)
    case "${code}" in
      2*|3*) exit 0 ;;
      *) exit 1 ;;
    esac
    ;;
  *)
    echo "unknown EXPECT=${EXPECT}" >&2
    exit 1
    ;;
esac
EOF
}

nested_docs_request_script() {
  cat <<'EOF'
set -euo pipefail
for _ in $(seq 1 30); do
  if docker info >/dev/null 2>&1; then
    break
  fi
  sleep 2
done
docker info >/dev/null
code="$(docker run --rm "${NESTED_CURL_IMAGE}" -sSL -o /dev/null -w "%{http_code}" --max-time 30 "${DOCS_URL}" || printf "curl_exit_%s" "$?")"
printf '%s\n' "${code}"
# Nested Docker traffic is intercepted transparently, so a blocked host fails at
# the TLS layer (curl error, http_code 000) rather than receiving a forward-proxy
# 403. Treat any non-2xx/3xx result as blocked; the policy-log assertion that
# follows confirms sbx recorded the block via the transparent proxy.
case "${code}" in
  2*|3*)
    echo "expected nested request to ${DOCS_URL} to be blocked, got ${code}" >&2
    exit 1
    ;;
esac
EOF
}

toolchain_script() {
  cat <<'EOF'
set -uo pipefail
root=/tmp/dune-egress-toolchain
mkdir -p "${root}/npm" "${root}/pip" "${root}/gomod" "${root}/gocache" "${root}/cargo"
export npm_config_cache="${root}/npm"
export PIP_CACHE_DIR="${root}/pip"
export GOMODCACHE="${root}/gomod"
export GOCACHE="${root}/gocache"
export CARGO_HOME="${root}/cargo"

failures=0
run_check() {
  label="$1"
  shift
  printf '\n## %s\n' "${label}"
  timeout 90 "$@"
  status=$?
  printf 'status=%s\n' "${status}"
  if [ "${status}" -ne 0 ]; then
    failures=$((failures + 1))
  fi
}

run_check npm npm view is-number version --json
run_check pip python -m pip index versions pip --disable-pip-version-check
run_check go go env GOPROXY
run_check go-mod go list -m -versions golang.org/x/text
run_check cargo cargo search serde --limit 1
run_check openai curl -sS -o /dev/null -w "openai http_code=%{http_code}\n" --max-time 30 https://api.openai.com/v1/models
run_check anthropic curl -sS -o /dev/null -w "anthropic http_code=%{http_code}\n" --max-time 30 https://api.anthropic.com/v1/messages
run_check gemini curl -sS -o /dev/null -w "gemini http_code=%{http_code}\n" --max-time 30 https://generativelanguage.googleapis.com/v1beta/models

exit "${failures}"
EOF
}

no_pipelock_script() {
  cat <<'EOF'
set -euo pipefail
# sbx runs its own policy-enforcing egress proxy and advertises it to the sandbox
# via the standard proxy env vars (http://gateway.docker.internal:3128). That
# sbx-managed gateway is expected; what must be gone is Dune's old Pipelock proxy.
# Fail only if a proxy references Pipelock or points anywhere other than the sbx
# gateway.
printenv HTTP_PROXY HTTPS_PROXY http_proxy https_proxy > /tmp/dune-egress-proxy-env 2>/dev/null || true
cat /tmp/dune-egress-proxy-env
if grep -iq pipelock /tmp/dune-egress-proxy-env; then
  echo "proxy env references pipelock" >&2
  exit 1
fi
if grep -vqE '^[[:space:]]*$|gateway\.docker\.internal' /tmp/dune-egress-proxy-env; then
  echo "proxy env points at a non-sbx endpoint (expected only gateway.docker.internal)" >&2
  exit 1
fi
if pgrep -af '[p]ipelock' | awk -v self="$$" '$1 != self { print; found = 1 } END { exit found ? 0 : 1 }'; then
  exit 1
fi
docker ps -a --format '{{.Names}}' > /tmp/dune-egress-docker-containers
cat /tmp/dune-egress-docker-containers
if grep -iq pipelock /tmp/dune-egress-docker-containers; then
  exit 1
fi
EOF
}

wait_for_policy_log_record() {
  local bucket="$1"
  local host="$2"
  local proxy_type="$3"
  local log_json="${WORK_DIR}/policy-log.json"

  for _ in $(seq 1 12); do
    if run_report sbx policy log "${SANDBOX_NAME}" --json --limit 200; then
      sbx policy log "${SANDBOX_NAME}" --json --limit 200 >"${log_json}" 2>/dev/null || true
      if jq -e --arg bucket "${bucket}" --arg host "${host}" --arg proxy "${proxy_type}" \
        '.[$bucket][]? | select(.host == $host and .proxy_type == $proxy)' "${log_json}" >/dev/null; then
        log "Observed ${bucket} record for ${host} via ${proxy_type}"
        return 0
      fi
    fi
    sleep 2
  done

  log "Missing ${bucket} policy-log record for ${host} via ${proxy_type}"
  return 1
}

record_toolchain_policy_findings() {
  local log_json="${WORK_DIR}/policy-log-toolchain.json"
  local missing

  if ! run_report sbx policy log "${SANDBOX_NAME}" --json --limit 300; then
    TOOLCHAIN_GAPS=1
    return 0
  fi
  sbx policy log "${SANDBOX_NAME}" --json --limit 300 >"${log_json}" 2>/dev/null || {
    TOOLCHAIN_GAPS=1
    return 0
  }

  missing="$(jq -r --arg docs "${DOCS_DOMAIN}:443" '.blocked_hosts[]?.host | select(. != $docs)' "${log_json}" | sort -u)"
  if [ -n "${missing}" ]; then
    TOOLCHAIN_GAPS=1
    log ""
    log "Toolchain/provider blocked hosts recorded for docs follow-up:"
    printf '%s\n' "${missing}" | tee -a "${REPORT_PATH}"
  else
    log ""
    log "Toolchain/provider blocked hosts recorded for docs follow-up: none"
  fi

  log ""
  log "Allowed hosts observed during smoke:"
  jq -r '.allowed_hosts[]?.host' "${log_json}" | sort -u | tee -a "${REPORT_PATH}"
}

assert_no_pipelock_yaml() {
  local matches="${WORK_DIR}/pipelock-yaml-matches"

  find "${XDG_CONFIG_HOME}" -name pipelock.yaml -print >"${matches}"
  log ""
  log "$ find ${XDG_CONFIG_HOME} -name pipelock.yaml -print"
  sed 's/^/  /' "${matches}" | tee -a "${REPORT_PATH}"
  if [ -s "${matches}" ]; then
    log "pipelock.yaml was generated under the temporary Dune config"
    return 1
  fi
  log "[exit 0]"
}

for required in sbx docker jq script; do
  if ! command -v "${required}" >/dev/null 2>&1; then
    log "Missing required command on host: ${required}"
    exit 1
  fi
done

log "# sbx egress smoke"
log "report: ${REPORT_PATH}"
log "template: ${SBX_TEMPLATE_REF}"
log "docs domain: ${DOCS_DOMAIN}"
log "docs url: ${DOCS_URL}"
log "nested curl image: ${NESTED_CURL_IMAGE}"

run_report sbx diagnose || {
  log "sbx diagnose failed. This smoke requires an installed, authenticated, healthy sbx daemon."
  exit 1
}
run_report sbx policy log --help
run_report sbx policy allow network --help
run_report sbx policy rm network --help

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

boot_sandbox
run_report sbx policy ls "${SANDBOX_NAME}" --type network

log ""
log "## Baseline block: shell forward traffic"
sandbox_exec "$(docs_request_script)" -e "DOCS_URL=${DOCS_URL}" -e "EXPECT=blocked"
wait_for_policy_log_record blocked_hosts "${DOCS_DOMAIN}:443" forward

log ""
log "## Baseline block: nested Docker transparent traffic"
sandbox_exec "$(nested_docs_request_script)" -e "DOCS_URL=${DOCS_URL}" -e "NESTED_CURL_IMAGE=${NESTED_CURL_IMAGE}"
wait_for_policy_log_record blocked_hosts "${DOCS_DOMAIN}:443" transparent

log ""
log "## Open exact and wildcard docs domains without recreation"
run_report sbx policy allow network --sandbox "${SANDBOX_NAME}" "${DOCS_DOMAIN}:443"
run_report sbx policy allow network --sandbox "${SANDBOX_NAME}" "*.${DOCS_DOMAIN}:443"
sandbox_exec "$(docs_request_script)" -e "DOCS_URL=${DOCS_URL}" -e "EXPECT=allowed"
# An explicitly allowed HTTPS domain skips the MITM forward proxy, so sbx records
# it as "forward-bypass" rather than "forward" (which is reserved for blocked or
# proxied traffic).
wait_for_policy_log_record allowed_hosts "${DOCS_DOMAIN}:443" forward-bypass

log ""
log "## Remove docs rules and confirm re-block"
run_report sbx policy rm network --sandbox "${SANDBOX_NAME}" --resource "${DOCS_DOMAIN}:443"
run_report sbx policy rm network --sandbox "${SANDBOX_NAME}" --resource "*.${DOCS_DOMAIN}:443"
sandbox_exec "$(docs_request_script)" -e "DOCS_URL=${DOCS_URL}" -e "EXPECT=blocked"
wait_for_policy_log_record blocked_hosts "${DOCS_DOMAIN}:443" forward

log ""
log "## Toolchain and provider traffic under the baseline"
if ! sandbox_exec "$(toolchain_script)"; then
  TOOLCHAIN_GAPS=1
  log "One or more toolchain/provider commands failed; policy findings are recorded below."
fi
record_toolchain_policy_findings

log ""
log "## No Pipelock in the sbx egress path"
sandbox_exec "$(no_pipelock_script)"
assert_no_pipelock_yaml

if [ "${TOOLCHAIN_GAPS}" -ne 0 ]; then
  log ""
  log "sbx egress smoke completed with toolchain/provider parity findings; see report above."
else
  log ""
  log "sbx egress smoke passed for ${SANDBOX_NAME} using ${SBX_TEMPLATE_REF}"
fi
