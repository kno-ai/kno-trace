# M5: Agent Tree & Swimlane

**Prerequisites:** Read [spec/README.md](../README.md) (core spec) and `SCHEMA.md`. M4 must be complete (live timeline and ticker working). Pay special attention to the AgentNode data model — agents are first-class citizens in kno-trace.

**Goal:** Subagents fully visible — expanded inline in timeline and rendered as parallel lanes in swimlane. Both live (while agents run) and post-hoc (reviewing completed sessions).

**Deliverable:** Multi-agent workflow users see exactly what each agent was asked, what it touched, and whether agents ran in parallel — in real time as it happens, not just after the fact.

**Use cases served:** UC4 (what are my agents doing?) — this is THE use case for kno-trace's target audience. The swimlane is the flagship view. A developer should be able to glance at kno-trace and immediately understand the state of every active agent.

---

## Build Order

Build in layers so you can verify at each stage:

1. **Agent tree builder + subagent file reading (static).** Build `internal/agent/tree.go` — read subagent JSONL files, build tree, detect parallel topology, detect file conflicts. Test against completed sessions only. **Checkpoint:** add agent tree data to `--dump` output and verify against `with_agent.jsonl` and `parallel_agents.jsonl` fixtures. Expand an agent node in the timeline and see its full tool call list from the subagent file.

2. **Live subagent file tailing.** Add `internal/agent/watcher.go` — when an Agent `tool_use` appears during a live session, start tailing the subagent JSONL file. Emit agent tool call events. **Checkpoint:** open alongside a running Claude Code session that spawns agents. Watch the agent's tool calls appear in the timeline detail pane in real-time — even for agents that don't emit progress lines.

3. **Swimlane view (static).** Build the swimlane for completed sessions — lanes, tool blocks, conflict highlighting. **Checkpoint:** press `s` on a completed session with parallel agents, verify lane layout and conflict markers.

4. **Swimlane live wiring.** Connect the live subagent tailer to the swimlane — tool blocks append in real time, lane status changes, live conflict detection. **Checkpoint:** open alongside a running Claude Code session that spawns agents. Watch swimlane lanes fill with tool blocks in real time. This is the "construction site control room" moment.

---

## Scope

### Agent tree builder

