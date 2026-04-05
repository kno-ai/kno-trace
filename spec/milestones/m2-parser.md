# M2: Parser, Builder & Live Tail

**Prerequisites:** Read [spec/README.md](../README.md) (core spec) and `SCHEMA.md`. All field names in this milestone must come from `SCHEMA.md`, not from the core spec — if there is any discrepancy, `SCHEMA.md` wins.

**Goal:** The data pipeline that powers the control room — JSONL becomes structured data, and the live streaming infrastructure is in place.

**Deliverable:** `kno-trace --dump <session>` prints a structured text summary to stdout, proving the parser is correct. Live tail infrastructure is ready for M3 to wire into the UI. `--dump` is a permanent feature — keep it working in all future milestones.

**Critical constraint:** The parser must NEVER crash on unexpected data. Claude Code's JSONL format changes without notice. See CLAUDE.md "Resilience" section. Every field access needs a safe default. Unknown line types, unknown tools, missing fields, null values, type mismatches — all must be handled gracefully. Extract what you recognize, skip what you don't.

**Use cases served:** UC1 (what did Claude just do?), UC7 (check past session via `--dump`)

---

## Scope

- `internal/parser/jsonl.go` — emits `[]RawEvent` from JSONL lines:
  - Handles streaming snapshots: dedupe by `requestId`, only use lines with `message.stop_reason != null` as final response
  - **CRITICAL:** Collect `tool_use` blocks from ALL streaming snapshots for a given `requestId`, not just the final one. Parallel agents may each appear in a different snapshot — the final snapshot only contains the last agent's tool_use. Merge tool_use blocks across snapshots before building the prompt's ToolCalls list.
  - Skip `isMeta: true` lines (not human turns)
  - Skip non-message line types gracefully: `progress`, `system`, `queue-operation`, `file-history-snapshot`, `last-prompt`
  - Handle `progress` lines with `data.type === "agent_progress"` — extract embedded subagent messages for real-time agent visibility
  - Skips malformed JSON lines with stderr log; does not crash
  - Emits unknown tool names as `ToolOther`
  - All field names per `SCHEMA.md` — do not guess field names
- `internal/parser/builder.go` — assembles RawEvent stream into `*model.Session`:
  - Prompt boundary: human turn (content[0].type === "text") starts a new prompt; tool result (content[0].type === "tool_result") does NOT
  - Groups tool_use + tool_result pairs by ID
  - Extracts Read tool content from tool_result (not tool_use input)
  - Detects `/compact`: detect `type: "system"` with `subtype: "compact_boundary"`, extract `compactMetadata.trigger` and `compactMetadata.preTokens`; records in `Session.CompactAt`
  - Detects interrupted session: last human turn has no following assistant turn
  - `ModelName` on each Prompt: from `model` field on last assistant message in that prompt
  - `CacheRead` / `CacheCreate`: from usage fields on assistant message if present; 0 if unavailable (confirm field names in M0)
  - `ContextPct`: `input_tokens` of last assistant message / model's context window size (from config) * 100; 0 if unavailable
  - `Session.IsLive = false` after parse (watcher sets it to true when actively tailing)
