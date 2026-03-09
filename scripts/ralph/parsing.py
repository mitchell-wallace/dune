"""Argument parsing helpers for the Ralph automation CLI."""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import Iterable, Sequence

from config import DEFAULT_END_ITER, TMUX_ACTIONS

AGENT_ALIAS_RE = re.compile(r"^(cc|claude|cx|codex|ge|gemini|op|opencode)(:[0-9]+)?$")
ITER_SPEC_RE = re.compile(r"^[0-9]+(?:-[0-9]+)?$")


class ParseError(ValueError):
    """Raised when user-provided CLI args are invalid."""


@dataclass(frozen=True)
class IterationRange:
    start: int
    end: int

    @property
    def total(self) -> int:
        return self.end - self.start + 1


@dataclass(frozen=True)
class AgentMix:
    weights: dict[str, int]
    order: list[str]
    cycle: list[str]
    label: str


@dataclass(frozen=True)
class LoopArgs:
    mode: str
    iteration_spec: str | None
    agent_specs: list[str]


@dataclass(frozen=True)
class TmuxStartArgs:
    mode: str
    session: str
    iteration_spec: str
    agent_args: list[str]


def is_iteration_spec(spec: str | None) -> bool:
    if not spec:
        return False
    return bool(ITER_SPEC_RE.fullmatch(spec))


def is_agent_spec(spec: str | None) -> bool:
    if not spec:
        return False
    return bool(AGENT_ALIAS_RE.fullmatch(spec))


def parse_iteration_spec(spec: str | None, default_end_iter: int = DEFAULT_END_ITER) -> IterationRange:
    raw = spec or str(default_end_iter)
    if not is_iteration_spec(raw):
        raise ParseError(f"ERROR: Invalid iteration spec '{raw}'. Use N or A-B (e.g. 15 or 21-30).")

    if "-" in raw:
        start_s, end_s = raw.split("-", 1)
        start = int(start_s)
        end = int(end_s)
    else:
        start = 1
        end = int(raw)

    if start < 1:
        raise ParseError("ERROR: Start iteration must be >= 1.")
    if end < start:
        raise ParseError("ERROR: End iteration must be >= start iteration.")

    return IterationRange(start=start, end=end)


def _alias_to_agent(alias: str) -> str:
    mapping = {
        "cc": "claude",
        "claude": "claude",
        "cx": "codex",
        "codex": "codex",
        "ge": "gemini",
        "gemini": "gemini",
        "op": "opencode",
        "opencode": "opencode",
    }
    try:
        return mapping[alias]
    except KeyError as exc:
        raise ParseError(
            f"ERROR: Unknown agent alias '{alias}'. Use cc/claude, cx/codex, ge/gemini, or op/opencode."
        ) from exc


def parse_agent_mix(specs: Sequence[str]) -> AgentMix:
    weights = {"claude": 0, "codex": 0, "gemini": 0, "opencode": 0}
    order: list[str] = []

    def add_weight(agent: str, amount: int) -> None:
        if amount < 1:
            raise ParseError("ERROR: Agent weight must be >= 1.")
        if agent not in order:
            order.append(agent)
        weights[agent] += amount

    if not specs:
        add_weight("claude", 1)
        add_weight("codex", 2)
    else:
        for token in specs:
            if not is_agent_spec(token):
                raise ParseError(
                    "ERROR: Unknown agent alias '{token}'. Use cc/claude, cx/codex, ge/gemini, or op/opencode.".format(
                        token=token.split(":", 1)[0]
                    )
                )

            alias = token
            amount = 1
            if ":" in token:
                alias, amount_s = token.split(":", 1)
                if not amount_s.isdigit():
                    raise ParseError(f"ERROR: Invalid weight in agent spec '{token}'.")
                amount = int(amount_s)
                if amount < 1:
                    raise ParseError(f"ERROR: Agent weight must be >= 1 in '{token}'.")

            add_weight(_alias_to_agent(alias), amount)

    cycle: list[str] = []
    for agent in order:
        cycle.extend([agent] * weights[agent])

    if not cycle:
        raise ParseError("ERROR: Empty agent cycle.")

    label_parts = [f"{agent}:{weights[agent]}" for agent in order if weights[agent] > 0]
    return AgentMix(weights=weights, order=order, cycle=cycle, label=" ".join(label_parts))


def agent_for_session(session_num: int, mix: AgentMix) -> str:
    idx = (session_num - 1) % len(mix.cycle)
    return mix.cycle[idx]


def detect_top_level_mode(argv: Sequence[str]) -> str:
    has_tmux = any(arg == "tmux" for arg in argv)
    has_stream = any(arg == "stream" for arg in argv)
    if has_tmux:
        return "tmux"
    if has_stream:
        return "stream"
    return "simple"


def strip_mode_tokens(argv: Sequence[str], mode: str) -> list[str]:
    remaining: list[str] = []
    for arg in argv:
        if mode == "tmux" and arg == "tmux":
            continue
        if mode == "stream" and arg == "stream":
            continue
        if mode == "simple" and arg == "simple":
            continue
        remaining.append(arg)
    return remaining


def parse_loop_args(mode: str, argv: Sequence[str]) -> LoopArgs:
    iter_spec: str | None = None
    agent_specs: list[str] = []

    for arg in argv:
        if is_iteration_spec(arg):
            if iter_spec is not None:
                raise ParseError(f"ERROR: Multiple iteration specs provided ('{iter_spec}' and '{arg}').")
            iter_spec = arg
        elif is_agent_spec(arg):
            agent_specs.append(arg)
        else:
            raise ParseError("loop_usage")

    return LoopArgs(mode=mode, iteration_spec=iter_spec, agent_specs=agent_specs)


def parse_tmux_action(argv: Sequence[str]) -> tuple[str, list[str]]:
    action = argv[0] if argv else "start"

    if is_iteration_spec(action) or action in {"simple", "stream"} or is_agent_spec(action):
        return "start", list(argv)

    return action, list(argv[1:])


def parse_tmux_start_args(argv: Sequence[str], default_end_iter: int = DEFAULT_END_ITER) -> TmuxStartArgs:
    remaining = list(argv)

    mode = "stream"
    session = "ralph"
    iteration_spec = str(default_end_iter)
    agent_args: list[str] = []

    if remaining and remaining[0] in {"simple", "stream"}:
        mode = remaining.pop(0)

    if remaining and not is_iteration_spec(remaining[0]) and not is_agent_spec(remaining[0]):
        session = remaining.pop(0)

    if remaining and is_iteration_spec(remaining[0]):
        iteration_spec = remaining.pop(0)

    while remaining:
        token = remaining.pop(0)
        if is_agent_spec(token):
            agent_args.append(token)
            continue
        raise ParseError(
            f"ERROR: Invalid start argument '{token}'.\n"
            "Expected [agent specs...] after mode/session/iteration arguments."
        )

    return TmuxStartArgs(mode=mode, session=session, iteration_spec=iteration_spec, agent_args=agent_args)


def validate_tmux_action(action: str) -> None:
    if action not in TMUX_ACTIONS:
        raise ParseError("tmux_usage")


def parse_loop_like_args(argv: Iterable[str]) -> tuple[str, list[str]]:
    args = list(argv)
    mode = detect_top_level_mode(args)
    remaining = strip_mode_tokens(args, mode)
    return mode, remaining
