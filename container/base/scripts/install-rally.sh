#!/usr/bin/env bash
set -euo pipefail

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

# rally's upstream installer resolves its version from the GitHub API. That call
# occasionally returns a transient 504, after which the installer proceeds with
# an empty version and 404s on download (curl exit 22), failing the image build.
# Retry the whole fetch+install a few times with backoff so a transient API blip
# doesn't break the build, and fail hard (never silently skip rally) if it still
# does not succeed.
attempts=5
delay=5
for attempt in $(seq 1 "${attempts}"); do
  if curl -fsSL --retry 3 --retry-all-errors \
       https://raw.githubusercontent.com/mitchell-wallace/rally/main/install.sh \
       -o "${tmpdir}/install.sh" \
     && bash "${tmpdir}/install.sh"; then
    exit 0
  fi
  echo "install-rally: attempt ${attempt}/${attempts} failed" >&2
  if [ "${attempt}" -lt "${attempts}" ]; then
    echo "install-rally: retrying in ${delay}s..." >&2
    sleep "${delay}"
    delay=$((delay * 2))
  fi
done

echo "install-rally: failed to install rally after ${attempts} attempts" >&2
exit 1
