# kno-trace — Confirmed JSONL Schema

> Produced during M0 schema investigation. This document is the authoritative
> reference for all field names. Where it conflicts with the spec, SCHEMA.md wins.

## Confirmed Field Names

### Line types

Claude Code JSONL files contain 7 distinct top-level `type` values:

| type | description |
|------|-------------|
| `assistant` | LLM response (streaming snapshots + final) |
| `user` | Human turns and tool results |
| `progress` | Agent/hook progress updates (embeds subagent messages) |
| `system` | Turn duration, compact boundary, local commands |
| `queue-operation` | Session queue management (enqueue/dequeue/remove) |
| `file-history-snapshot` | File backup tracking |
| `last-prompt` | Records last user prompt text |

Only `assistant` and `user` types carry `message` objects with content blocks. The parser must handle all types gracefully — skip unknown types, don't crash.

### Common fields on user and assistant lines

All user and assistant lines carry these fields:

```
parentUuid      — UUID of the parent message (conversation threading)
isSidechain     — boolean, true for sidechain/subagent conversations
promptId        — groups messages within a single prompt cycle
type            — "user" or "assistant"
message         — the message object (role, content, etc.)
uuid            — unique ID for this line
timestamp       — ISO 8601 timestamp
userType        — e.g., "external", "human"
cwd             — working directory at time of message
sessionId       — session UUID (matches the JSONL filename without .jsonl)
version         — Claude Code version string
gitBranch       — git branch name at time of message
```

### Human turn identification

- Field path: `message.content[0].type`
- Value that identifies a human turn (not tool result): `"text"`
- Value that identifies a tool result: `"tool_result"`

**Additional differentiators:**
- Tool result lines have a top-level `toolUseResult` object (not inside `message`)
- Tool result lines have `sourceToolAssistantUUID` pointing to the assistant message that requested the tool
- Meta/local-command messages have `isMeta: true` and `message.content` is a **string** (not a list) — these are NOT human turns, skip them

**Important:** Check `isMeta` first. If `true`, skip the line regardless of content type. Then check `message.content[0].type` — if it's a list, the first element's type distinguishes human text from tool result.

### Session / conversation ID

- Field name: `sessionId`
- Example value: `"7a971014-a18b-4791-90a9-3e545badd4d5"` (UUID string, matches JSONL filename)
- The `uuid` field is per-line unique, not per-session

### Subagent architecture

**CRITICAL DIFFERENCE FROM SPEC:** Subagent messages are in **separate JSONL files**, not in the parent session file.

**File layout:**
```
~/.claude/projects/<project-dir>/
  <sessionId>.jsonl              — parent session
  <sessionId>/                   — session directory (created when agents spawn)
    subagents/
      agent-a<agentId>.jsonl     — regular Agent tool subagent
      agent-a<agentId>.meta.json — metadata: {"agentType": "Explore", ...}
      agent-acompact-<hex>.jsonl — compact operation (no .meta.json)
      agent-aside_question-<hex>.jsonl — /btw side question (no .meta.json)
    tool-results/
      <hash>.txt                 — large tool output stored separately
```

**`/btw` side questions:** Confirmed: `/btw` adds ZERO lines to the parent session JSONL. It creates a `side_question` subagent file that replays the full parent conversation history (all lines marked `isSidechain: true`), appends the question as a user message with string content wrapped in `<system-reminder>` tags, and includes the response. Completely invisible in the parent JSONL.

**Meta file structure:** `{"agentType": "<type>"}` with optional `"description"`. Observed agentTypes: `Explore` (most common), `general-purpose`, `Plan`, `claude-code-guide`.

**However:** The parent session file contains `progress` lines that **embed full subagent messages inline**:

```json
{
  "type": "progress",
  "data": {
    "type": "agent_progress",
    "message": { /* full subagent user/assistant message */ },
    "prompt": "...",
    "agentId": "ae0392d9e28ec418d"
  },
  "toolUseID": "agent_msg_01GHXXxD95Wya42uFwkat7Tk",
  "parentToolUseID": "toolu_01NpEB65bU9evLHydHpLFyPs"
}
```

