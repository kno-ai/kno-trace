# kno-trace Development Guidelines

## Use Cases Drive Features

Every feature decision must trace back to a use case defined in `spec/README.md#use-cases`. Before implementing or designing a feature, identify which use case(s) it serves. If it doesn't serve one, it doesn't belong in v1. The use cases are:

- **UC1:** "What did Claude just do?" — real-time prompt-by-prompt visibility
- **UC2:** "What happened to this file?" — complete file change history within a session
- **UC3:** "What changed between then and now?" — session-scoped diff between any two points
- **UC4:** "What are my agents doing right now?" — live visibility into multi-agent workflows. This is THE use case. Claude Code shows nothing while agents run. kno-trace is the only window in.
- **UC5:** "Is my context filling up?" — passive context window monitoring
- **UC6:** "Which files are getting thrashed?" — identify hot spots and churn
- **UC7:** "Let me quickly check a past session" — fast session discovery and review

The overarching vision is **"control room for Claude Code"** — you glance at kno-trace to know the state of things, drill in when something looks wrong, and leave it running in a second terminal. Every feature should serve this: passive monitoring with on-demand depth.

When in doubt about scope, complexity, or priority, ask: "Which use case does this serve, and does it make the control room better?"

---

## Inviolable Rules

These rules cannot be broken under any circumstances. No feature, optimization, convenience, or dependency justifies violating them. They are not defaults that can be toggled — they are architectural constraints. If a proposed change would require relaxing any of these, the change is rejected.

- **Never transmit data.** kno-trace makes zero network connections. No telemetry, no analytics, no update checks, no crash reports, no DNS lookups, no license validation. The binary must function identically air-gapped. There is no code path that opens a socket. No dependency may phone home. This means: no HTTP clients in the dependency tree, no "optional" reporting, no deferred network calls behind feature flags.
- **Never write to the user's filesystem** (outside of kno-trace's own config directory). kno-trace is read-only by design. It reads JSONL logs and its own config. It writes nothing else. No temp files in the project directory, no sidecar files next to sessions, no cache files outside config dir, no lock files, no pid files.
- **Never capture or store session content.** kno-trace displays session data in memory while running. It does not copy, cache, or persist any session content beyond what the user's config file contains. When kno-trace exits, session data is gone. No replay logs, no session snapshots to disk, no "recent sessions" cache with content.
- **Never collect usage data in any form.** No counters, no timings, no feature-usage flags, no anonymous statistics. Not locally, not remotely. The user's workflow is their own.
- **No obfuscated behavior.** Every behavior of kno-trace must be explainable by reading the source. No hidden state, no undocumented flags, no compile-time toggles that change behavior. If it's not in the source and the config, it doesn't exist.
- **Dependencies must be auditable.** Every dependency must be open source with a compatible license. Prefer well-known, widely-audited libraries. No vendored binaries, no pre-compiled blobs, no dependencies that pull from remote sources at build time beyond the Go module proxy. `go mod vendor` must produce a complete, buildable tree.
- **No platform lock-in.** kno-trace builds and runs on macOS, Linux, and Windows from the same source. No platform-specific features that degrade the experience elsewhere. No CGO requirement unless absolutely unavoidable (and it is avoidable for this project).

## Resource Discipline

kno-trace runs alongside Claude Code — it must never compete for resources.

- **Bounded memory.** No unbounded in-memory lists, maps, or buffers. Every collection that grows with session size must have a cap or a strategy:
  - `FileHistory.WriteSnapshots`: cap stored snapshots per config (`max_snapshots_per_file`). Evict oldest, never evict most recent.
  - `ToolCall.Output` (Bash output): truncated to 500 chars — enforce at parse time.
  - Search/filter results: work on indices, not copies of data.
  - Large JSONL replay: process line-by-line streaming, never load entire file into memory.
- **Bounded CPU.** No polling loops. Use fsnotify events, not timers. Diff computation is on-demand (user navigates to it), never pre-computed for all file pairs. Fuzzy search debounces input — don't re-score on every keystroke.
- **Bounded disk.** Config file only. No logs to disk by default. If debug logging is added, it must be opt-in and size-capped (rotating, max size in config).
- **Graceful with large sessions.** A 100MB JSONL with 500 prompts must not OOM or freeze. Design for streaming and lazy computation. If a session is too large for a view, degrade gracefully (truncate lists with "N more..." indicators) rather than crash.

