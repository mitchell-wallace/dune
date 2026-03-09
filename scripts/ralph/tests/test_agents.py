from __future__ import annotations

import sys
import unittest
from pathlib import Path

RALPH_DIR = Path(__file__).resolve().parents[1]
if str(RALPH_DIR) not in sys.path:
    sys.path.insert(0, str(RALPH_DIR))

from agents import build_agent_command  # noqa: E402


class AgentCommandTests(unittest.TestCase):
    def test_claude_simple_command(self) -> None:
        cmd = build_agent_command("claude", "simple", "PROMPT")
        self.assertEqual(
            cmd.argv,
            [
                "claude",
                "-p",
                "--dangerously-skip-permissions",
                "--output-format",
                "text",
                "PROMPT",
            ],
        )

    def test_codex_simple_suppresses_stderr(self) -> None:
        cmd = build_agent_command("codex", "simple", "PROMPT")
        self.assertTrue(cmd.suppress_stderr)
        self.assertEqual(cmd.argv[0:3], ["codex", "exec", "--dangerously-bypass-approvals-and-sandbox"])

    def test_gemini_stream_command(self) -> None:
        cmd = build_agent_command("gemini", "stream", "PROMPT")
        self.assertEqual(
            cmd.argv,
            ["gemini", "--prompt", "PROMPT", "--yolo", "--output-format", "stream-json"],
        )

    def test_opencode_stream_command(self) -> None:
        cmd = build_agent_command("opencode", "stream", "PROMPT")
        self.assertEqual(cmd.argv, ["opencode", "run", "--format", "json", "PROMPT"])

    def test_unknown_agent_raises(self) -> None:
        with self.assertRaises(ValueError):
            build_agent_command("unknown", "simple", "PROMPT")


if __name__ == "__main__":
    unittest.main()
