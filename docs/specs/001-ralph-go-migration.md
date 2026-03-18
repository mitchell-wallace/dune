# Spec 001: Ralph Go Migration MVP

## Summary

- Replace `scripts/ralph` as the active orchestrator with a dedicated Go binary `sand-orch` plus an in-container Bubble Tea TUI.
- This sprint ships a full replacement for the current Python loop for single-container, single-active-batch operation in simple/non-interactive mode.
- `tmux`, stream mode, detached/background execution, host-level dashboarding, and multi-agent delegation are explicitly deferred.

## Current Sprint Scope

- Add `cmd/sand-orch` and shared `internal/orchestrator/...` packages for state, messages, progress, runner, and TUI.
- Extend host-side `sand` provisioning so each container gets a read-only bind mount for a Linux build of `sand-orch` at `/usr/local/bin/sand-orch`.
- Initial rollout requires container recreation once to pick up the new mount; after that, orchestrator updates only require rebuilding the mounted binary, not rebuilding the image.
- Keep the Python prototype in `scripts/ralph/` as legacy reference only; it is no longer the primary path once Go is shipped.
- Preserve existing agent mix behavior and non-interactive agent commands for Claude, Codex, Gemini, and OpenCode.
- Replace manual session numbering/ranges with orchestrator-assigned monotonic `batch_id` and `session_id`; TUI displays both global `session_id` and per-batch `iteration_index`.

## Data Model And Storage

- Data dir lives under `/persist/agent/ralph/<container-name>/`; host `sand` injects `SAND_CONTAINER_NAME`, `SAND_ORCH_DATA_DIR`, and `SAND_ORCH_REPO_PROGRESS_PATH`.
- Operational truth lives in the data dir, not in the repo. Repo progress is the canonical human-facing artifact and is rebuilt from data-dir session records if it becomes invalid.
- Use append-only `messages.jsonl` in the data dir. Each line is an event record with `event_id`, `message_id`, `scope`, `event_type`, `created_at` or `updated_at` or `consumed_at`, `body`, and batch or session linkage.
- Support `message_created`, `message_updated`, `message_consumed`, and `message_cancelled` events. Messages are editable until consumed; consumed messages are immutable in this sprint.
- Use `state.yaml` in the data dir for runner state: schema version, active batch, target iterations, completed iterations, stop flag, next ids, and validation or repair metadata.
- Store per-session artifacts in `sessions/session-<id>/meta.yaml` and `sessions/session-<id>/terminal.log`.
- Store repo progress at `docs/orchestration/ralph-progress.yaml`.
- Keep only the 50 most recent sessions in the repo progress file. Older sessions remain in data-dir session records so the repo file can be regenerated deterministically.

## Progress Log Format

- `docs/orchestration/ralph-progress.yaml` contains only human-facing progress and batch summary, not runner counters.
- Top-level fields: `version`, `updated_at`, `active_batch`, `history_window`, `recent_sessions`.
- Each `recent_sessions` entry contains `session_id`, `batch_id`, `iteration_index`, `agent`, `status`, `started_at`, `ended_at`, `runtime_seconds`, `summary`, `files_touched`, `commits`, `follow_ups`, and `message_ids`.
- Agents record progress with `sand-orch progress record`, which reads YAML from stdin and merges it into the current session entry. Direct edits to the repo YAML are tolerated if they keep the file valid; otherwise the orchestrator repairs from data-dir records and warns in the TUI.
- Do not depend on agent stop hooks in this sprint. Prompt contracts and the helper command are the required path because hook support is inconsistent across the supported CLIs.

## Message Semantics

- Session-scope messages are FIFO and consumed one-at-a-time by the next unstarted session.
- Batch-scope messages target either the next batch or the currently running batch. When attached to a batch they are considered consumed once and then applied to all remaining unstarted sessions in that batch.
- TUI shows `created_at`, `updated_at`, `consumed_at`, scope, target, and final status for every message.
- TUI supports create, edit, cancel, and inspect. Editing after consume is deferred.

## Runner And TUI Behavior

- The TUI owns the active batch process. Closing the TUI stops future work after confirmation; no detached supervisor is added in this sprint.
- Runner executes sessions sequentially in simple/non-interactive mode, tees stdout and stderr to the session transcript, and computes runtime from `started_at` and `ended_at`.
- Default batch controls: start batch, change target iterations, request stop-after-current-session, browse completed sessions, inspect transcripts, and view repo progress.
- Changing batch size upward adds remaining iterations. Changing it downward cannot go below completed iterations. End batch early means stop after the current session finishes; hard-killing the active session is deferred.
- The dashboard shows active batch status, current session, elapsed runtime, agent mix, pending messages, recent progress warnings, and completed session summaries.
- Preserve the existing weighted agent mix grammar; selection uses orchestrator `session_id` so scheduling remains deterministic across batches.

## CLI And Interface Changes

- New commands: `sand-orch tui`, `sand-orch progress record`, `sand-orch progress repair`, and `sand-orch import-legacy`.
- `sand-orch progress record` uses the current session from env when available and accepts YAML on stdin.
- Host `sand` gains the responsibility to build the Linux-targeted `sand-orch` artifact and inject the container mount and env wiring.
- Legacy markdown inbox or progress files are not used at runtime once migrated. `sand-orch import-legacy` can optionally import existing `ralph-*.md` files if present, otherwise it initializes clean state.

## Testing And Acceptance

- Unit tests for message-event folding, state transitions, batch resizing, stop-after-current behavior, retention compaction, repo-progress regeneration, invalid-YAML repair, and deterministic agent selection.
- Integration tests with fake agent binaries for a full batch run, session transcript capture, progress helper writes, missing-progress warning, session-message FIFO delivery, and mid-batch batch-message attachment.
- Container integration test in an ephemeral container to verify the mounted Linux binary, env wiring, persisted data path, and transcript or progress visibility inside the container.
- Acceptance scenario: start a 5-iteration batch, add and edit a session message before consume, add a batch message mid-run, increase then decrease iteration count, stop after current session, reopen the TUI, and browse completed transcripts plus the compacted 50-session YAML history.

## Assumptions And Defaults

- Single active batch per container and workspace and profile.
- One agent process runs at a time.
- Simple output-text mode only in this sprint; stream mode and `tmux` are deferred.
- Repo progress path is `docs/orchestration/ralph-progress.yaml`.
- Repo progress keeps 50 recent sessions by default.
- Existing containers need one recreate to receive the new binary mount, but future orchestrator updates do not require image rebuilds.

## Deferred Future Goals

- Host `sand` TUI can inspect all sand-managed containers and surface per-container orchestrator status.
- Detached or background runner support if reconnectable long-running batches becomes necessary.
- Cross-agent hook integration once support is verified across Claude, Codex, Gemini, and OpenCode.
- Multi-agent orchestration where a coordinator agent prepares handoffs and assigns work by role.
- Richer retention and archiving for transcripts or session artifacts beyond the repo-progress 50-session window.