## Conventions & Predictability

- **Follow established TUI conventions.** Keybindings follow vim/lazygit/k9s patterns. `j/k` navigate, `g/G` top/bottom, `/` search, `?` help, `esc` back, `q` quit. Don't invent novel interactions when a convention exists.
- **Consistent behavior across views.** Same key does the same conceptual thing everywhere. `esc` always means "back/dismiss/cancel." `enter` always means "select/expand." `q` always quits. `/` always searches. No view should surprise the user.
- **Predictable navigation model.** Views are a flat set (timeline, swimlane, heatmap, diff) — not a deep hierarchy. Single-letter keys switch views. `esc` always moves toward "less detail." `P` always returns to the session picker. The user should always know where they are and how to get back.
- **Show what you know, hide what you don't.** If data is unavailable (no token counts, no model name, no baseline for diff), hide the UI element entirely. Never show "0%", "unknown", or placeholder text. Absence is cleaner than noise.
- **No heuristics in v1.** If a value cannot be derived exactly from the JSONL, don't display it. No guessing, no "approximately", no fuzzy classifications. This applies to Bash risk, agent retry detection, and any other pattern matching.
- **Errors are visible, not fatal.** Malformed data, missing fields, unresolvable links — display a clear indicator in the UI and continue. Never crash on bad input. Never silently swallow errors. The user should be able to distinguish "this data doesn't exist" from "this data couldn't be parsed."

## Configurability

- **Configurable where users would reasonably disagree.** Thresholds (context% warnings), display preferences (auto-open behavior), and values that change over time (context window sizes) belong in config. Internal implementation details do not.
- **Sensible defaults for everything.** The tool must work perfectly with zero configuration. Config exists for users who want to tune, not as a setup requirement. First run with no config file must be indistinguishable from a configured run with all defaults.
- **Config is a single file.** `~/.config/kno-trace/config.yaml`, resolved via `os.UserConfigDir()`. No environment variables overriding config, no multiple config locations, no config merging. One file, one source of truth.
- **Config is forward-compatible.** Unknown keys are ignored. Removing a config key reverts to the default. The tool never fails because of a config key it doesn't recognize.

## Resilience — Never Break on Unexpected Data

Claude Code's JSONL format is not versioned, not documented by Anthropic, and will change without notice. kno-trace must be **reliable in the face of unexpected data**. This is a core design constraint, not a nice-to-have.

**The rule: if the parser encounters anything it doesn't recognize, it skips it and continues. It never panics, never crashes, never hangs.**

