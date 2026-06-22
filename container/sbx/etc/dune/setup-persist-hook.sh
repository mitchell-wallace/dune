#!/usr/bin/env bash
# /etc/dune/setup-persist-hook.sh - Dune login-shell boot hook
#
# Refreshes the /workspace compatibility symlink whenever DUNE_WORKSPACE is
# supplied, then runs setup-persist.sh exactly once per sandbox
# (sentinel-guarded). Logs timestamped start/done/fail lines to
# /var/log/dune/setup-persist.log.
#
# Sourced from /etc/profile.d/dune-setup-persist.sh (bash/sh) and
# /etc/zsh/zprofile (zsh). The sentinel lives in the NON-persisted home
# (~/.dune-setup-persist-done) so sandbox recreation re-runs the hook.
#
# Build-time guard: set DUNE_SETUP_PERSIST_SKIP=1 in docker build RUN steps
# that use login shells (bash -lc) to prevent the hook from firing and baking
# the sentinel into the image layer.

# Skip during image build (the sentinel would be baked into the layer).
[ "${DUNE_SETUP_PERSIST_SKIP:-}" = "1" ] && return 0

# DUNE_WORKSPACE is supplied by the sbx backend on the hook-firing exec:
#   sbx exec -e DUNE_WORKSPACE=<absolute-mounted-repo> <name> bash -lc true
#
# /workspace is the focused workspace bridge. When DUNE_HOST_HOME is supplied
# and the workspace lives under it, also expose a home-relative alias so host
# paths like /home/alice/Documents/app can be reached as ~/Documents/app
# without surfacing the host username in normal sandbox use.
_DUNE_LOG="/var/log/dune/setup-persist.log"

# Ensure log directory exists (should already exist from the image build, but
# guard defensively).
if [ ! -d /var/log/dune ]; then
  sudo install -d -m 0755 -o "$(id -un)" -g "$(id -gn)" /var/log/dune 2>/dev/null || true
fi

if [ -n "${DUNE_WORKSPACE:-}" ]; then
  {
    printf '[%s] workspace: start\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

    case "${DUNE_WORKSPACE}" in
      /*)
        if [ -d "${DUNE_WORKSPACE}" ]; then
          if _DUNE_WORKSPACE_TARGET="$(cd "${DUNE_WORKSPACE}" 2>/dev/null && pwd -P)"; then
            _DUNE_WORKSPACE_READY=1

            if [ -e /workspace ] && [ ! -L /workspace ]; then
              if [ -d /workspace ]; then
                sudo rmdir /workspace 2>&1 || _DUNE_WORKSPACE_READY=0
              else
                sudo rm -f /workspace 2>&1 || _DUNE_WORKSPACE_READY=0
              fi
            fi

            if [ "${_DUNE_WORKSPACE_READY}" = "1" ] \
              && sudo ln -sfnT "${_DUNE_WORKSPACE_TARGET}" /workspace 2>&1; then
              printf '[%s] workspace: linked /workspace -> %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "${_DUNE_WORKSPACE_TARGET}"
            else
              printf '[%s] workspace: FAILED to link /workspace -> %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "${_DUNE_WORKSPACE_TARGET}"
            fi

            _DUNE_HOST_HOME="${DUNE_HOST_HOME:-}"
            _DUNE_HOST_HOME="${_DUNE_HOST_HOME%/}"
            if [ -n "${_DUNE_HOST_HOME}" ]; then
              case "${_DUNE_WORKSPACE_TARGET}" in
                "${_DUNE_HOST_HOME}/"*)
                  _DUNE_WORKSPACE_HOME_REL="${_DUNE_WORKSPACE_TARGET#"${_DUNE_HOST_HOME}/"}"
                  _DUNE_WORKSPACE_HOME_ALIAS="${HOME}/${_DUNE_WORKSPACE_HOME_REL}"
                  if [ -e "${_DUNE_WORKSPACE_HOME_ALIAS}" ] && [ ! -L "${_DUNE_WORKSPACE_HOME_ALIAS}" ]; then
                    printf '[%s] workspace: skipped home alias because path exists: %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "${_DUNE_WORKSPACE_HOME_ALIAS}"
                  elif mkdir -p "$(dirname "${_DUNE_WORKSPACE_HOME_ALIAS}")" 2>&1 \
                    && ln -sfnT "${_DUNE_WORKSPACE_TARGET}" "${_DUNE_WORKSPACE_HOME_ALIAS}" 2>&1; then
                    printf '[%s] workspace: linked %s -> %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "${_DUNE_WORKSPACE_HOME_ALIAS}" "${_DUNE_WORKSPACE_TARGET}"
                  else
                    printf '[%s] workspace: FAILED to link home alias %s -> %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "${_DUNE_WORKSPACE_HOME_ALIAS}" "${_DUNE_WORKSPACE_TARGET}"
                  fi
                  ;;
              esac
            fi
          else
            printf '[%s] workspace: FAILED to resolve DUNE_WORKSPACE: %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "${DUNE_WORKSPACE}"
          fi
        else
          printf '[%s] workspace: FAILED DUNE_WORKSPACE is not a directory: %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "${DUNE_WORKSPACE}"
        fi
        ;;
      *)
        printf '[%s] workspace: FAILED DUNE_WORKSPACE is not absolute: %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "${DUNE_WORKSPACE}"
        ;;
    esac
  } >> "${_DUNE_LOG}"

  unset _DUNE_HOST_HOME _DUNE_WORKSPACE_HOME_ALIAS _DUNE_WORKSPACE_HOME_REL _DUNE_WORKSPACE_TARGET _DUNE_WORKSPACE_READY
fi

# Sentinel check: setup-persist already ran in this sandbox instance.
_DUNE_SENTINEL="${HOME}/.dune-setup-persist-done"
[ -f "${_DUNE_SENTINEL}" ] && { unset _DUNE_SENTINEL _DUNE_LOG; return 0; }

{
  printf '[%s] setup-persist: start\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

  if /bin/bash /usr/local/bin/setup-persist.sh 2>&1; then
    if touch "${_DUNE_SENTINEL}" 2>&1; then
      printf '[%s] setup-persist: done\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
    else
      _rc=$?
      printf '[%s] setup-persist: FAILED to write sentinel (exit %d)\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "${_rc}"
      unset _rc
    fi
  else
    _rc=$?
    printf '[%s] setup-persist: FAILED (exit %d)\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "${_rc}"
    unset _rc
  fi
} >> "${_DUNE_LOG}"

unset _DUNE_SENTINEL _DUNE_LOG
