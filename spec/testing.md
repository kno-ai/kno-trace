# Testing Strategy

**Prerequisites:** Read [spec/README.md](README.md) (core spec).

Write tests alongside or before implementation — not after.

---

## Unit tests (required)

- `internal/config/`: config loading with defaults, model name substring matching, missing config file uses all defaults
- `internal/parser/`: fixture-based. Cover: prompt boundary detection (human turn vs tool result), all tool types, Read content from tool_result, subagent linkage, interrupted session, malformed JSON lines, truncated file (no trailing newline), context% when tokens present, context% 0 when tokens absent, context% with different model window sizes, compact detection
- `internal/replay/`: cover Snapshot Reconstruction Algorithm exhaustively — Write-only file, Read baseline then Edit chain, Write after Edit resets chain, MultiEdit, unresolvable Edit (WarnReplayGap, not crash), `GetContentAt` at every prompt index for `simple.jsonl`
- `internal/agent/`: cover no agents, single agent, two parallel agents (exact timestamp check), nested agent, unresolvable linkage (goes to UnlinkedAgents, not guessed)

## Integration tests

- Parse each fixture end-to-end; assert exact prompt count, file count, agent count
- Live tail: write lines to temp JSONL incrementally; assert events emitted in correct order
- `--dump` stdout: assert output matches expected for `simple.jsonl`

## TUI smoke tests

- Each view renders without panic for: empty session, single prompt, session with parallel agents
- Key events produce correct state transitions

## Fixture files

All fixtures in `internal/testdata/` are real anonymized sessions created in M0. Do not invent synthetic JSONL — real JSONL structure from actual Claude Code sessions is the only ground truth.
