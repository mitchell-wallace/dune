"""Agent command builders for Ralph loop execution."""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class AgentCommand:
    display_name: str
    argv: list[str]
    suppress_stderr: bool = False


def build_agent_command(agent_name: str, mode: str, prompt: str) -> AgentCommand:
    if agent_name == "claude":
        if mode == "simple":
            return AgentCommand(
                display_name="Claude Code (claude)",
                argv=[
                    "claude",
                    "-p",
                    "--dangerously-skip-permissions",
                    "--output-format",
                    "text",
                    prompt,
                ],
            )
        return AgentCommand(
            display_name="Claude Code (claude)",
            argv=[
                "claude",
                "-p",
                "--dangerously-skip-permissions",
                "--output-format",
                "stream-json",
                "--include-partial-messages",
                prompt,
            ],
        )

    if agent_name == "codex":
        if mode == "simple":
            return AgentCommand(
                display_name="Codex (codex)",
                argv=[
                    "codex",
                    "exec",
                    "--dangerously-bypass-approvals-and-sandbox",
                    prompt,
                ],
                suppress_stderr=True,
            )
        return AgentCommand(
            display_name="Codex (codex)",
            argv=[
                "codex",
                "exec",
                "--dangerously-bypass-approvals-and-sandbox",
                "--json",
                prompt,
            ],
        )

    if agent_name == "gemini":
        if mode == "simple":
            return AgentCommand(
                display_name="Gemini CLI (gemini)",
                argv=[
                    "gemini",
                    "--prompt",
                    prompt,
                    "--yolo",
                    "--output-format",
                    "text",
                ],
            )
        return AgentCommand(
            display_name="Gemini CLI (gemini)",
            argv=[
                "gemini",
                "--prompt",
                prompt,
                "--yolo",
                "--output-format",
                "stream-json",
            ],
        )

    if agent_name == "opencode":
        if mode == "simple":
            return AgentCommand(
                display_name="OpenCode (opencode)",
                argv=["opencode", "run", prompt],
            )
        return AgentCommand(
            display_name="OpenCode (opencode)",
            argv=["opencode", "run", "--format", "json", prompt],
        )

    raise ValueError(f"Unsupported scheduled agent '{agent_name}'.")
