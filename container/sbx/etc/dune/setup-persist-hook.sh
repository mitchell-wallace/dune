#!/usr/bin/env bash
# /etc/dune/setup-persist-hook.sh - one-time login-shell boot hook
#
# Runs setup-persist.sh exactly once per sandbox (sentinel-guarded) and logs
# timestamped start/done/fail lines to /var/log/dune/setup-persist.log.
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

# Sentinel check: already ran in this sandbox instance.
_DUNE_SENTINEL="${HOME}/.dune-setup-persist-done"
[ -f "${_DUNE_SENTINEL}" ] && { unset _DUNE_SENTINEL; return 0; }

_DUNE_LOG="/var/log/dune/setup-persist.log"

# Ensure log directory exists (should already exist from the image build, but
# guard defensively).
if [ ! -d /var/log/dune ]; then
  sudo install -d -m 0755 -o "$(id -un)" -g "$(id -gn)" /var/log/dune 2>/dev/null || true
fi

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
