from __future__ import annotations

import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

RALPH_DIR = Path(__file__).resolve().parents[1]
if str(RALPH_DIR) not in sys.path:
    sys.path.insert(0, str(RALPH_DIR))

import tmux_runner  # noqa: E402


class TmuxRunnerTests(unittest.TestCase):
    def test_missing_tmux_binary_fails_fast(self) -> None:
        with patch.object(tmux_runner, "_has_tmux", return_value=False):
            rc = tmux_runner.run_tmux(["start"])
        self.assertEqual(rc, 1)

    def test_invalid_action_prints_usage(self) -> None:
        with patch.object(tmux_runner, "_has_tmux", return_value=True):
            rc = tmux_runner.run_tmux(["frob"])
        self.assertEqual(rc, 1)

    def test_short_form_iteration_defaults_to_start(self) -> None:
        with (
            patch.object(tmux_runner, "_has_tmux", return_value=True),
            patch.object(tmux_runner, "_tmux_has_session", return_value=False),
            patch.object(tmux_runner, "_run", return_value=subprocess.CompletedProcess(args=["tmux"], returncode=0)) as run_mock,
            tempfile.TemporaryDirectory() as tmpdir,
            patch.object(tmux_runner, "LOG_DIR", Path(tmpdir) / ".ralph-logs"),
            patch.object(tmux_runner, "REPO_ROOT", Path(tmpdir)),
        ):
            rc = tmux_runner.run_tmux(["21-30", "cx:2"])

        self.assertEqual(rc, 0)
        tmux_new_session_call = run_mock.call_args_list[-1].args[0]
        self.assertEqual(tmux_new_session_call[0:4], ["tmux", "new-session", "-d", "-s"])
        self.assertIn("ralph.py stream 21-30 cx:2", tmux_new_session_call[-1])

    def test_explicit_tmux_start_simple_session(self) -> None:
        with (
            patch.object(tmux_runner, "_has_tmux", return_value=True),
            patch.object(tmux_runner, "_tmux_has_session", return_value=False),
            patch.object(tmux_runner, "_run", return_value=subprocess.CompletedProcess(args=["tmux"], returncode=0)) as run_mock,
            tempfile.TemporaryDirectory() as tmpdir,
            patch.object(tmux_runner, "LOG_DIR", Path(tmpdir) / ".ralph-logs"),
            patch.object(tmux_runner, "REPO_ROOT", Path(tmpdir)),
        ):
            rc = tmux_runner.run_tmux(["start", "simple", "mysession", "5-8", "cc:1", "cx:2"])

        self.assertEqual(rc, 0)
        tmux_new_session_call = run_mock.call_args_list[-1].args[0]
        self.assertIn("-s", tmux_new_session_call)
        self.assertIn("mysession", tmux_new_session_call)
        self.assertIn("ralph.py simple 5-8 cc:1 cx:2", tmux_new_session_call[-1])

    def test_status_missing_session_returns_one(self) -> None:
        with (
            patch.object(tmux_runner, "_has_tmux", return_value=True),
            patch.object(tmux_runner, "_tmux_has_session", return_value=False),
        ):
            rc = tmux_runner.run_tmux(["status", "does-not-exist"])
        self.assertEqual(rc, 1)


if __name__ == "__main__":
    unittest.main()
