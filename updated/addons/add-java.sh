#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "[add-java] must run as root" >&2
  exit 1
fi

TARGET_USER="${SAND_TARGET_USER:-node}"
TARGET_HOME="${SAND_TARGET_HOME:-/home/${TARGET_USER}}"
JAVA_VERSION="${SAND_JAVA_VERSION:-temurin}"
MAVEN_VERSION="${SAND_MAVEN_VERSION:-latest}"
GRADLE_VERSION="${SAND_GRADLE_VERSION:-latest}"

log() {
  echo "[add-java] $*"
}

run_as_target_user() {
  runuser -u "$TARGET_USER" -- env \
    HOME="$TARGET_HOME" \
    USER="$TARGET_USER" \
    LOGNAME="$TARGET_USER" \
    PATH="${TARGET_HOME}/.local/bin:${TARGET_HOME}/.local/share/mise/shims:${PATH}" \
    "$@"
}

if ! run_as_target_user command -v mise >/dev/null 2>&1; then
  echo "[add-java] mise is required but not found for ${TARGET_USER}" >&2
  exit 1
fi

log "Installing Java via mise (${JAVA_VERSION})"
if [ "$JAVA_VERSION" = "latest" ] || [ "$JAVA_VERSION" = "temurin" ]; then
  run_as_target_user mise use -g java@temurin
else
  run_as_target_user mise use -g "java@${JAVA_VERSION}"
fi

log "Installing Maven via mise (${MAVEN_VERSION})"
if [ "$MAVEN_VERSION" = "latest" ]; then
  run_as_target_user mise use -g maven@latest
else
  run_as_target_user mise use -g "maven@${MAVEN_VERSION}"
fi

log "Installing Gradle via mise (${GRADLE_VERSION})"
if [ "$GRADLE_VERSION" = "latest" ]; then
  run_as_target_user mise use -g gradle@latest
else
  run_as_target_user mise use -g "gradle@${GRADLE_VERSION}"
fi

run_as_target_user mise reshim

log "Done. Verify with 'java -version', 'mvn -version', and 'gradle -version'."
