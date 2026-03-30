#!/usr/bin/env bash
set -euo pipefail

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "${tmpdir}"
}
trap cleanup EXIT

curl -fsSL https://raw.githubusercontent.com/mitchell-wallace/rally/main/install.sh -o "${tmpdir}/install.sh"
bash "${tmpdir}/install.sh"
