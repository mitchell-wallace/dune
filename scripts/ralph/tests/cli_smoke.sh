#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
BASH_BIN="$(command -v bash)"
SESSION_INBOX_FILE="$ROOT_DIR/docs/specs/todos/MIT-14/ralph-session-inbox.md"
BATCH_INBOX_FILE="$ROOT_DIR/docs/specs/todos/MIT-14/ralph-batch-inbox.md"

TMP_DIR="$(mktemp -d)"
SESSION_INBOX_BACKUP="$TMP_DIR/ralph-session-inbox.md.bak"
BATCH_INBOX_BACKUP="$TMP_DIR/ralph-batch-inbox.md.bak"
SESSION_INBOX_EXISTS=0
BATCH_INBOX_EXISTS=0
if [[ -f "$SESSION_INBOX_FILE" ]]; then
  cp "$SESSION_INBOX_FILE" "$SESSION_INBOX_BACKUP"
  SESSION_INBOX_EXISTS=1
fi
if [[ -f "$BATCH_INBOX_FILE" ]]; then
  cp "$BATCH_INBOX_FILE" "$BATCH_INBOX_BACKUP"
  BATCH_INBOX_EXISTS=1
fi

cleanup() {
  if [[ "$SESSION_INBOX_EXISTS" -eq 1 ]]; then
    cp "$SESSION_INBOX_BACKUP" "$SESSION_INBOX_FILE"
  else
    rm -f "$SESSION_INBOX_FILE"
  fi
  if [[ "$BATCH_INBOX_EXISTS" -eq 1 ]]; then
    cp "$BATCH_INBOX_BACKUP" "$BATCH_INBOX_FILE"
  else
    rm -f "$BATCH_INBOX_FILE"
  fi
  rm -rf "$TMP_DIR"
  rm -rf "$ROOT_DIR/.ralph-logs"
}

trap cleanup EXIT

FAKE_BIN="$TMP_DIR/fake-bin"
NO_TMUX_BIN="$TMP_DIR/no-tmux-bin"
mkdir -p "$FAKE_BIN" "$NO_TMUX_BIN"

cat > "$FAKE_BIN/git" <<'GIT'
#!/usr/bin/env bash
set -euo pipefail
args=("$@")
if [[ "${args[0]:-}" == "-C" ]]; then
  args=("${args[@]:2}")
fi
case "${args[0]:-}" in
  branch)
    if [[ "${args[1]:-}" == "--show-current" ]]; then
      echo "feat/MIT-14"
      exit 0
    fi
    ;;
  status)
    if [[ "${args[1]:-}" == "--porcelain" ]]; then
      exit 0
    fi
    ;;
  pull|add|commit|push)
    exit 0
    ;;
esac
exit 0
GIT
chmod +x "$FAKE_BIN/git"
cp "$FAKE_BIN/git" "$NO_TMUX_BIN/git"

for agent in claude codex gemini opencode; do
  cat > "$FAKE_BIN/$agent" <<'AGENT'
#!/usr/bin/env bash
set -euo pipefail
exit 0
AGENT
  chmod +x "$FAKE_BIN/$agent"
  cp "$FAKE_BIN/$agent" "$NO_TMUX_BIN/$agent"
done

cat > "$FAKE_BIN/tmux" <<'TMUX'
#!/usr/bin/env bash
set -euo pipefail
STATE_FILE="${RALPH_SMOKE_TMUX_STATE_FILE:?}"
touch "$STATE_FILE"

cmd="${1:-}"
case "$cmd" in
  has-session)
    session="${3:-}"
    if grep -Fxq "$session" "$STATE_FILE"; then
      exit 0
    fi
    exit 1
    ;;
  new-session)
    session=""
    i=1
    while (( i <= $# )); do
      if [[ "${!i}" == "-s" ]]; then
        j=$((i + 1))
        session="${!j}"
      fi
      i=$((i + 1))
    done
    if [[ -z "$session" ]]; then
      exit 1
    fi
    if ! grep -Fxq "$session" "$STATE_FILE"; then
      echo "$session" >> "$STATE_FILE"
    fi
    exit 0
    ;;
  list-sessions)
    while IFS= read -r s; do
      [[ -z "$s" ]] && continue
      echo "$s: 1 windows (created Wed)"
    done < "$STATE_FILE"
    exit 0
    ;;
  kill-session)
    session="${3:-}"
    grep -Fxv "$session" "$STATE_FILE" > "$STATE_FILE.tmp" || true
    mv "$STATE_FILE.tmp" "$STATE_FILE"
    exit 0
    ;;
  attach-session)
    session="${3:-}"
    if grep -Fxq "$session" "$STATE_FILE"; then
      exit 0
    fi
    exit 1
    ;;
  *)
    exit 0
    ;;
esac
TMUX
chmod +x "$FAKE_BIN/tmux"

assert_contains() {
  local haystack="$1"
  local needle="$2"
  if [[ "$haystack" != *"$needle"* ]]; then
    echo "ASSERT FAILED: expected output to contain: $needle"
    echo "Actual output:"
    echo "$haystack"
    exit 1
  fi
}

run_expect_success() {
  local output
  output="$(RALPH_SMOKE_TMUX_STATE_FILE="$TMP_DIR/tmux_state" PATH="$FAKE_BIN:$PATH" "$BASH_BIN" "$ROOT_DIR/ralph.sh" "$@" 2>&1)"
  echo "$output"
}

run_expect_failure() {
  set +e
  local output
  output="$(RALPH_SMOKE_TMUX_STATE_FILE="$TMP_DIR/tmux_state" PATH="$FAKE_BIN:$PATH" "$BASH_BIN" "$ROOT_DIR/ralph.sh" "$@" 2>&1)"
  local rc=$?
  set -e
  if [[ $rc -eq 0 ]]; then
    echo "ASSERT FAILED: expected failure for: $*"
    echo "$output"
    exit 1
  fi
  echo "$output"
}

out="$(run_expect_failure --help)"
assert_contains "$out" "Usage: ./ralph.sh"

out="$(run_expect_success simple 3 cx:2 cc:1)"
assert_contains "$out" "Range: 1-3 (3 runs)"
assert_contains "$out" "Agent mix: codex:2 claude:1"

out="$(run_expect_success stream 2-4)"
assert_contains "$out" "Range: 2-4 (3 runs)"
assert_contains "$out" "Stream-ralph"

out="$(run_expect_success tmux start stream ralph 5-7 cx:1)"
assert_contains "$out" "Started tmux session 'ralph'"
assert_contains "$out" ".ralph-logs/ralph.log"

out="$(run_expect_success tmux status ralph)"
assert_contains "$out" "ralph:"

out="$(run_expect_failure tmux frob)"
assert_contains "$out" "Usage:"

PYTHON_DIR="$(dirname "$(command -v python3)")"
set +e
out="$(RALPH_SMOKE_TMUX_STATE_FILE="$TMP_DIR/tmux_state" PATH="$NO_TMUX_BIN:$PYTHON_DIR" "$BASH_BIN" "$ROOT_DIR/ralph.sh" tmux status ralph 2>&1)"
rc=$?
set -e
if [[ $rc -eq 0 ]]; then
  echo "ASSERT FAILED: expected missing tmux case to fail"
  echo "$out"
  exit 1
fi
assert_contains "$out" "ERROR: tmux is not installed."

echo "CLI smoke tests passed."