Specific defensive practices:
- **Unknown line types** (`type` field not recognized): skip silently. Claude Code adds new line types regularly.
- **Unknown tool names**: classify as `ToolOther`, display by name. Never fail because a tool isn't in the known list.
- **Unknown content block types** (not `text`, `tool_use`, `thinking`): skip the block, process the rest of the message.
- **Unknown system subtypes**: skip. Only process what we recognize (`turn_duration`, `compact_boundary`).
- **Missing fields**: every field access must have a safe default. Missing `message` → skip the line. Missing `content` → empty list. Missing `usage` → zero tokens. Missing `model` → empty string. Missing `timestamp` → zero time. Missing `file_path` on a tool_use → empty string (don't display a path).
- **Null values where objects expected**: treat `null` the same as missing. `content: null` → empty list. `usage: null` → zero tokens.
- **Empty arrays**: `content: []` is valid — it means no content blocks. Never index into an array without checking length.
- **Type mismatches**: `toolUseResult` can be a string, dict, list, null, or absent. Always check the type before accessing fields. Never assume it's a dict.
- **Forward-compatible parsing**: use `json.RawMessage` or equivalent for fields we don't fully understand. Don't fail on unknown keys in any object.
- **Circular references in agent trees**: guard against infinite recursion. Use a visited set or depth limit.
- **Future-proofing**: write parsing code as "extract what I know, ignore what I don't" rather than "validate that the structure matches my expectations."

**Test this explicitly.** The `edge_cases.jsonl` fixture contains malformed JSON, blank lines, and unexpected structures. Every parser code path should be exercised against it without panicking.

## Code Quality

- **Comment classes, methods, and properties well** focusing on the "why" — why is this important, what is it responsible for, etc.
- **Single source of truth.** Domain types in `internal/model/`. Styles in `ui/styles.go`. Formatting in `ui/format.go`. Config in `config/config.go`. Don't define the same concept in two places.
- **No dead code.** Don't leave commented-out code, unused imports, or placeholder functions. If something isn't needed yet, don't write it.
- **Errors are information.** Malformed JSONL lines, missing files, unresolvable agent links — log to stderr and continue. Never crash on bad input. Never silently swallow errors that would help the user understand what happened.
- **Reproducible builds.** `go build` with the same source and Go version must produce functionally identical binaries. No build-time randomness, no timestamp embedding beyond what goreleaser provides for versioning.

---

## Implementation Learnings

Hard-won knowledge from building each milestone. Read before starting a new milestone — these reflect real behavior observed in production JSONL files, not spec assumptions.

### M1: Session Picker

- **First JSONL line is not always a user message.** Real sessions often start with `file-history-snapshot` lines that lack `timestamp` and `cwd` fields. The meta parser must scan forward through the first ~10 lines to find these fields. Don't assume line 1 has what you need.
- **Not all line types carry `timestamp`.** `file-history-snapshot` and some `queue-operation` lines omit it. Any code extracting timestamps must check for presence, not assume.
- **`cwd` field is the source of truth for project path.** The encoded directory name under `~/.claude/projects/` is ambiguous to reverse (dashes in real directory names). Extract the real project path from the `cwd` field on user/assistant message lines instead.
- **JSONL lines can be large.** Assistant messages with streaming snapshots produce single lines over 2MB. Use `bufio.Reader.ReadBytes('\n')` instead of `bufio.Scanner` — it grows dynamically with no cap. `bufio.Scanner` has a fixed max buffer and stops entirely when exceeded, silently losing all subsequent lines.
- **`go test` changes working directory** to the package directory. `os.Getwd()` in tests returns `internal/discovery/`, not the project root. `FindCWDSessions()` returning 0 in tests is correct behavior, not a bug.
- **Global keybindings vs view-local input.** Any key handled globally (like `q` for quit) will intercept that key during text input (like fuzzy filter). Keys that conflict with text input must be handled per-view, not globally. Only `ctrl+c` is safe as a true global.

### M2: Parser & Builder

- **`message.content` on user lines has 3 shapes.** Plain string (human turns in some fixtures), array of text blocks (human turns in others), array of tool_result blocks. Check `sourceToolAssistantUUID` presence as the most reliable indicator of tool results — more reliable than inspecting content block types.
- **`toolUseResult` has 4 shapes.** Plain string (Write/Edit/MCP), object with `file` (Read), object with `stdout/stderr/exitCode` (Bash), object with `status/agentId/totalDurationMs` (Agent). The `content` field within Agent results can itself be a string or an array.
- **Streaming snapshots are complete replacements.** Each snapshot for a given `requestId` contains the full message content at that point — not a delta. For parallel agents, each Agent `tool_use` may appear in a DIFFERENT snapshot. Must collect tool_use blocks from ALL snapshots and merge, deduplicating by tool_use ID.
- **`.claude/` is 8 characters, not 7.** Off-by-one when computing string index after `.claude/` prefix. Use `len(prefix)` instead of hardcoded offsets.
- **Duration outlier detection: strict >2σ.** Prompts exactly at the threshold (mean + 2*stddev) are NOT flagged. The last prompt's EndTime may be zero (not sealed by a following human turn) — use session EndTime as fallback.
- **Progress lines appear AFTER the main conversation flow** in some fixtures. Don't assume chronological ordering by line position. Sort all events by timestamp after parsing.
- **`promptId` on tool_result lines is inconsistent.** Sometimes present, sometimes absent. Do not rely on it for tool_result lines — use `sourceToolAssistantUUID` and content block type instead.
- **Real JSONL lines can exceed 2MB.** Use `bufio.Reader.ReadBytes('\n')` everywhere — no buffer cap needed. `bufio.Scanner` silently fails and loses all remaining lines when the buffer is exceeded. Tested against 59 real sessions — largest line was 2.18MB.
- **Some sessions contain only `file-history-snapshot` lines.** No user/assistant messages, so 0 prompts. This is valid — display the header with zero prompts, don't crash.

### M4: Live Timeline & Ticker

- **`classifyToolName` was renamed to `ClassifyToolName` (exported)** to allow the UI package to classify tool names from progress line content blocks for the ticker. Any new callers outside `parser` must use the exported name.
- **`parentToolUseID` is a top-level field on progress lines**, not nested inside `data`. It's a sibling of `type`, `data`, `uuid`, etc. The `agentId` is inside `data`.
- **Progress line embedded messages are double-nested.** Structure is `data.message.message.content[]`. The outer `message` has `type` (user/assistant) and the inner `message` has the actual API message with `content`, `model`, `usage`, etc.
- **Incremental rebuild must re-classify the active prompt on each update.** Clear `prompt.Warnings` before calling `classifyPrompt()` to avoid duplicate warnings accumulating across incremental rebuilds.
- **`IsLive` = watcher running AND last prompt unsealed.** Not just "watcher running" — completed sessions where all prompts are sealed should not show the live dot even if the watcher is technically watching.
- **Duration outliers are NOT recomputed during incremental rebuilds.** They require all prompts with final EndTimes and are only meaningful for completed sessions. The initial `BuildSession` computes them; `RebuildActivePrompt` does not.
- **Agent color assignments persist across prompt resets.** When `ticker.ResetForNewPrompt()` clears entries and loop counts, the `agentColors` map is preserved so agents keep consistent colors across prompts.
- **Ticker entries are separate from `prompt.ToolCalls`.** The ticker tracks its own entries (including subagent tool calls from progress lines). `prompt.ToolCalls` remains parent-session direct tool calls only. This preserves the data model for M5's agent tree builder.
- **`tea.Tick` lifecycle.** The 1-second tick starts on `MsgReplayDone` for live sessions and self-reissues each time. It naturally stops when the user navigates away (view changes) or `IsLive` becomes false. No explicit stop needed.
- **`isHumanTurn` was renamed to `IsHumanTurn` (exported)** so the watcher can reuse the same logic. The watcher adds its own `evt.Type == "user"` guard since it sees all event types; the builder already switches on type before calling.
- **`RebuildActivePrompt` receives only new events, not the full accumulated slice.** After `MsgReplayDone`, `a.events` is freed. Each `MsgNewEvents` batch is passed directly with `startIdx=0`. This prevents unbounded memory growth during long live sessions.
- **Zero-value Ticker is safe.** `Push()` and `AgentColor()` defensively initialize nil maps. This handles the edge case where a session starts with 0 prompts and the ticker wasn't initialized via `NewTicker()`.
- **Negative terminal dimensions.** `setSize()` clamps `detailWidth` and `contentHeight` to `≥ 1` to prevent layout corruption in very small terminals.

### M5: Agent Tree & Swimlane (Layer 1)

- **Subagent file naming verified against real data.** Format is `agent-a<hexId>.jsonl` where hexId is a 16-char lowercase hex string. Path: `<sessionDir>/<sessionId>/subagents/agent-a<agentId>.jsonl`. The `agentId` from `toolUseResult` maps directly to the filename suffix. Each agent also has an `agent-a<agentId>.meta.json` with type/description.
- **Subagent JSONL files are self-contained sessions.** All lines have `isSidechain: true` and `agentId` fields. They share the parent's `sessionId`. The same `parser.ParseFile` pipeline works for parsing them — no separate parser needed.
- **Parallel detection belongs in the agent package, not the parser.** The parser creates `AgentNode` stubs; `agent.EnrichSession()` runs `detectParallelAgents()` after reading subagent files. Parallel = Agent B spawned before Agent A completed (timestamp-based, exact).
- **File conflict detection requires subagent file data.** `FilesTouched` is populated from actual tool calls in subagent files, not from progress lines. Conflicts are only flagged between parallel agents where at least one performed a write/edit.
- **`ensurePickerLoaded` was replaced with `refreshPicker`.** The old function skipped rescans when sessions were cached, causing lockouts when files were deleted. Always rescan on `P` key or watcher error.
- **Watcher must send an error message when file open fails.** `MsgWatcherError` was added — without it, the app gets stuck in "Loading..." forever when a session file is missing.
