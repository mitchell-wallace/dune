"""Main Ralph run-loop implementation for simple and stream modes."""

from __future__ import annotations

import re
import subprocess
import sys
from datetime import datetime
from pathlib import Path
from typing import Sequence

from agents import build_agent_command
from config import (
    BATCH_INBOX_FILE,
    BATCH_INBOX_FILE_INIT,
    BRANCH,
    DEFAULT_END_ITER,
    PROMPT_FILE,
    PROGRESS_FILE,
    PROGRESS_FILE_INIT,
    REPO_ROOT,
    SESSION_INBOX_FILE,
    SESSION_INBOX_FILE_INIT,
    USAGE_LOOP,
    USAGE_LOOP_EXAMPLE,
)
from parsing import ParseError, agent_for_session, parse_agent_mix, parse_iteration_spec, parse_loop_args

INBOX_UNCHECKED_RE = re.compile(r"^(\s*[-*]\s*)\[\s\](.*)$")
AGENT_LABELS = {
    "claude": "Claude",
    "codex": "Codex",
    "gemini": "Gemini",
    "opencode": "OpenCode",
}


def _now_str() -> str:
    return datetime.now().strftime("%Y-%m-%d %H:%M:%S")


def _run(argv: Sequence[str], *, cwd: Path = REPO_ROOT, check: bool = False, stderr=None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        list(argv),
        cwd=str(cwd),
        check=check,
        text=True,
        stderr=stderr,
    )


def _git_output(argv: Sequence[str]) -> str:
    result = subprocess.run(
        ["git", "-C", str(REPO_ROOT), *argv],
        check=True,
        text=True,
        capture_output=True,
    )
    return result.stdout.strip()


def ensure_on_branch() -> None:
    current_branch = _git_output(["branch", "--show-current"])
    if current_branch != BRANCH:
        raise RuntimeError(f"ERROR: Expected branch '{BRANCH}', but on '{current_branch}'. Aborting.")


def ensure_progress_file() -> None:
    abs_progress = REPO_ROOT / PROGRESS_FILE
    if abs_progress.exists():
        return

    abs_progress.parent.mkdir(parents=True, exist_ok=True)
    abs_progress.write_text(PROGRESS_FILE_INIT, encoding="utf-8")

    _run(["git", "-C", str(REPO_ROOT), "add", str(PROGRESS_FILE)], check=True)
    _run(
        [
            "git",
            "-C",
            str(REPO_ROOT),
            "commit",
            "-m",
            "chore(ralph): initialize progress tracking file",
        ],
        check=True,
    )


def _ensure_inbox(inbox_file: Path, init_content: str, label: str) -> None:
    abs_inbox = REPO_ROOT / inbox_file
    if abs_inbox.exists():
        return

    abs_inbox.parent.mkdir(parents=True, exist_ok=True)
    abs_inbox.write_text(init_content, encoding="utf-8")

    _run(["git", "-C", str(REPO_ROOT), "add", str(inbox_file)], check=True)
    _run(
        [
            "git",
            "-C",
            str(REPO_ROOT),
            "commit",
            "-m",
            f"chore(ralph): initialize {label} inbox file",
        ],
        check=True,
    )


def ensure_session_inbox_file() -> None:
    _ensure_inbox(SESSION_INBOX_FILE, SESSION_INBOX_FILE_INIT, "session")


def ensure_batch_inbox_file() -> None:
    _ensure_inbox(BATCH_INBOX_FILE, BATCH_INBOX_FILE_INIT, "batch")


def load_base_prompt() -> str:
    if not PROMPT_FILE.exists():
        raise RuntimeError(f"ERROR: Prompt file not found: {PROMPT_FILE}")
    return PROMPT_FILE.read_text(encoding="utf-8")


def print_loop_usage() -> None:
    print(USAGE_LOOP)
    print(USAGE_LOOP_EXAMPLE)


def _commit_leftovers(session_num: int) -> None:
    status = _git_output(["status", "--porcelain"])
    if not status:
        return

    print("  Committing leftover uncommitted changes...")
    _run(["git", "add", "-A"], check=True)
    _run(
        [
            "git",
            "commit",
            "-m",
            f"chore(mit-14): auto-commit uncommitted changes after session {session_num}",
        ],
        check=True,
    )
    _run(["git", "push", "origin", BRANCH], check=True)


