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

import loop_runner  # noqa: E402
from agents import AgentCommand  # noqa: E402


class LoopRunnerFlowTests(unittest.TestCase):
    def test_ensure_on_branch_rejects_wrong_branch(self) -> None:
        with patch.object(loop_runner, "_git_output", return_value="main"):
            with self.assertRaises(RuntimeError):
                loop_runner.ensure_on_branch()

    def test_ensure_progress_file_bootstraps_and_commits(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            progress_rel = Path("test-progress.md")

            with (
                patch.object(loop_runner, "REPO_ROOT", repo_root),
                patch.object(loop_runner, "PROGRESS_FILE", progress_rel),
                patch.object(loop_runner, "_run") as run_mock,
            ):
                loop_runner.ensure_progress_file()

            progress_abs = repo_root / progress_rel
            self.assertTrue(progress_abs.exists())
            self.assertIn("# MIT-14 Ralph Progress", progress_abs.read_text(encoding="utf-8"))

            self.assertEqual(run_mock.call_count, 2)
            add_call = run_mock.call_args_list[0].args[0]
            commit_call = run_mock.call_args_list[1].args[0]
            self.assertIn("add", add_call)
            self.assertIn("commit", commit_call)

    def test_ensure_session_inbox_file_bootstraps_and_commits(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            inbox_rel = Path("test-session-inbox.md")

            with (
                patch.object(loop_runner, "REPO_ROOT", repo_root),
                patch.object(loop_runner, "SESSION_INBOX_FILE", inbox_rel),
                patch.object(loop_runner, "_run") as run_mock,
            ):
                loop_runner.ensure_session_inbox_file()

            inbox_abs = repo_root / inbox_rel
            self.assertTrue(inbox_abs.exists())
            self.assertIn("# MIT-14 Ralph Session Inbox", inbox_abs.read_text(encoding="utf-8"))

            self.assertEqual(run_mock.call_count, 2)
            add_call = run_mock.call_args_list[0].args[0]
            commit_call = run_mock.call_args_list[1].args[0]
            self.assertIn("add", add_call)
            self.assertIn("commit", commit_call)

    def test_ensure_batch_inbox_file_bootstraps_and_commits(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            inbox_rel = Path("test-batch-inbox.md")

            with (
                patch.object(loop_runner, "REPO_ROOT", repo_root),
                patch.object(loop_runner, "BATCH_INBOX_FILE", inbox_rel),
                patch.object(loop_runner, "_run") as run_mock,
            ):
                loop_runner.ensure_batch_inbox_file()

            inbox_abs = repo_root / inbox_rel
            self.assertTrue(inbox_abs.exists())
            self.assertIn("# MIT-14 Ralph Batch Inbox", inbox_abs.read_text(encoding="utf-8"))

            self.assertEqual(run_mock.call_count, 2)
            add_call = run_mock.call_args_list[0].args[0]
            commit_call = run_mock.call_args_list[1].args[0]
            self.assertIn("add", add_call)
            self.assertIn("commit", commit_call)

    def test_commit_leftovers_runs_add_commit_and_push(self) -> None:
        with (
            patch.object(loop_runner, "_git_output", return_value=" M apps/frontend/src/foo.ts"),
            patch.object(loop_runner, "_run") as run_mock,
        ):
            loop_runner._commit_leftovers(7)

        self.assertEqual(run_mock.call_count, 3)
        self.assertEqual(run_mock.call_args_list[0].args[0], ["git", "add", "-A"])
        self.assertEqual(
            run_mock.call_args_list[1].args[0],
            [
                "git",
                "commit",
                "-m",
                "chore(mit-14): auto-commit uncommitted changes after session 7",
            ],
        )
        self.assertEqual(run_mock.call_args_list[2].args[0], ["git", "push", "origin", loop_runner.BRANCH])

    def test_extract_recent_progress_context_uses_last_four_l2_sections(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            progress_rel = Path("docs/specs/todos/MIT-14/ralph-progress.md")
            progress_abs = repo_root / progress_rel
            progress_abs.parent.mkdir(parents=True, exist_ok=True)
            progress_abs.write_text(
                "\n".join(
                    [
                        "# Title",
                        "",
                        "## Session 1",
                        "one",
                        "## Session 2",
                        "two",
                        "## Session 3",
                        "three",
                        "## Session 4",
                        "four",
                        "## Session 5",
                        "five",
                    ]
                )
                + "\n",
                encoding="utf-8",
            )

            with (
                patch.object(loop_runner, "REPO_ROOT", repo_root),
                patch.object(loop_runner, "PROGRESS_FILE", progress_rel),
            ):
                excerpt = loop_runner._extract_recent_progress_context(max_sections=4)

            self.assertNotIn("## Session 1", excerpt)
            self.assertIn("## Session 2", excerpt)
            self.assertIn("## Session 5", excerpt)

    def test_consume_next_session_inbox_message_marks_and_comments_first_item(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            inbox_rel = Path("docs/specs/todos/MIT-14/ralph-session-inbox.md")
            inbox_abs = repo_root / inbox_rel
            inbox_abs.parent.mkdir(parents=True, exist_ok=True)
            inbox_abs.write_text(
                "\n".join(
                    [
                        "# Inbox",
                        "",
                        "- [ ] First instruction",
                        "  Context line",
                        "---",
                        "",
                        "- [ ] Second instruction",
                        "---",
                    ]
                )
                + "\n",
                encoding="utf-8",
            )

            with (
                patch.object(loop_runner, "REPO_ROOT", repo_root),
                patch.object(loop_runner, "SESSION_INBOX_FILE", inbox_rel),
                patch.object(loop_runner, "_run") as run_mock,
            ):
                message = loop_runner._consume_next_session_inbox_message(
                    session_num=23,
                    agent_name="codex",
                    timestamp="2026-03-04 13:15:00",
                )

            self.assertEqual(message, "- [ ] First instruction\n  Context line")
            updated = inbox_abs.read_text(encoding="utf-8")
            self.assertIn("<!-- Sent to Session 23, Codex, 2026-03-04 13:15:00 -->", updated)
            self.assertIn("- [X] First instruction", updated)
            self.assertIn("- [ ] Second instruction", updated)

            self.assertEqual(run_mock.call_count, 2)
            self.assertIn("add", run_mock.call_args_list[0].args[0])
            commit_call = run_mock.call_args_list[1].args[0]
            self.assertIn("commit", commit_call)
            self.assertIn("chore(ralph):", commit_call[-1])

    def test_consume_next_batch_inbox_message_marks_and_comments(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            inbox_rel = Path("docs/specs/todos/MIT-14/ralph-batch-inbox.md")
            inbox_abs = repo_root / inbox_rel
            inbox_abs.parent.mkdir(parents=True, exist_ok=True)
            inbox_abs.write_text(
                "\n".join(
                    [
                        "# Batch Inbox",
                        "",
                        "- [ ] Focus on test coverage this batch",
                        "---",
                    ]
                )
                + "\n",
                encoding="utf-8",
            )

            with (
                patch.object(loop_runner, "REPO_ROOT", repo_root),
                patch.object(loop_runner, "BATCH_INBOX_FILE", inbox_rel),
                patch.object(loop_runner, "_run") as run_mock,
            ):
                message = loop_runner._consume_next_batch_inbox_message(
                    iteration_label="21-30",
                    agent_name="codex",
                    timestamp="2026-03-04 13:15:00",
                )

            self.assertEqual(message, "- [ ] Focus on test coverage this batch")
            updated = inbox_abs.read_text(encoding="utf-8")
            self.assertIn("<!-- Sent to Batch 21-30, Codex, 2026-03-04 13:15:00 -->", updated)
            self.assertIn("- [X] Focus on test coverage this batch", updated)

            self.assertEqual(run_mock.call_count, 2)
            commit_call = run_mock.call_args_list[1].args[0]
            self.assertIn("chore(ralph):", commit_call[-1])

    def test_consume_next_session_inbox_message_excludes_frontmatter_examples(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            repo_root = Path(tmpdir)
            inbox_rel = Path("docs/specs/todos/MIT-14/ralph-session-inbox.md")
            inbox_abs = repo_root / inbox_rel
            inbox_abs.parent.mkdir(parents=True, exist_ok=True)
            inbox_abs.write_text(
                "\n".join(
                    [
                        "---",
                        "example_unchecked: |",
                        "  - [ ] Template unchecked example (must not send)",
                        "example_checked: |",
                        "  <!-- Sent to Session 23, Codex, 2026-03-04 13:00:00 -->",
                        "  - [X] Template checked example",
                        "---",
                        "",
                        "- [ ] Real instruction",
                        "  Real context",
                        "---",
                    ]
                )
                + "\n",
                encoding="utf-8",
            )

            with (
                patch.object(loop_runner, "REPO_ROOT", repo_root),
                patch.object(loop_runner, "SESSION_INBOX_FILE", inbox_rel),
                patch.object(loop_runner, "_run"),
            ):
                message = loop_runner._consume_next_session_inbox_message(
                    session_num=24,
                    agent_name="codex",
                    timestamp="2026-03-04 13:20:00",
                )

            self.assertEqual(message, "- [ ] Real instruction\n  Real context")
            updated = inbox_abs.read_text(encoding="utf-8")
            self.assertIn("  - [ ] Template unchecked example (must not send)", updated)
            self.assertIn("  - [X] Template checked example", updated)
            self.assertIn("- [X] Real instruction", updated)

    def test_build_session_prompt_includes_recent_progress_and_both_inboxes(self) -> None:
        with (
            patch.object(loop_runner, "_extract_recent_progress_context", return_value="## Session 99\nDone"),
            patch.object(loop_runner, "_consume_next_session_inbox_message", return_value="- [ ] Fix tests first"),
        ):
            prompt = loop_runner.build_session_prompt(
                base_prompt="Base prompt",
                session_num=40,
                agent_name="codex",
                timestamp="2026-03-04 13:16:00",
                batch_inbox_message="- [ ] Focus on coverage",
            )

        self.assertIn("Base prompt", prompt)
        self.assertIn("## Session 99\nDone", prompt)
        self.assertIn("### Batch Inbox", prompt)
        self.assertIn("- [ ] Focus on coverage", prompt)
        self.assertIn("### Session Inbox", prompt)
        self.assertIn("- [ ] Fix tests first", prompt)

    def test_run_loop_executes_expected_single_session(self) -> None:
        with (
            patch.object(loop_runner, "ensure_on_branch"),
            patch.object(loop_runner, "ensure_progress_file"),
            patch.object(loop_runner, "ensure_session_inbox_file"),
            patch.object(loop_runner, "ensure_batch_inbox_file"),
            patch.object(loop_runner, "_consume_next_batch_inbox_message", return_value=None),
            patch.object(loop_runner, "load_base_prompt", return_value="BASE PROMPT"),
            patch.object(loop_runner, "build_session_prompt", return_value="PROMPT"),
            patch.object(loop_runner, "_commit_leftovers"),
            patch.object(loop_runner, "build_agent_command", return_value=AgentCommand("Codex (codex)", ["echo", "ok"])),
            patch.object(loop_runner, "_run", return_value=subprocess.CompletedProcess(args=["echo"], returncode=0)),
            patch.object(loop_runner.subprocess, "run", return_value=subprocess.CompletedProcess(args=["git"], returncode=0)),
        ):
            rc = loop_runner.run_loop("simple", ["1", "cx:1"])

        self.assertEqual(rc, 0)


if __name__ == "__main__":
    unittest.main()
