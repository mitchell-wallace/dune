from __future__ import annotations

import sys
import unittest
from pathlib import Path

RALPH_DIR = Path(__file__).resolve().parents[1]
if str(RALPH_DIR) not in sys.path:
    sys.path.insert(0, str(RALPH_DIR))

from parsing import (  # noqa: E402
    ParseError,
    agent_for_session,
    detect_top_level_mode,
    parse_agent_mix,
    parse_iteration_spec,
    parse_loop_args,
    parse_tmux_action,
    parse_tmux_start_args,
    strip_mode_tokens,
)


class IterationParsingTests(unittest.TestCase):
    def test_single_number_defaults_start_to_one(self) -> None:
        iteration = parse_iteration_spec("15")
        self.assertEqual(iteration.start, 1)
        self.assertEqual(iteration.end, 15)
        self.assertEqual(iteration.total, 15)

    def test_range_parses_start_and_end(self) -> None:
        iteration = parse_iteration_spec("21-30")
        self.assertEqual(iteration.start, 21)
        self.assertEqual(iteration.end, 30)
        self.assertEqual(iteration.total, 10)

    def test_invalid_iteration_spec_raises(self) -> None:
        with self.assertRaises(ParseError):
            parse_iteration_spec("bad")


class AgentMixTests(unittest.TestCase):
    def test_default_mix_matches_existing_behavior(self) -> None:
        mix = parse_agent_mix([])
        self.assertEqual(mix.label, "claude:1 codex:2")
        self.assertEqual(mix.cycle, ["claude", "codex", "codex"])

    def test_aliases_and_weights_are_accumulated(self) -> None:
        mix = parse_agent_mix(["cc:1", "cx:2", "codex:1", "ge:1", "op:2"])
        self.assertEqual(mix.weights["claude"], 1)
        self.assertEqual(mix.weights["codex"], 3)
        self.assertEqual(mix.weights["gemini"], 1)
        self.assertEqual(mix.weights["opencode"], 2)
        self.assertEqual(mix.order, ["claude", "codex", "gemini", "opencode"])

    def test_agent_for_session_uses_cycle_indexing(self) -> None:
        mix = parse_agent_mix(["cc:1", "cx:2"])
        self.assertEqual(agent_for_session(1, mix), "claude")
        self.assertEqual(agent_for_session(2, mix), "codex")
        self.assertEqual(agent_for_session(3, mix), "codex")
        self.assertEqual(agent_for_session(4, mix), "claude")

    def test_invalid_agent_spec_raises(self) -> None:
        with self.assertRaises(ParseError):
            parse_agent_mix(["unknown"])


class TopLevelRoutingTests(unittest.TestCase):
    def test_mode_precedence_tmux_then_stream_then_simple(self) -> None:
        self.assertEqual(detect_top_level_mode(["stream", "tmux"]), "tmux")
        self.assertEqual(detect_top_level_mode(["stream", "10"]), "stream")
        self.assertEqual(detect_top_level_mode(["10"]), "simple")

    def test_strip_mode_tokens_only_removes_selected_mode_token(self) -> None:
        self.assertEqual(strip_mode_tokens(["tmux", "stream", "10"], "tmux"), ["stream", "10"])
        self.assertEqual(strip_mode_tokens(["stream", "10"], "stream"), ["10"])
        self.assertEqual(strip_mode_tokens(["simple", "10"], "simple"), ["10"])

    def test_loop_args_reject_multiple_iteration_specs(self) -> None:
        with self.assertRaises(ParseError):
            parse_loop_args("simple", ["1-2", "3-4"])


class TmuxParsingTests(unittest.TestCase):
    def test_tmux_action_short_form_defaults_to_start(self) -> None:
        action, remaining = parse_tmux_action(["21-30", "cx:2"])
        self.assertEqual(action, "start")
        self.assertEqual(remaining, ["21-30", "cx:2"])

    def test_tmux_action_returns_explicit_action(self) -> None:
        action, remaining = parse_tmux_action(["status", "ralph"])
        self.assertEqual(action, "status")
        self.assertEqual(remaining, ["ralph"])

    def test_tmux_start_parse_for_short_examples(self) -> None:
        parsed = parse_tmux_start_args(["21-30", "cx:2"])
        self.assertEqual(parsed.mode, "stream")
        self.assertEqual(parsed.session, "ralph")
        self.assertEqual(parsed.iteration_spec, "21-30")
        self.assertEqual(parsed.agent_args, ["cx:2"])

        parsed = parse_tmux_start_args(["stream", "21-30"])
        self.assertEqual(parsed.mode, "stream")
        self.assertEqual(parsed.session, "ralph")
        self.assertEqual(parsed.iteration_spec, "21-30")

        parsed = parse_tmux_start_args(["simple", "mysession", "5-8", "cc:1", "cx:2"])
        self.assertEqual(parsed.mode, "simple")
        self.assertEqual(parsed.session, "mysession")
        self.assertEqual(parsed.iteration_spec, "5-8")
        self.assertEqual(parsed.agent_args, ["cc:1", "cx:2"])


if __name__ == "__main__":
    unittest.main()