- `internal/agent/tree.go`:
  - **Subagent JSONL files are the primary data source for agent tool calls.** Each agent's tool calls live in `<sessionId>/subagents/agent-a<agentId>.jsonl`. These files are always present (created by Claude Code when the agent spawns) and contain the full tool call history. Parsed using the same `parser/jsonl.go` pipeline as the parent session.
  - **Subagent file path resolution:** `filepath.Join(sessionDir, sessionId, "subagents", "agent-a"+agentId+".jsonl")` where `sessionDir` is the directory containing the parent JSONL and `sessionId` is `Session.ID`. If the file does not exist, log to stderr and use whatever data is available from progress lines / toolUseResult summary.
  - **Progress lines are supplementary.** `progress` lines in the parent JSONL (`data.type: "agent_progress"`) provide early real-time visibility (used by M4's ticker). When subagent files are available, they are authoritative — progress lines may be incomplete or absent entirely.
  - Linkage chain: Agent `tool_use.id` → `toolUseResult.agentId` → subagent file `agent-a<agentId>.jsonl`; progress lines link via `parentToolUseID` and `data.agentId` (when present)
  - Rich `toolUseResult` metadata available on Agent completion: `totalDurationMs`, `totalTokens`, `totalToolUseCount`, `status`
  - **Parallel detection (exact, timestamp-based):** Agent B is parallel with Agent A if `timestamp(Agent B tool_use)` < `timestamp(Agent A tool_result)`. B was spawned before A returned.
  - **Unresolvable linkage:** if session ID linkage cannot be resolved for an agent, add it to `Session.UnlinkedAgents`, emit a parse warning to stderr, generate `WarnAgentUnlinked` — do NOT guess attachment by time proximity or any other method
  - Nested agents (agents spawning agents): full depth supported. Subagent files for nested agents live in the same `subagents/` directory.
  - **No retry detection** — removed from v1
  - `AgentNode.IsParallel = true` when parallel topology confirmed
  - File conflict detection: `WarnAgentConflict` when two parallel agents wrote/edited the same path — based on actual file paths from subagent tool calls
- `Session.UnlinkedAgents []*AgentNode` — add this field to `Session` in `model/types.go`

### Live subagent file tailing

This is what makes the swimlane a live dashboard instead of a post-mortem viewer. Without it, agents that don't emit progress lines show empty lanes until they complete — the same black-box experience as Claude Code itself.

- `internal/agent/watcher.go`:
  - When an Agent `tool_use` event arrives during a live session, the agent watcher begins watching for the corresponding subagent JSONL file
  - **File appearance:** The subagent file may not exist immediately after the Agent `tool_use`. Watch the `<sessionId>/subagents/` directory via fsnotify for file creation. Once `agent-a<agentId>.jsonl` appears, begin tailing it.
  - **Tailing:** Use the same streaming approach as the parent session watcher (M2's `tail.go`) — buffer until `\n`, parse each line, extract tool calls. Emit `MsgAgentToolCall` events that the UI consumes.
  - **Event:** `MsgAgentToolCall struct { AgentID string; ToolCall *model.ToolCall }` — the swimlane and detail pane append this to the agent's tool call list and re-render.
  - **Lifecycle:** When the parent session receives the Agent `tool_result` (agent completed), stop tailing the subagent file. Do a final read to ensure all lines are captured (the tool_result in the parent may arrive before the last line is flushed to the subagent file).
  - **Multiple concurrent agents:** Each active agent has its own tailer goroutine. The agent watcher manages the lifecycle of all active tailers. Bounded: at most N concurrent tailers where N = number of active agents (typically 1-5, rarely more).
  - **Error handling:** If the subagent file never appears (timeout or unexpected state), log to stderr, show `⚠ no agent file` in the lane, and rely on progress lines and toolUseResult summary. Do not crash or block.
  - **Completed sessions:** No tailing needed — the tree builder reads subagent files in full during the initial parse. The agent watcher only activates for live sessions.

### Agent display in timeline

- Prompt list (left pane):
  - Prompts with agents show agent count badge: `⬡ 3` (three agents)
  - **Live prompt with active agents:** badge shows running count: `⬡ 2 active` — updates as agents complete
  - Completed prompts with failures: `⬡ 2 ✗1` (two succeeded, one failed)
- Detail pane expansion (updates in real time for the active prompt via `MsgPromptUpdate` and `MsgAgentToolCall`):
  - `⬡ subagent-N` nodes selectable with `enter`; expands to show full detail
  - Collapsed view (rich summary — agents are first-class):
    - Label, type if available (e.g., `subagent-1 (Explore)`), task description (<=60 chars)
    - Model name shown when different from parent prompt's model (e.g., `haiku` badge)
    - **Status-aware display:**
      - Running: `running — 4 tools so far — 12s` with running elapsed time
      - Succeeded: `done — 7 tools, 3 files — 18s` with token summary (in/out)
      - Failed: `✗ failed — 2 tools — 5s`
    - `parallel` badge on parallel agents
    - `WarnAgentConflict` highlighted red when present
  - Expanded view:
    - Full task description and task prompt text
    - **Scope summary line:** `"Asked: <truncated prompt> → Touched: N files"` with file list below. This juxtaposition of intent vs. action is the primary signal for spotting agent scope drift — no heuristics needed, just making the data visible.
    - Complete tool call list (same rendering as parent) — populated from subagent JSONL file, with tool calls appearing in real time for active agents
    - Files touched list with per-file op summary (W/R/E counts)
    - If agent has children: nested agent nodes (same collapsed/expanded pattern, indented)
  - Breadcrumb in detail pane header: `#4 > subagent-2 > subagent-2a`; `esc` pops one level
  - Unlinked agents shown in a separate section with `⚠ agent linkage unresolved`

### Swimlane view

The swimlane is kno-trace's flagship view for agent-heavy workflows. It must work as a **live dashboard** during active sessions, not just a post-hoc review tool. When agents are running, the swimlane is where a developer should be looking.

- `internal/ui/swimlane.go`:
  - Prompt selector bar — only prompts with agents; others dimmed
  - **Auto-selects the active prompt** when session is live and current prompt has agents — the user sees agents working immediately when they switch to swimlane. If the user navigates to a different prompt, auto-follow disengages (same behavior as timeline).
  - One lane per actor (parent + each agent), color-coded
  - Lane headers: label, model name (if different from parent), type if available
  - **Lane status indicator:** Each lane shows its current state:
    - Active agent: pulsing/bright lane header with elapsed time — `subagent-1 (Explore, haiku) — 14s`
    - Completed agent: normal lane header with final duration — `subagent-1 — done 18s`
    - Failed agent: muted color with `✗` marker — `subagent-1 — ✗ failed 5s`
  - Tool blocks per lane: show tool type + file path (truncated) — e.g., `Edit parser/jsonl.go`
  - **Live tool blocks append in real time** as `MsgAgentToolCall` events arrive from the subagent file tailer. The user watches blocks appear in each lane as agents work — regardless of whether the agent emits progress lines. This is the "construction site control room" experience.
  - **Active agent's latest tool block is highlighted** — visually answers "what is this agent doing RIGHT NOW?"
  - Parent lane shows `spawn agent-N` at Agent call points, `awaiting...`, `synthesize` when done
  - File conflict: shared file paths highlighted in both lanes; warning banner at top
    - **Live conflict detection:** If two active agents write to the same file, the conflict warning appears immediately — don't wait for both agents to finish
  - `enter` on a block shows block details below lanes
  - Nested agents: indented sub-lanes within parent agent's lane region
  - "No agents in this prompt" message when applicable
  - `s` key activates swimlane view

### Build order clarifications

- **Layer 1 (tree builder) focuses on data correctness.** The `--dump` output is the primary verification mechanism. Agent detail expand/collapse with breadcrumb navigation in the detail pane is deferred to layer 3, not layer 1.
- **Layer 3 (swimlane) includes timeline detail pane enhancements.** The expand/collapse agent view, breadcrumb navigation (`#4 > subagent-2 > subagent-2a`), and full agent detail rendering in the detail pane are built alongside the swimlane — both are "agent visualization" work and share styling/interaction patterns.

### Known limitations (v1)

- **Nested agent tailing adds complexity.** If an agent spawns its own sub-agents, those sub-agents have their own JSONL files. v1 supports nested agents in the tree structure and reads their files on completion, but does not tail nested subagent files in real-time. The parent agent's file IS tailed, so the nested Agent tool_use/tool_result is visible — just not the nested agent's individual tool calls until it completes.

---

## Acceptance Criteria

### Agent tree builder
- Subagent JSONL files read successfully; agent's ToolCalls and FilesTouched populated from subagent file
- Missing subagent file handled gracefully (log warning, use toolUseResult summary data)
- Agent tool calls visible in expanded view and swimlane
- File conflict detection uses actual file paths from subagent tool calls
- `--dump` output shows agent tool calls from subagent files

### Live subagent tailing
- When agent spawns during live session, its subagent JSONL file is tailed
- Agent tool calls appear in the detail pane and swimlane in real time — even for agents that emit zero progress lines
- File conflict detection works in real time from tailed subagent tool calls
- Tailing stops cleanly when agent completes
- Missing subagent file shows `⚠ no agent file` indicator, does not crash or block
- Multiple concurrent agent tailers work correctly

### Timeline
- Agent count badge correct in prompt list
- Failed agent badge correct in prompt list
- Expand/collapse and breadcrumb work correctly, including nested agents
- Collapsed agent view shows model, duration, tokens, file count
- Agent type shown when available from Agent input
- Parallel badge shown correctly
- Unresolvable agents shown in `UnlinkedAgents` section, not guessed
- `WarnAgentUnlinked` visible in UI

### Swimlane — live behavior
- Swimlane auto-selects active prompt when session is live
- Lane status shows running/done/failed with elapsed time
- Tool blocks append in real time as agent tool calls arrive from subagent tailer
- Active agent's latest tool block is visually highlighted
- File conflict warning appears immediately when two active agents touch the same file
- Swimlane remains responsive during rapid tool call updates

### Swimlane — static behavior
- File conflict warning shown correctly with shared paths highlighted
- Swimlane lane headers show model when different from parent
- Swimlane tool blocks show file paths from subagent JSONL
- Swimlane renders correctly for: single agent, two parallel agents, failed agent, nested agents

---

## Notes

- Test against `internal/testdata/parallel_agents.jsonl` and associated subagent files from M0
- The parallel detection algorithm must use the exact timestamps from the log
- The subagent file tailer reuses the same streaming/parsing logic as the parent session watcher — do not build a separate parser
- **Verified subagent file naming (2026-04-05):** Files are `agent-a<hexId>.jsonl` where hexId is a 16-char lowercase hex string. Path: `<sessionDir>/<sessionId>/subagents/agent-a<agentId>.jsonl`. Each agent also has an `agent-a<agentId>.meta.json` with `{"agentType":"...","description":"..."}`. The `agentId` in `toolUseResult` maps directly to the filename suffix.
