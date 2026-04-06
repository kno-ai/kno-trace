# M5: Agent Data Layer (COMPLETE)

**Status:** Complete. Data layer built and tested. Swimlane view built then removed — the timeline detail pane is the right home for agent data.

**What was built:**
- Agent tree builder (`internal/agent/tree.go`): reads subagent JSONL files, populates ToolCalls/FilesTouched/ModelName, detects parallel topology, detects file conflicts
- Live subagent tailing (`internal/agent/watcher.go`): fsnotify-based, tails subagent files in real time, emits MsgAgentToolCall
- Agent display in timeline detail pane: collapsed summary with status/model/parallel badges, enter-to-expand with breadcrumb navigation, full tool call list and file ops in expanded view
- Prompt list badges: ⬡N active, ⬡N ✗M
- Live enrichment: `enrichCompletedAgent` does authoritative full-file read on tool_result
- `--dump` output shows agent tool calls, files touched, conflicts

**What was learned:**
- The swimlane view (separate screen showing agent lanes) was built, tested, then removed. It duplicated the timeline's detail pane without adding new information. The two-panel layout (prompts left, detail right) is the right abstraction — drill in for depth, don't switch views.
- The collapsed agent summary shows basically the same info Claude Code already shows. The value add is in the **detail** — file changes, diffs, live tool streams — which is M6's territory.
- Navigation must be consistent: enter to drill in, esc to back out, j/k for selection at every level.

**What remains (moved to M6):**
- Agent detail that actually adds value over Claude Code's output
- Inline diffs for agent edits
- File activity summaries showing churn across agents
- Live tool call streaming visible without drilling in
