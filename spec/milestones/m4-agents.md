# M4: Agent Tree & Swimlane

**Prerequisites:** Read [spec/README.md](../README.md) (core spec) and `SCHEMA.md`. M3 must be complete (timeline view working). Pay special attention to the AgentNode data model — agents are first-class citizens in kno-trace.

**Goal:** Subagents fully visible — expanded inline in timeline and rendered as parallel lanes in swimlane. Both live (while agents run) and post-hoc (reviewing completed sessions).

**Deliverable:** Multi-agent workflow users see exactly what each agent was asked, what it touched, and whether agents ran in parallel — in real time as it happens, not just after the fact.

**Use cases served:** UC4 (what are my agents doing?) — this is THE use case for kno-trace's target audience. The swimlane is the flagship view. A developer should be able to glance at kno-trace and immediately understand the state of every active agent.

---

## Scope

### Agent tree builder

- `internal/agent/tree.go`:
  - Session ID linkage per `SCHEMA.md` findings — exact field names only
  - **Parallel detection (exact, timestamp-based):** Agent B is parallel with Agent A if `timestamp(Task B tool_use)` < `timestamp(Task A tool_result)`. B was spawned before A returned.
  - **Unresolvable linkage:** if session ID linkage cannot be resolved for an agent, add it to `Session.UnlinkedAgents`, emit a parse warning to stderr, generate `WarnAgentUnlinked` — do NOT guess attachment by time proximity or any other method
  - Nested agents (agents spawning agents): full depth supported
  - **No retry detection** — removed from v1
  - `AgentNode.IsParallel = true` when parallel topology confirmed
  - File conflict detection: `WarnAgentConflict` when two parallel agents wrote/edited the same path
- `Session.UnlinkedAgents []*AgentNode` — add this field to `Session` in `model/types.go`

### Agent display in timeline

- Prompt list (left pane):
  - Prompts with agents show agent count badge: `⬡ 3` (three agents)
  - **Live prompt with active agents:** badge shows running count: `⬡ 2 active` — updates as agents complete
  - Completed prompts with failures: `⬡ 2 ✗1` (two succeeded, one failed)
- Detail pane expansion (updates in real time for the active prompt via `MsgPromptUpdate`):
  - `⬡ subagent-N` nodes selectable with `enter`; expands to show full detail
  - Collapsed view (rich summary — agents are first-class):
    - Label, type if available (e.g., `subagent-1 (Explore)`), task description (<=60 chars)
    - Model name shown when different from parent prompt's model (e.g., `haiku` badge)
    - **Status-aware display:**
      - Running: `running ��� 4 tools so far — 12s` with running elapsed time
      - Succeeded: `done — 7 tools, 3 files — 18s` with token summary (in/out)
      - Failed: `✗ failed — 2 tools — 5s`
    - `parallel` badge on parallel agents
    - `WarnAgentConflict` highlighted red when present
  - Expanded view:
    - Full task description and task prompt text
    - Complete tool call list (same rendering as parent)
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
  - **Live tool blocks append in real time** as `MsgPromptUpdate` events arrive. The user watches blocks appear in each lane as agents work. This is the "construction site control room" experience.
  - **Active agent's latest tool block is highlighted** — visually answers "what is this agent doing RIGHT NOW?"
  - Parent lane shows `spawn agent-N` at Task call points, `awaiting...`, `synthesize` when done
  - File conflict: shared file paths highlighted in both lanes; warning banner at top
    - **Live conflict detection:** If two active agents write to the same file, the conflict warning appears immediately — don't wait for both agents to finish
  - `enter` on a block shows block details below lanes
  - Nested agents: indented sub-lanes within parent agent's lane region
  - "No agents in this prompt" message when applicable
  - `s` key activates swimlane view

---

## Acceptance Criteria

### Timeline
- Agent count badge correct in prompt list
- Failed agent badge correct in prompt list
- Expand/collapse and breadcrumb work correctly, including nested agents
- Collapsed agent view shows model, duration, tokens, file count
- Agent type shown when available from Task input
- Parallel badge shown correctly
- Unresolvable agents shown in `UnlinkedAgents` section, not guessed
- `WarnAgentUnlinked` visible in UI

### Swimlane — live behavior
- Swimlane auto-selects active prompt when session is live
- Lane status shows running/done/failed with elapsed time
- Tool blocks append in real time as agent tool calls arrive
- Active agent's latest tool block is visually highlighted
- File conflict warning appears immediately when two active agents touch the same file
- Swimlane remains responsive during rapid tool call updates

### Swimlane — static behavior
- File conflict warning shown correctly with shared paths highlighted
- Swimlane lane headers show model when different from parent
- Swimlane tool blocks show file paths
- Swimlane renders correctly for: single agent, two parallel agents, failed agent, nested agents

---

## Notes

- Test against `internal/testdata/parallel_agents.jsonl` from M0
- The parallel detection algorithm must use the exact timestamps from the log