**Recommended approach:** Use `progress` lines with `data.type === "agent_progress"` as the primary source for subagent activity. This avoids reading separate files and gives real-time visibility within the parent session's JSONL stream.

### Subagent parent linkage

- **Parent side:** `tool_use` block with `"name": "Agent"` and `"id": "toolu_01..."`. The `input` contains `subagent_type`, `description`, `prompt`.
- **Result side:** The tool_result `user` line has a top-level `toolUseResult` object:
  ```json
  {
    "status": "completed",
    "agentId": "ae0392d9e28ec418d",
    "prompt": "...",
    "totalDurationMs": 44748,
    "totalTokens": 53512,
    "totalToolUseCount": 33,
    "usage": { ... }
  }
  ```
- **Progress lines** link via `parentToolUseID` (matches the Agent `tool_use.id`) and `data.agentId`
- **Subagent JSONL files** are named `agent-a<agentId>.jsonl` where `agentId` matches `toolUseResult.agentId`

**Linkage chain:** Agent `tool_use.id` → progress lines' `parentToolUseID` → `data.agentId` → tool_result's `toolUseResult.agentId`

### Token usage

- Input tokens field: `message.usage.input_tokens`
- Output tokens field: `message.usage.output_tokens`
- Cache read tokens field: `message.usage.cache_read_input_tokens`
- Cache creation tokens field: `message.usage.cache_creation_input_tokens`
- Which message type carries this field: assistant messages only
- Additional fields in `usage`:
  - `service_tier` — e.g., `"standard"`
  - `server_tool_use` — `{web_search_requests, web_fetch_requests}`
  - `cache_creation.ephemeral_1h_input_tokens`, `cache_creation.ephemeral_5m_input_tokens`
  - `speed` — e.g., `"standard"`

### Model name

- Field name: `message.model`
- Example values: `"claude-opus-4-6"`, `"claude-haiku-4-5-20251001"`
- Present on assistant messages only, inside the `message` object

### Streaming snapshots

**CRITICAL:** Assistant responses are written as **multiple JSONL lines** sharing the same `requestId` and `message.id`. Intermediate lines have `stop_reason: null`. The final line has `stop_reason: "end_turn"` or `"tool_use"`.

Each line is a **complete replacement** of the message content (not a delta). For parsing:
- Track lines by `requestId`
- Only use the line with `message.stop_reason != null` as the final response
- For live display, show the latest line for each `requestId` and replace when a new one arrives

**CRITICAL — Parallel agents and streaming snapshots:** When Claude spawns multiple agents in one response, each Agent tool_use may appear in a **different streaming snapshot**. Example from a real session:
- Snapshot 1 (stop_reason: null): contains Agent tool_use `toolu-001` (first agent)
- Snapshot 2 (stop_reason: tool_use): contains Agent tool_use `toolu-002` (second agent) — but NOT `toolu-001`

Both agents run (both have tool_results). The parser MUST collect tool_use blocks from ALL streaming snapshots for a given requestId, not just the final one. Otherwise parallel agents will appear as a single agent.

**Progress lines are NOT reliably present.** Confirmed across multiple real sessions (including general-purpose agents that wrote files): agent activity in the parent JSONL is limited to:
1. The Agent tool_use in the assistant message (spawned)
2. The tool_result with toolUseResult metadata (completed — includes totalDurationMs, totalTokens, totalToolUseCount, status)
Progress lines (`agent_progress`) were observed in some sessions but absent in others — even for agents that performed Write/Edit operations. The agent's individual tool calls are only guaranteed to exist in the separate subagent JSONL file. The parser must handle both cases gracefully.

### Stop reason

- Field path: `message.stop_reason`
- Values: `"end_turn"` (conversation done), `"tool_use"` (tool call requested), `null` (streaming snapshot)

### Thinking blocks