def _extract_recent_progress_context(max_sections: int = 4) -> str:
    progress_path = REPO_ROOT / PROGRESS_FILE
    if not progress_path.exists():
        return ""

    lines = progress_path.read_text(encoding="utf-8").splitlines()
    heading_indexes = [idx for idx, line in enumerate(lines) if line.startswith("## ")]

    if heading_indexes:
        start_idx = heading_indexes[max(0, len(heading_indexes) - max_sections)]
        excerpt_lines = lines[start_idx:]
    else:
        excerpt_lines = lines[-200:]

    return "\n".join(excerpt_lines).strip()


def _frontmatter_end_index(lines: list[str]) -> int:
    if not lines or lines[0].strip() != "---":
        return -1

    for idx in range(1, len(lines)):
        if lines[idx].strip() == "---":
            return idx
    return -1


def _consume_next_inbox_message(inbox_file: Path, sent_comment: str, commit_message: str) -> str | None:
    inbox_path = REPO_ROOT / inbox_file
    if not inbox_path.exists():
        return None

    original = inbox_path.read_text(encoding="utf-8")
    lines = original.splitlines()

    frontmatter_end = _frontmatter_end_index(lines)
    scan_start = frontmatter_end + 1 if frontmatter_end >= 0 else 0

    start_idx = -1
    for idx in range(scan_start, len(lines)):
        line = lines[idx]
        if INBOX_UNCHECKED_RE.match(line):
            start_idx = idx
            break

    if start_idx < 0:
        return None

    end_idx = len(lines)
    for idx in range(start_idx + 1, len(lines)):
        if lines[idx].strip() == "---":
            end_idx = idx
            break

    message_block = "\n".join(lines[start_idx:end_idx]).strip()
    if not message_block:
        return None

    lines[start_idx] = INBOX_UNCHECKED_RE.sub(r"\1[X]\2", lines[start_idx], count=1)
    lines.insert(start_idx, f"<!-- {sent_comment} -->")

    updated = "\n".join(lines)
    if original.endswith("\n"):
        updated += "\n"
    inbox_path.write_text(updated, encoding="utf-8")

    _run(["git", "-C", str(REPO_ROOT), "add", str(inbox_file)], check=True)
    _run(["git", "-C", str(REPO_ROOT), "commit", "-m", commit_message], check=True)

    return message_block


def _consume_next_session_inbox_message(session_num: int, agent_name: str, timestamp: str) -> str | None:
    agent_label = AGENT_LABELS.get(agent_name, agent_name)
    comment = f"Sent to Session {session_num}, {agent_label}, {timestamp}"
    commit_msg = f"chore(ralph): consume session inbox message for session {session_num}"
    return _consume_next_inbox_message(SESSION_INBOX_FILE, comment, commit_msg)


def _consume_next_batch_inbox_message(iteration_label: str, agent_name: str, timestamp: str) -> str | None:
    agent_label = AGENT_LABELS.get(agent_name, agent_name)
    comment = f"Sent to Batch {iteration_label}, {agent_label}, {timestamp}"
    commit_msg = f"chore(ralph): consume batch inbox message for batch {iteration_label}"
    return _consume_next_inbox_message(BATCH_INBOX_FILE, comment, commit_msg)


def build_session_prompt(
    base_prompt: str,
    session_num: int,
    agent_name: str,
    timestamp: str,
    batch_inbox_message: str | None = None,
) -> str:
    recent_progress = _extract_recent_progress_context(max_sections=4)
    session_inbox_message = _consume_next_session_inbox_message(
        session_num=session_num, agent_name=agent_name, timestamp=timestamp,
    )

    parts = [base_prompt.strip(), ""]
    parts.append("## Deterministic Ralph Context")
    parts.append("The following context is injected by Ralph. Use it directly in this session.")
    parts.append("")
    parts.append("### Recent Progress (last 4 `##` sections from `ralph-progress.md`)")
    parts.append(recent_progress if recent_progress else "_No progress context available._")
    parts.append("")
    parts.append("### Batch Inbox (Priority — applies to all sessions in this batch)")
    if batch_inbox_message:
        parts.append("Prioritise this batch-level instruction across the entire batch.")
        parts.append("")
        parts.append(batch_inbox_message)
    else:
        parts.append("_No pending batch inbox instruction._")
    parts.append("")
    parts.append("### Session Inbox (Priority — applies to this session only)")
    if session_inbox_message:
        parts.append("Prioritise this session-level instruction for this session before default task selection.")
        parts.append("")
        parts.append(session_inbox_message)
    else:
        parts.append("_No pending session inbox instruction._")
    parts.append("")
    return "\n".join(parts)


