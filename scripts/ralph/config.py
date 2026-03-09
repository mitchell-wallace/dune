"""Configuration constants for the Ralph automation CLI."""

from __future__ import annotations

from pathlib import Path

SCRIPT_DIR = Path(__file__).resolve().parent
REPO_ROOT = SCRIPT_DIR.parent.parent

PLAN_DIR = Path("docs/specs/todos/MIT-14")
PROGRESS_FILE = PLAN_DIR / "ralph-progress.md"
SESSION_INBOX_FILE = PLAN_DIR / "ralph-session-inbox.md"
BATCH_INBOX_FILE = PLAN_DIR / "ralph-batch-inbox.md"
BRANCH = "feat/MIT-14"
DEFAULT_END_ITER = 20
PROMPT_FILE = SCRIPT_DIR / "prompt.txt"
LOG_DIR = REPO_ROOT / ".ralph-logs"

PROGRESS_FILE_INIT = """# MIT-14 Ralph Progress

Automated progress log for the MIT-14 agent loop.

---

"""

SESSION_INBOX_FILE_INIT = """---
title: Ralph Session Inbox
scope: docs/specs/todos/MIT-14
how_to: |
  Add one checkbox message. Ralph consumes at most one unchecked message per session, top-down.
  The consumed message is marked [X] and annotated with a sent comment.
priority: Inbox instructions should be prioritised over default task picking.
example_unchecked: |
  - [ ] Prioritise stabilizing sync tests before taking new feature work.
    Context: focus on flaky pull/full-resync flow first.
  ---
example_checked: |
  <!-- Sent to Session 23, Codex, 2026-03-04 13:00:00 -->
  - [X] Prioritise stabilizing sync tests before taking new feature work.
    Context: focus on flaky pull/full-resync flow first.
  ---
---

# MIT-14 Ralph Session Inbox

Add one message block at a time. Message content starts at `- [ ]` and continues until the next `---` line.
Each message is sent to exactly one session.

- [X] Replace this example with your next instruction for Ralph.
  Include any context needed for the next session.
---
"""

BATCH_INBOX_FILE_INIT = """---
title: Ralph Batch Inbox
scope: docs/specs/todos/MIT-14
how_to: |
  Add one checkbox message. Ralph consumes at most one unchecked message per batch, top-down.
  The consumed message is sent to EVERY session in the batch, then marked [X].
priority: Batch inbox instructions should be prioritised over default task picking.
example_unchecked: |
  - [ ] All sessions this batch should focus on test coverage.
  ---
example_checked: |
  <!-- Sent to Batch 21-30, Codex, 2026-03-04 13:00:00 -->
  - [X] All sessions this batch should focus on test coverage.
  ---
---

# MIT-14 Ralph Batch Inbox

Add one message block at a time. Message content starts at `- [ ]` and continues until the next `---` line.
Each message is sent to every session in a single batch run.

- [X] Replace this example with your next batch instruction for Ralph.
  Include any context that applies to all sessions in the batch.
---
"""

TMUX_ACTIONS = {"start", "attach", "status", "stop", "tail"}
VALID_MODES = {"simple", "stream"}

USAGE_LOOP = (
    "Usage: ./ralph.sh [simple|stream] [N|A-B] "
    "[cc|claude|cx|codex|ge|gemini|op|opencode|"
    "cc:N|claude:N|cx:N|codex:N|ge:N|gemini:N|op:N|opencode:N ...]"
)

USAGE_LOOP_EXAMPLE = "Example: ./ralph.sh simple 21-30 cx:3 cc:1"

USAGE_TMUX = """Usage:
  ./ralph.sh tmux start [stream|simple] [session-name] [N|A-B] [cc|claude|cx|codex|ge|gemini|op|opencode|cc:N|claude:N|cx:N|codex:N|ge:N|gemini:N|op:N|opencode:N ...]
  ./ralph.sh tmux attach [session-name]
  ./ralph.sh tmux status [session-name]
  ./ralph.sh tmux stop [session-name]
  ./ralph.sh tmux tail [session-name]
  ./ralph.sh tmux [N|A-B] [agent specs...]
  ./ralph.sh tmux simple [N|A-B] [agent specs...]
  ./ralph.sh tmux stream [N|A-B] [agent specs...]
"""