Assistant messages may contain thinking/reasoning blocks:
```json
{"type": "thinking", "thinking": "...", "signature": "..."}
```
These appear in `message.content` alongside text and tool_use blocks.

### Non-message metadata lines

**system lines** — three subtypes observed:
- `turn_duration` (most common): `{type: "system", subtype: "turn_duration", durationMs, messageCount, entrypoint: "cli"}`
- `compact_boundary`: marks conversation compaction (see below)
- `local_command`: local command output

**queue-operation:** `{type: "queue-operation", operation: "enqueue"|"dequeue"|"remove", timestamp, sessionId}`

**file-history-snapshot:** `{type: "file-history-snapshot", messageId, snapshot: {trackedFileBackups: [...]}, isSnapshotUpdate}`

**last-prompt:** `{type: "last-prompt", lastPrompt: "...", sessionId: "..."}`

**progress:** Two subtypes:
- `data.type: "agent_progress"` — embeds subagent messages (see Subagent architecture)
- `data.type: "hook_progress"` — pre/post tool hook execution

### /compact representation

Compact appears as a `system` line:
```json
{
  "type": "system",
  "subtype": "compact_boundary",
  "content": "Conversation compacted",
  "agentId": "acompact-a6897da3a48553a5",
  "isSidechain": true,
  "isMeta": false,
  "level": "info",
  "logicalParentUuid": "4368310c-...",
  "compactMetadata": {
    "trigger": "auto",
    "preTokens": 168954
  }
}
```

- `trigger` can be `"auto"` (context pressure) or `"manual"` (user typed `/compact`)
- `preTokens` is the token count before compaction — useful for showing context% before/after
- Compact spawns its own subagent file: `agent-acompact-<hex>.jsonl`

### Additional useful fields

- `slug` — human-readable session identifier (e.g., `"swift-seeking-cat"`), appears partway through session, not on first message. Could be used for display.
- `permissionMode` — on human turn messages: `"default"`, `"acceptEdits"`, etc.
- `caller` — on tool_use blocks: `{"type": "direct"}` (only value observed)
- `isSidechain` — boolean, present on all message lines. True for subagent/compact conversations.
- `parentUuid` — threading: which message this is a response to

## All Observed Tool Names

Built-in tools:
**Observed in local sessions:**
`Read`, `Write`, `Edit`, `Bash`, `Glob`, `Grep`, `Agent`, `ToolSearch`, `TodoWrite`, `WebSearch`, `WebFetch`, `TaskUpdate`, `TaskCreate`, `Skill`, `ExitPlanMode`

**Documented in Claude Code tools reference but not observed locally:**
`AskUserQuestion`, `CronCreate`, `CronDelete`, `CronList`, `EnterPlanMode`, `EnterWorktree`, `ExitWorktree`, `ListMcpResourcesTool`, `LSP`, `NotebookEdit`, `PowerShell`, `ReadMcpResourceTool`, `SendMessage`, `TaskGet`, `TaskList`, `TaskOutput`, `TaskStop`, `TeamCreate`, `TeamDelete`

**Does not exist:** `MultiEdit` — Claude uses parallel `Edit` calls instead.

**Handling strategy:** Only `Write`, `Edit`, and `Read` (for baseline) need special parsing for file reconstruction. `Bash` and `PowerShell` are shell execution — note their presence but can't track file effects. `NotebookEdit` modifies `.ipynb` files — same treatment as Bash (note presence, can't reconstruct). All other tools are displayed as info-only (`ToolOther` or by their specific name for display).

MCP tools (follow `mcp__<server>__<tool>` naming):
`mcp__kno__kno_vault_status`, `mcp__kno__kno_note_create`, `mcp__kno__kno_note_list`, `mcp__kno__kno_note_show`, `mcp__kno__kno_note_update`, `mcp__kno__kno_page_create`, `mcp__kno__kno_page_show`, `mcp__kno__kno_page_update`, `mcp__kno__kno_set_option`

## Hash Algorithm

