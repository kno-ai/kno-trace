# Testing Strategy

**Prerequisites:** Read [spec/README.md](README.md) (core spec).

Write tests alongside or before implementation — not after.

---

## Unit tests (required)

- `internal/config/`: config loading with defaults, model name substring matching, missing config file uses all defaults
- `internal/parser/`: fixture-based. Cover: prompt boundary detection (human turn vs tool result), all tool types, Read content from tool_result, subagent linkage, interrupted session, malformed JSON lines, truncated file (no trailing newline), context% when tokens present, context% 0 when tokens absent, context% with different model window sizes, compact detection, loop detection (fires at threshold, does not fire below, clears when pattern breaks), git branch transition detection, duration outlier flagging (≥5 prompts only)
- `internal/replay/`: cover Snapshot Reconstruction Algorithm exhaustively — Write-only file, Read baseline then Edit chain, Write after Edit resets chain, multiple parallel Edits on same file, unresolvable Edit (WarnReplayGap, not crash), `GetContentAt` at every prompt index for `simple.jsonl`, agent Write followed by parent Edit (agent Write used as base, no WarnReplayGap), agent tool calls interleaved by timestamp in file history, HeatScore includes agent file modifications
- `internal/agent/`: cover no agents, single agent, two parallel agents (exact timestamp check), nested agent, unresolvable linkage (goes to UnlinkedAgents, not guessed), subagent JSONL file reading (tool calls populated from file), missing subagent file (graceful fallback to toolUseResult summary), live subagent file tailing (write lines to temp subagent JSONL, assert MsgAgentToolCall events emitted), tailing stops on agent completion

## Resilience tests (required)

The parser must never crash. These tests verify graceful handling of unexpected data:
- Parse `edge_cases.jsonl` without panic (malformed JSON, blank lines, isMeta, tool errors)
- Feed a line with `type` field missing → skipped, no crash
- Feed a line with `message` field missing → skipped, no crash
- Feed a line with `message.content: null` → treated as empty, no crash
- Feed a line with `message.content: []` (empty array) → no index-out-of-range
- Feed a tool_result with `toolUseResult: null` → no crash
- Feed a tool_result with `toolUseResult` as an unexpected type (e.g., integer) → no crash
- Feed an assistant message with unknown content block type `{"type": "new_block_type"}` → skipped, other blocks processed
- Feed a line with unknown `type` value (e.g., `"type": "future_type"`) → skipped silently
- Feed a system line with unknown `subtype` → skipped silently
- Feed a progress line with unknown `data.type` → skipped silently
- Feed an Edit tool_use with missing `old_string` field → treated as no-op or ToolOther, no crash
- Feed an Agent tool_use with missing `subagent_type` → defaults to empty string, no crash
- Feed a line with `timestamp` in an unexpected format → zero time, no crash
- Feed a JSONL where all assistant lines have `stop_reason: null` (no final) → use last snapshot, no hang
- Feed a tool_result referencing a `tool_use_id` that doesn't exist → skip linkage, no crash

## Integration tests

- Parse each fixture end-to-end; assert exact prompt count, file count, agent count
- Live tail: write lines to temp JSONL incrementally; assert events emitted in correct order
- `--dump` stdout: assert output matches expected for `simple.jsonl`

## TUI smoke tests

- Each view renders without panic for: empty session, single prompt, session with parallel agents
- Key events produce correct state transitions

## Fixture files

All fixtures in `internal/testdata/` are real anonymized sessions created in M0. Do not invent synthetic JSONL — real JSONL structure from actual Claude Code sessions is the only ground truth.
