from __future__ import annotations

import sys
import unittest
from pathlib import Path
from unittest.mock import patch

RALPH_DIR = Path(__file__).resolve().parents[1]
if str(RALPH_DIR) not in sys.path:
    sys.path.insert(0, str(RALPH_DIR))

import ralph  # noqa: E402


class RalphCliRoutingTests(unittest.TestCase):
    def test_default_tmux_mode_starts_stream(self) -> None:
        with patch.object(ralph, "run_tmux", return_value=0) as tmux_mock:
            rc = ralph.main(["tmux"])
        self.assertEqual(rc, 0)
        tmux_mock.assert_called_once_with([])

    def test_tmux_action_passthrough(self) -> None:
        with patch.object(ralph, "run_tmux", return_value=0) as tmux_mock:
            rc = ralph.main(["tmux", "status", "ralph"])
        self.assertEqual(rc, 0)
        tmux_mock.assert_called_once_with(["status", "ralph"])

    def test_tmux_unknown_action_is_passed_for_usage_handling(self) -> None:
        with patch.object(ralph, "run_tmux", return_value=1) as tmux_mock:
            rc = ralph.main(["tmux", "frob"])
        self.assertEqual(rc, 1)
        tmux_mock.assert_called_once_with(["frob"])

    def test_stream_mode_routes_to_run_loop(self) -> None:
        with patch.object(ralph, "run_loop", return_value=0) as loop_mock:
            rc = ralph.main(["stream", "5", "cx:2"])
        self.assertEqual(rc, 0)
        loop_mock.assert_called_once_with("stream", ["5", "cx:2"])

    def test_simple_mode_routes_to_run_loop(self) -> None:
        with patch.object(ralph, "run_loop", return_value=0) as loop_mock:
            rc = ralph.main(["simple", "5", "cx:2"])
        self.assertEqual(rc, 0)
        loop_mock.assert_called_once_with("simple", ["5", "cx:2"])


if __name__ == "__main__":
    unittest.main()
