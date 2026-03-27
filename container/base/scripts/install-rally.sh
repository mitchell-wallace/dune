#!/usr/bin/env bash
set -euo pipefail

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "${tmpdir}"
}
trap cleanup EXIT

if curl -fsSL https://raw.githubusercontent.com/mitchell-wallace/rally/main/install.sh -o "${tmpdir}/install.sh"; then
  bash "${tmpdir}/install.sh"
else
  echo "[install-rally] skipping Rally install; installer unavailable" >&2
fi