- **Not a hash.** Directory name is the absolute project path with `/` replaced by `-`.
- Example: project path `/Users/careykevin/code/kno-ai/kno-trace` → directory `-Users-careykevin-code-kno-ai-kno-trace`
- Reverse mapping: replace leading `-` with `/`, then replace remaining `-` with... ambiguous (directory names can contain `-`). **Recommended:** Use the directory name as-is for display; do not attempt reverse mapping. Instead, match CWD by encoding it the same way and comparing.
- No separate metadata file in project directories that records the original path.

## Fixture Files

Created in `internal/testdata/`:

| Fixture | Lines | Contents |
|---|---|---|
| `simple.jsonl` | 18 | 3 prompts, no agents. Read, Bash, Write, Edit, system turn_duration lines |
| `interrupted.jsonl` | 4 | 1 complete prompt + 1 human turn with no assistant response |
| `with_agent.jsonl` | 14 | 2 prompts, one with Agent tool call + 3 agent_progress lines |
| `mcp_calls.jsonl` | 7 | 1 prompt with 2 MCP tool calls (kno vault_status, note_create) |
| `with_compact.jsonl` | 7 | 1 prompt before compact, compact_boundary system line, 1 prompt after |

| `parallel_agents.jsonl` | 7 | 1 prompt with 2 parallel Agent calls. Streaming snapshot has Agent 1, final has Agent 2. Both tool_results with toolUseResult metadata. |
| `replay_chain.jsonl` | 18 | 2 prompts, single file (`config.go`). Read baseline → Edit × 2 → Write (resets chain) → Edit → failed Edit (`is_error: true`, old_str not found → WarnReplayGap). Tests full snapshot reconstruction algorithm. |
| `edge_cases.jsonl` | 15 | 2 prompts + malformed JSON line + blank line + `isMeta` message. Text-only response (no tools), Bash `is_error: true` with exit code, fix→retry cycle. Tests parser resilience. |
| `advanced_session.jsonl` | 33 | 5 prompts covering advanced patterns: git branch transition (main→feature/auth-refactor), model switching (opus→sonnet), CWD change, permission mode change (default→acceptEdits), agent that writes/edits files (haiku model), sidechain `/btw` exchange, hook progress line. Tests duration outlier (≥5 prompts), branch detection, agent file conflict detection. |

**Not created:**
- None — all fixture types sourced from real sessions or synthetic data with real schema structure.

## Schema Discrepancies vs spec

| Area | Spec assumed | Actual |
|------|-------------|--------|
| Directory naming | Hash of project path (SHA256 or similar) | Slash-to-dash encoding of absolute path |
| Subagent tool name | `Task` | `Agent` |
| MultiEdit tool | Exists as a tool | Does not exist. Claude uses parallel Edit calls instead. Removed from spec. |
| Subagent messages | Same JSONL file, different sessionId | Separate files in `subagents/` directory; BUT `progress` lines embed them in parent |
| Known tool list | Write, Read, Edit, MultiEdit, Bash, Task, TodoWrite, WebSearch | Add: Glob, Grep, Agent, ToolSearch, TaskCreate, TaskUpdate, WebFetch, Skill, ExitPlanMode |
| MCP tool detection | Any unknown tool name | MCP tools follow `mcp__<server>__<tool>` naming convention — use prefix match |
| Compact representation | Special message type or synthetic turn | `system` line with `subtype: "compact_boundary"` |
| Assistant messages | One line per response | Multiple streaming snapshot lines per response; only final has `stop_reason != null` |
| Message content format | Always a list of blocks | Usually a list, but meta/local-command messages have content as a raw string |
| Additional line types | Not anticipated | `progress`, `system`, `queue-operation`, `file-history-snapshot`, `last-prompt` |
| Agent result metadata | Inferred from tool_result text | Rich `toolUseResult` object with `status`, `agentId`, `totalDurationMs`, `totalTokens`, `totalToolUseCount` |
| Thinking blocks | Not mentioned | Present in assistant content as `{"type": "thinking", ...}` |
| Slug field | Not mentioned | Human-readable session name, appears mid-session |
