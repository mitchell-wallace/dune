#!/usr/bin/env bash

canonicalize_mode() {
  local raw="${1:-std}"
  local mode
  mode="$(printf '%s' "$raw" | tr '[:upper:]' '[:lower:]')"

  case "$mode" in
    std|standard)
      printf 'std\n'
      ;;
    lax|yolo|strict)
      printf '%s\n' "$mode"
      ;;
    *)
      return 1
      ;;
  esac
}

mode_enabled() {
  local mode="$1"
  local mode_list="$2"

  IFS=',' read -r -a items <<<"$mode_list"
  for item in "${items[@]}"; do
    if [ "$item" = "$mode" ]; then
      return 0
    fi
  done

  return 1
}

normalize_profile() {
  local raw="${1:-0}"
  local profile
  profile="$(printf '%s' "$raw" | tr '[:upper:]' '[:lower:]')"

  if [[ "$profile" =~ ^[0-9a-z]$ ]]; then
    printf '%s\n' "$profile"
    return 0
  fi

  return 1
}

resolve_ipv4s_with_retry() {
  local domain="$1"
  local attempts="${2:-5}"
  local delay_seconds="${3:-1}"
  local dig_ips=""
  local getent_ips=""
  local ips=""

  for _ in $(seq 1 "$attempts"); do
    dig_ips="$(dig +short A "$domain" | grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' || true)"
    getent_ips="$(getent ahostsv4 "$domain" 2>/dev/null | awk '{print $1}' | grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' || true)"
    ips="$(printf '%s\n%s\n' "$dig_ips" "$getent_ips" | grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' | sort -u || true)"
    if [ -n "$ips" ]; then
      printf '%s\n' "$ips"
      return 0
    fi
    sleep "$delay_seconds"
  done

  return 1
}

run_as_target_user() {
  local target_user="${TARGET_USER:?TARGET_USER is required}"
  local target_home="${TARGET_HOME:?TARGET_HOME is required}"
  local path_value="$PATH"
  local npm_prefix="${NPM_CONFIG_PREFIX:-${NPM_PREFIX:-}}"

  if [ -n "$npm_prefix" ]; then
    path_value="${npm_prefix}/bin:${path_value}"
  fi

  if [ -n "${DUNE_TARGET_EXTRA_PATH:-}" ]; then
    path_value="${DUNE_TARGET_EXTRA_PATH}:${path_value}"
  fi

  runuser -u "$target_user" -- env \
    HOME="$target_home" \
    USER="$target_user" \
    LOGNAME="$target_user" \
    PATH="$path_value" \
    XDG_CACHE_HOME="${XDG_CACHE_HOME:-${target_home}/.cache}" \
    XDG_CONFIG_HOME="${XDG_CONFIG_HOME:-${target_home}/.config}" \
    NPM_CONFIG_PREFIX="$npm_prefix" \
    "$@"
}

ensure_mise_available() {
  if ! run_as_target_user bash -lc 'command -v mise >/dev/null 2>&1'; then
    echo "[utils] mise is required but not found for ${TARGET_USER}" >&2
    return 1
  fi
}

install_mise_tool() {
  local tool="$1"
  local version="${2:-latest}"

  if [ "$version" = "latest" ]; then
    run_as_target_user mise use -g "${tool}@latest"
  else
    run_as_target_user mise use -g "${tool}@${version}"
  fi
}

install_npm_global_package() {
  local package_name="$1"
  local version="${2:-latest}"

  if [ "$version" = "latest" ]; then
    run_as_target_user npm install -g "${package_name}@latest"
  else
    run_as_target_user npm install -g "${package_name}@${version}"
  fi
}

ipv4_to_cidr_network() {
  local ip="$1"
  local cidr_bits="$2"

  case "$cidr_bits" in
    32)
      printf '%s/32\n' "$ip"
      ;;
    24)
      awk -F. '{print $1 "." $2 "." $3 ".0/24"}' <<<"$ip"
      ;;
    16)
      awk -F. '{print $1 "." $2 ".0.0/16"}' <<<"$ip"
      ;;
    *)
      return 1
      ;;
  esac
}
