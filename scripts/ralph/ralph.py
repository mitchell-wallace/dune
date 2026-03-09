#!/usr/bin/env python3
"""Top-level Ralph CLI entrypoint."""

from __future__ import annotations

import sys
from typing import Sequence

from loop_runner import run_loop
from parsing import detect_top_level_mode, strip_mode_tokens
from tmux_runner import run_tmux


def main(argv: Sequence[str]) -> int:
    mode = detect_top_level_mode(argv)
    remaining = strip_mode_tokens(argv, mode)

    if mode == "simple":
        return run_loop("simple", remaining)

    if mode == "stream":
        return run_loop("stream", remaining)

    if mode == "tmux":
        return run_tmux(remaining)

    print(f"ERROR: Unsupported mode '{mode}'.")
    return 1


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
