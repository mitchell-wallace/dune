"""Tmux wrapper commands for Ralph automation."""

from __future__ import annotations

import shlex
import shutil
import subprocess
import sys
from pathlib import Path
from typing import Sequence

from config import DEFAULT_END_ITER, LOG_DIR, REPO_ROOT, TMUX_ACTIONS, USAGE_TMUX
from parsing import ParseError, is_agent_spec, is_iteration_spec, parse_tmux_action, parse_tmux_start_args


def _run(argv: Sequence[str], *, check: bool = False, capture_output: bool = False, text: bool = True):
    return subprocess.run(list(argv), check=check, capture_output=capture_output, text=text)


def _has_tmux() -> bool:
    return shutil.which("tmux") is not None


def _tmux_has_session(session: str) -> bool:
    return _run(["tmux", "has-session", "-t", session], check=False).returncode == 0


def _build_start_command(mode: str, iteration_spec: str, agent_args: Sequence[str]) -> str:
    argv = [
        sys.executable,
        str((Path(__file__).resolve().parent / "ralph.py")),
        mode,
        iteration_spec,
        *agent_args,
    ]
    return shlex.join(argv)


def _print_tmux_usage() -> None:
    print(USAGE_TMUX, end="")


def run_tmux(argv: Sequence[str]) -> int:
    if not _has_tmux():
        print("ERROR: tmux is not installed.")
        return 1

    action, remaining = parse_tmux_action(argv)

    if action not in TMUX_ACTIONS:
        _print_tmux_usage()
        return 1

    if action == "start":
        try:
            start_args = parse_tmux_start_args(remaining, default_end_iter=DEFAULT_END_ITER)
        except ParseError as exc:
            print(str(exc))
            return 1

        mode = start_args.mode
        if mode not in {"stream", "simple"}:
            print(f"ERROR: Invalid mode '{mode}'. Use 'stream' or 'simple'.")
            return 1

        session = start_args.session
        if _tmux_has_session(session):
            print(f"Session '{session}' already exists.")
            print(f"Attach with: ./ralph.sh tmux attach {session}")
            return 0

        LOG_DIR.mkdir(parents=True, exist_ok=True)
        log_file = LOG_DIR / f"{session}.log"

        start_cmd = _build_start_command(mode, start_args.iteration_spec, start_args.agent_args)
        tmux_cmd = (
            f"cd {shlex.quote(str(REPO_ROOT))} && {start_cmd} "
            f"2>&1 | tee -a {shlex.quote(str(log_file))}"
        )

        result = _run(["tmux", "new-session", "-d", "-s", session, tmux_cmd], check=False)
        if result.returncode != 0:
            return result.returncode

        print(f"Started tmux session '{session}' running ralph.py {mode} ({start_args.iteration_spec})")
        if start_args.agent_args:
            print(f"Agent mix args: {' '.join(start_args.agent_args)}")
        print(f"Log file: {log_file}")
        print(f"Attach with: ./ralph.sh tmux attach {session}")
        print(f"Monitor with: ./ralph.sh tmux tail {session}")
        return 0

    if action == "attach":
        session = remaining[0] if remaining else "ralph"
        return _run(["tmux", "attach-session", "-t", session], check=False).returncode

    if action == "status":
        session = remaining[0] if remaining else "ralph"
        if not _tmux_has_session(session):
            print(f"Session '{session}' not found.")
            return 1

        listed = _run(["tmux", "list-sessions"], check=True, capture_output=True)
        for line in listed.stdout.splitlines():
            if line.startswith(f"{session}:"):
                print(line)
                break

        log_file = LOG_DIR / f"{session}.log"
        if log_file.exists():
            print()
            print(f"Last 50 log lines ({log_file}):")
            with log_file.open("r", encoding="utf-8", errors="replace") as fh:
                lines = fh.readlines()[-50:]
            for line in lines:
                print(line.rstrip("\n"))
        return 0

    if action == "stop":
        session = remaining[0] if remaining else "ralph"
        rc = _run(["tmux", "kill-session", "-t", session], check=False).returncode
        if rc == 0:
            print(f"Stopped session '{session}'.")
        return rc

    if action == "tail":
        session = remaining[0] if remaining else "ralph"
        log_file = LOG_DIR / f"{session}.log"
        if not log_file.exists():
            print(f"Log file not found for session '{session}': {log_file}")
            return 1
        return _run(["tail", "-n", "50", "-f", str(log_file)], check=False).returncode

    _print_tmux_usage()
    return 1