def run_loop(mode: str, argv: Sequence[str]) -> int:
    if mode not in {"simple", "stream"}:
        print(f"ERROR: Invalid mode '{mode}'. Use 'simple' or 'stream'.")
        return 1

    try:
        loop_args = parse_loop_args(mode, argv)
    except ParseError as exc:
        if str(exc) == "loop_usage":
            print_loop_usage()
            return 1
        print(str(exc))
        return 1

    try:
        iteration = parse_iteration_spec(loop_args.iteration_spec, default_end_iter=DEFAULT_END_ITER)
        mix = parse_agent_mix(loop_args.agent_specs)
        ensure_on_branch()
        ensure_progress_file()
        ensure_session_inbox_file()
        ensure_batch_inbox_file()
        base_prompt = load_base_prompt()
    except (ParseError, RuntimeError, subprocess.CalledProcessError) as exc:
        print(str(exc))
        return 1

    title = "Simple-ralph" if mode == "simple" else "Stream-ralph"

    iteration_label = f"{iteration.start}-{iteration.end}"
    batch_start = _now_str()
    batch_inbox_message = _consume_next_batch_inbox_message(
        iteration_label=iteration_label,
        agent_name=mix.label,
        timestamp=batch_start,
    )

    print("========================================")
    print(f"  {title} - MIT-14 Agent Loop")
    print(f"  Range: {iteration_label} ({iteration.total} runs)")
    print(f"  Agent mix: {mix.label}")
    if batch_inbox_message:
        print(f"  Batch inbox: ✓ (message consumed)")
    print("========================================")
    print()

    run_index = 1
    for session_num in range(iteration.start, iteration.end + 1):
        print()
        print("----------------------------------------")
        print(f"  Session {session_num} ({run_index}/{iteration.total})")
        print("----------------------------------------")

        subprocess.run(
            ["git", "pull", "--rebase", "origin", BRANCH],
            cwd=str(REPO_ROOT),
            check=False,
            stderr=subprocess.DEVNULL,
        )

        agent_name = agent_for_session(session_num, mix)
        session_start = _now_str()

        try:
            prompt = build_session_prompt(
                base_prompt=base_prompt,
                session_num=session_num,
                agent_name=agent_name,
                timestamp=session_start,
                batch_inbox_message=batch_inbox_message,
            )
        except RuntimeError as exc:
            print(str(exc))
            return 1

        try:
            command = build_agent_command(agent_name, mode, prompt)
        except ValueError as exc:
            print(f"ERROR: {exc}")
            return 1

        print(f"  Agent: {command.display_name}")
        print(f"  Starting at {session_start}")
        print()

        try:
            result = _run(
                command.argv,
                check=False,
                stderr=subprocess.DEVNULL if command.suppress_stderr else None,
            )
            exit_code = result.returncode
        except FileNotFoundError:
            print(f"ERROR: Command not found: {command.argv[0]}")
            exit_code = 127

        print()
        print(f"  Agent exited with code {exit_code} at {_now_str()}")

        try:
            _commit_leftovers(session_num)
        except subprocess.CalledProcessError as exc:
            print(f"ERROR: Failed git cleanup after session {session_num}: {exc}")
            return 1

        print(f"  Session {session_num} complete.")
        run_index += 1

    print()
    print("========================================")
    print(f"  Completed sessions {iteration.start}-{iteration.end}.")
    print("========================================")
    return 0


def main(argv: Sequence[str]) -> int:
    mode = argv[0] if argv else "simple"
    remaining = list(argv[1:])
    return run_loop(mode, remaining)


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