- `internal/parser/classify.go`:
  - Sets `IsCLAUDEMD` for paths matching the exact list in Key Design Decision #9
  - Identifies MCP tool calls via `mcp__` prefix match (not "any name not in known list")
  - Generates `WarnMCPExternal` for MCP calls
  - Generates `WarnContextHigh` and `WarnContextCritical` per prompt using configured thresholds
  - Generates `WarnInterrupted` on interrupted prompts
  - **File-modifying tools** (tracked for replay): `Write`, `Edit`, `Read` (baseline)
  - **Shell execution tools** (noted as present, effects not tracked): `Bash`, `PowerShell`
  - **Notebook tools** (noted as present, effects not tracked): `NotebookEdit`
  - **Agent tool** (spawns subagent): `Agent`
  - **All other known built-in tools** (displayed as info-only): `Glob`, `Grep`, `ToolSearch`, `TodoWrite`, `WebSearch`, `WebFetch`, `TaskCreate`, `TaskUpdate`, `TaskGet`, `TaskList`, `TaskStop`, `TaskOutput`, `Skill`, `ExitPlanMode`, `EnterPlanMode`, `EnterWorktree`, `ExitWorktree`, `AskUserQuestion`, `CronCreate`, `CronDelete`, `CronList`, `LSP`, `ListMcpResourcesTool`, `ReadMcpResourceTool`, `SendMessage`, `TeamCreate`, `TeamDelete`, `NotebookEdit`
  - **MCP tools**: detected by `mcp__` prefix
  - **Unknown tools**: any name not in the above lists or MCP prefix → `ToolOther`, displayed as info-only
  - **Note:** `MultiEdit` does not exist. Claude uses parallel `Edit` calls instead.
  - **No Bash risk classification**
  - **Loop/spin detection:** Generates `WarnLoopDetected` when the same tool+file path pair appears N times (configurable via `loop_detection_threshold`, default 3) within a sliding window of recent tool calls across the active prompt. This is structural repetition detection — no content comparison, no heuristics. Only checks tool name + file path identity.
  - **Git branch transitions:** Detects when `gitBranch` changes between consecutive messages. Records branch transitions on the prompt where the change occurred (`Prompt.BranchTransition` field — previous and new branch names).
  - **Prompt duration outliers:** After all prompts are built, computes mean and standard deviation of prompt durations. Flags prompts with duration >2σ above the mean as `Prompt.IsDurationOutlier = true`. Only computed when ≥5 prompts exist (avoid flagging short sessions).
- `internal/watcher/tail.go`:
  - On startup: replay all existing lines, emit `MsgPromptSealed` per sealed prompt incrementally (not batched), emit `MsgReplayDone` when caught up
  - During initial replay: discard any trailing bytes without a terminating `\n` (truncated file) — do not hang waiting for more data
  - Live tail: watch for new bytes via fsnotify, buffer until `\n`, emit `MsgPromptUpdate` / `MsgPromptSealed`
  - Handle `fsnotify.Remove` event: emit `MsgSessionFileDeleted`
  - Never access `Session` or any model types directly — communicate only via tea.Msg
  - Set `Session.IsLive = true` when tailing begins; set to `false` if file is removed
- `--dump` flag: parse session file, print structured summary to stdout, exit 0:
  ```
  Session: kno-trace · 2024-04-04 14:32 · 1h 14m · 2.4 MB · claude-sonnet-4-5

  #1 [14:32–14:35] scaffold the Go project...
     ctx: 12%  tokens: 4.2k in / 380 out
     Write  cmd/kno-trace/main.go (+42)
     Write  go.mod (+12)
     Bash   go build ./...

  ── branch: main → feature/parser ──

  #2 [14:35–14:40] build the JSONL parser...  ⏱ slow
     ctx: 24%  tokens: 12.1k in / 2.4k out
     ⬡ subagent-1 (Explore, haiku) — "find parser patterns" — 3 tools, 2 files, 1.2k tokens — 8s
     ⬡ subagent-2 (Explore, haiku) — "check test fixtures" — 5 tools, 3 files, 1.8k tokens — 12s  [parallel]
     Write  internal/parser/jsonl.go (+156)
     Edit   internal/model/types.go (+12 -3)
     ⟳ LOOP: Edit internal/parser/jsonl.go repeated 4×
     ...
  ```
  - Branch transition dividers shown between prompts when `gitBranch` changes
  - `⏱ slow` badge on duration outlier prompts
  - `⟳ LOOP` shown on prompts with `WarnLoopDetected`

---

## Acceptance Criteria

- Model fields added to `internal/model/types.go`: `Prompt.BranchTransition` (struct with `From` and `To` strings, zero value when no transition), `Prompt.IsDurationOutlier` (bool), `WarnLoopDetected` warning type
- `--dump internal/testdata/simple.jsonl` produces correct structured output
- `--dump internal/testdata/with_agent.jsonl` correctly shows agent tool calls
- `--dump internal/testdata/interrupted.jsonl` shows interrupted flag on last prompt
- Malformed JSON lines skipped with stderr log, no crash
- `ContextPct` correct on a session with known token counts; 0 on a session without
- Loop detection fires when same tool+path repeats ≥threshold times; does not fire below threshold
- Git branch transition detected and recorded on correct prompt
- Duration outlier flagging correct for session with ≥5 prompts; not computed for shorter sessions
- All parser unit tests pass (see [testing.md](../testing.md))
- Live tail test passes: write lines to temp JSONL, assert events emitted in correct order and incrementally
