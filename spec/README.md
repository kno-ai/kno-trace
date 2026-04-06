# kno-trace — Project Specification

> The control room for Claude Code.
> See what Claude is doing — right now, prompt by prompt. Read-only. No interference.

**Design principle: kno-trace displays only what is directly known from the log. No guesses. No approximations. No heuristic classifications. If a value is not in the JSONL, it is not shown.**

---

## Table of Contents

1. [Vision](#vision)
2. [Use Cases](#use-cases)
3. [Non-Goals v1](#non-goals-v1)
4. [Tech Stack](#tech-stack)
5. [JSONL Format Reference](#jsonl-format-reference)
6. [Go Data Model](#go-data-model)
7. [Architecture](#architecture)
8. [Session Discovery](#session-discovery)
9. [Key Design Decisions](#key-design-decisions)
10. [Configuration](#configuration)
11. [Display Formatting](#display-formatting)
12. [Milestones](#milestones)
13. [Testing Strategy](#testing-strategy)
14. [Distribution](#distribution)
15. [Reference: Full Keybindings](#reference-full-keybindings)

---

## Vision

Claude Code is a black box while it runs. You see output but not the path taken. kno-trace is the control room.

A developer opens a second terminal alongside Claude Code and runs `kno-trace`. Immediately, they see the session state: which prompt is active, how long it's been running, how full the context window is, and whether agents are working. Tool calls stream across the ticker as Claude works. They can drill into any prompt, compare file states between prompts, and spot which files are being thrashed — all without touching Claude Code's session.

For multi-agent workflows, kno-trace is the only window into what's happening. Claude Code's terminal shows "waiting" while agents work in the background. kno-trace shows every agent's activity in real time — what each one is reading, writing, and building, whether they're running in parallel, and whether they're stepping on each other's files.

**kno-trace is a control room, not a debugger.** You glance at it to know the state of things. You drill in when something looks wrong. You leave it running in a second terminal and it stays out of your way until you need it.

**Core principle: kno-trace is purely observational. It writes nothing. It touches nothing. It reads only the JSONL logs that Claude Code already writes.**

---

## Use Cases

These are the concrete problems kno-trace solves. Every feature must trace back to at least one. If a proposed feature doesn't serve a use case here, it doesn't belong in v1.

### UC1: "What did Claude just do?"

A developer is running Claude Code and wants to see, prompt by prompt, what actions were taken — which files were written, edited, or read, what Bash commands ran, what agents were spawned. They want this in real time as Claude works, and they want to drill into any prompt for full detail.

**Served by:** Static timeline (M3), live timeline (M4), live tail (M2), `--dump` (M2)

### UC2: "What happened to this file?"

A developer notices a file is in an unexpected state and wants to trace its complete history within the session — every read, write, and edit, in order, with diffs showing exactly what changed at each step.

**Served by:** Heatmap file history (M6), inline mini-diffs (M6), snapshot reconstruction (M6)

### UC3: "What changed between then and now?"

A developer wants to compare the state of all files at one point in the session versus another — a session-scoped diff that captures every intermediate state, not just git commits.

**Served by:** Diff view (M7), `m` mark workflow (M7)

### UC4: "What are my agents doing right now?"

This is the primary use case for kno-trace's target audience. A developer running multi-agent workflows has near-zero visibility in Claude Code itself — the parent session just shows "waiting" while agents work. kno-trace is the only way to see inside.

In real time, the developer needs to know:
- **Which agents are active right now** and which have finished
- **What each active agent is doing** ��� its most recent tool call, files it's touching
- **Whether agents are running in parallel** or sequentially
- **Whether agents are conflicting** — writing to the same files simultaneously
- **When an agent fails** — immediately, not after the parent resumes

After agents complete, the developer needs:
- What each agent was asked to do, what model it used, what files it touched
- Full tool call history per agent, expandable inline
- Side-by-side comparison of parallel agent timelines

This use case should feel like watching multiple workers on a construction site from a control room. Not a post-mortem — a live feed.

**Served by:** Live ticker with agent attribution (M4), agent activity in stats bar (M4), agent tree and live tailing (M5), agent detail expansion with inline diffs and file activity (M5/M6)

### UC5: "Is my context filling up?"

A developer wants to monitor context window usage across a session — seeing when it's getting high, when `/compact` was used, and whether they should start a new session. They want this as a passive indicator, not something they have to check.

**Served by:** Context% badges (M3), context nudge in ticker (M8), compact markers (M3)

### UC6: "Which files are getting thrashed?"

A developer suspects Claude is repeatedly modifying the same files and wants to identify hot spots — files with disproportionate edit activity — to understand whether the session is making progress or spinning.

**Served by:** Heatmap view (M6), HeatScore intensity bars (M6), `/` file search (M6)

### UC7: "Let me quickly check a past session"

A developer wants to review what happened in an earlier Claude Code session — maybe from yesterday, maybe from a different project. They want to find it fast and see the summary without re-running anything.

**Served by:** Session picker (M1), auto-open latest (M1), `--dump` (M2), `P` to switch sessions

---

## Non-Goals v1

- No cross-session comparison (data model must accommodate it, but not implemented)
- No prompt×file matrix view (deferred to IDEAS.md — heatmap + search covers the use case)
- No sidecar/annotation storage — everything derived dynamically from the JSONL
- No file rollback or modification — read-only throughout
- No web UI, no server, no daemon
- No authentication or multi-user scenarios
- No network access of any kind — no telemetry, no update checks, no data transmission
- No Claude Code API integration — only file-based observation
- No Bash command risk classification — Bash commands are shown as-is; risk assessment is heuristic and belongs in a future version
- No agent retry detection — detecting "similar" task descriptions is heuristic; not in v1
- File state reconstruction reflects only Write/Edit tool calls. Files modified by Bash, PowerShell, NotebookEdit, MCP tools, or external processes are not captured — the diff view notes when these tools were present in a range

---

## Tech Stack

All decisions are final. Do not substitute.

| Layer | Choice | Reason |
|---|---|---|
| Language | Go 1.24+ | Single static binary, trivial cross-compilation, strong concurrency |
| TUI framework | [Bubbletea](https://github.com/charmbracelet/bubbletea) | Elm-architecture TUI, mature, widely used |
| Styling | [Lipgloss](https://github.com/charmbracelet/lipgloss) | ANSI color, borders, layout for Bubbletea |
| Components | [Bubbles](https://github.com/charmbracelet/bubbles) | viewport, list, textinput — do not rebuild these |
| File watching | [fsnotify](https://github.com/fsnotify/fsnotify) | Cross-platform inotify/kqueue/ReadDirectoryChanges |
| Diff computation | [go-diff](https://github.com/sergi/go-diff) | Myers diff between file state snapshots |
| Fuzzy search | [sahilm/fuzzy](https://github.com/sahilm/fuzzy) | Lightweight, no external binary needed |
| Release | [goreleaser](https://goreleaser.com/) | Multi-platform builds + Homebrew tap automation |

**Color palette (non-negotiable — brand consistency with kno ecosystem):**
```
Background:  #0A0E14
Brand teal:  #4DFFC4
```

**Terminal compatibility target:**
- macOS: Terminal.app, iTerm2, Alacritty, Kitty, Ghostty
- Linux: gnome-terminal, konsole, xterm, Alacritty, any VTE-based
- Windows: Windows Terminal (minimum bar), PowerShell 7+
- Multiplexers: tmux, screen (termenv handles capability detection automatically)

---

## JSONL Format Reference

Claude Code writes one JSON object per line to session files.

> **IMPORTANT:** The schema below reflects current understanding. Field names must be confirmed against real session files during M0 before any parsing code is written. `SCHEMA.md` produced in M0 is the authoritative reference — if it conflicts with this spec, `SCHEMA.md` wins.

### File location

```
<claudeDir>/projects/<hash>/
  <timestamp>.jsonl      # one file per session
  <timestamp>.jsonl
```

`claudeDir` is resolved at runtime via `os.UserHomeDir()` — never hardcode `~`. Use `filepath.Join()` for all path construction. This ensures correct behavior on Windows (`%USERPROFILE%\.claude\projects\`), macOS, and Linux.

The `<hash>` is derived from the absolute project path. The exact algorithm must be confirmed in M0.

### Known message structure

**Human/user turn** — marks the start of a prompt unit:
```json
{
  "type": "user",
  "message": {
    "role": "user",
    "content": [{ "type": "text", "text": "user prompt text here" }]
  },
  "uuid": "...",
  "sessionId": "...",
  "timestamp": "2024-04-04T14:32:11.000Z"
}
```

**Assistant turn** — may contain text and/or tool_use blocks:
```json
{
  "type": "assistant",
  "message": {
    "role": "assistant",
    "content": [
      { "type": "text", "text": "I'll start by..." },
      {
        "type": "tool_use",
        "id": "toolu_01...",
        "name": "Write",
        "input": { "path": "internal/parser/jsonl.go", "content": "package parser\n..." }
      }
    ]
  },
  "uuid": "...",
  "sessionId": "...",
  "timestamp": "2024-04-04T14:32:14.000Z",
  "usage": { "input_tokens": 4210, "output_tokens": 380 },
  "model": "claude-sonnet-4-5"
}
```

**Tool result** — follows each tool_use, appears as a user-role message:
```json
{
  "type": "user",
  "message": {
    "role": "user",
    "content": [{
      "type": "tool_result",
      "tool_use_id": "toolu_01...",
      "content": [{ "type": "text", "text": "File written successfully" }]
    }]
  },
  "uuid": "...",
  "sessionId": "...",
  "timestamp": "2024-04-04T14:32:15.000Z"
}
```

**Distinguishing human turns from tool results:** Both appear with `role: "user"`. A human turn has `message.content[0].type === "text"`. A tool result has `message.content[0].type === "tool_result"`. This distinction is the foundation of prompt boundary detection. Confirm the exact check in M0.

### Known tool names and their inputs

| Tool | Key input fields | Notes |
|---|---|---|
| `Write` | `path`, `content` | Full file content — complete snapshot |
| `Read` | `path` | Result content in tool_result — use as file baseline |
| `Edit` | `path`, `old_str`, `new_str` | str_replace — this IS the diff |
| `Bash` | `command` | Shell command — result in tool_result; shown as-is |
| `Agent` | `description`, `prompt`, `subagent_type` (optional) | Spawns subagent — critical for agent tree. `subagent_type` (e.g., "Explore", "Plan") confirmed in M0 |
| `Glob` | `pattern`, `path` (optional) | File pattern matching — shown as non-file op |
| `Grep` | `pattern`, `path` (optional) | Content search — shown as non-file op |
| `ToolSearch` | `query` | Deferred tool lookup — shown as non-file op |
| `TaskCreate` | task fields | Task management — shown as info only |
| `TaskUpdate` | task fields | Task management — shown as info only |
| `TodoWrite` | `todos` | Task list update — render as info only |
| `WebSearch` | `query` | External search — shown as non-file op |
| `WebFetch` | `url` | Web page fetch — shown as non-file op |
| `Skill` | `skill`, `args` (optional) | Skill invocation — shown as non-file op |
| `ExitPlanMode` | — | Exits plan mode — shown as info only |
| MCP tools | varies by server | `mcp__<server>__<tool>` naming convention — flagged as irreversible |

### Subagent identification

Subagent messages are stored in **separate JSONL files**: `<sessionId>/subagents/agent-a<agentId>.jsonl`. The parent session file may contain `progress` lines with `data.type: "agent_progress"` that embed subagent messages inline, but **progress lines are not reliably present** — confirmed in real sessions where general-purpose agents performed file writes with zero progress lines in the parent JSONL.

**v1 approach — progressive agent visibility:**
- **Agent spawned:** Agent `tool_use` block (subagent_type, description, prompt)
- **Agent completed:** `tool_result` with `toolUseResult` metadata (status, totalDurationMs, totalTokens, totalToolUseCount)
- **Agent progress (when present):** `progress` lines with embedded messages — use for real-time visibility when available. These provide live tool call attribution (M4 ticker).
- **Agent file modifications:** The agent's individual tool calls (Write, Edit, etc.) live in the subagent JSONL file at `<sessionId>/subagents/agent-a<agentId>.jsonl`. Progress lines embed some tool calls in the parent, but are not reliably present for all agent types.

**v1 reads and tails subagent files (M5).** The agent tree builder reads each subagent's JSONL file to populate the agent's `ToolCalls` and `FilesTouched`. For live sessions, the agent watcher tails subagent files in real-time — tool calls appear in the swimlane and detail pane as the agent works. This is read-only (consistent with core principles) and uses the same parser as the parent session. The `agentId` from `toolUseResult` maps directly to the filename.

**Progressive visibility across milestones:**
1. **M4 (live):** Progress lines give real-time tool call attribution in the ticker — when progress lines are present. Agents without progress lines show running status only.
2. **M5 (live + complete):** Subagent JSONL files are tailed in real-time for active agents (all agents visible, regardless of progress lines) and read in full for completed agents. The swimlane always shows agent activity.

**Agent modifications in heatmap and diff:** The replay engine (M6) includes agent tool calls in its file history, interleaved by timestamp. Agent Writes become snapshots, agent Edits apply in sequence — the reconstruction algorithm works unchanged. This means the heatmap, file history, and diff view all reflect the complete picture of what happened to each file, regardless of whether the parent or an agent made the change.

**Linkage chain:** Agent `tool_use.id` → progress lines' `parentToolUseID` (when present) → `data.agentId` → tool_result's `toolUseResult.agentId`

**Parallel agents and streaming snapshots:** When Claude spawns multiple agents in one response, each Agent `tool_use` may appear in a **different streaming snapshot** (same `requestId`, different lines). The final snapshot (with `stop_reason != null`) may only contain the LAST agent's tool_use. The parser MUST collect tool_use blocks from ALL snapshots for a given `requestId` to detect parallel agents correctly. See SCHEMA.md for details.

### Usage metadata and context%

Token counts appear on assistant messages in a `usage` field (`input_tokens`, `output_tokens`). The `model` field names the Claude model.

**Context% is derived directly from `input_tokens` on the last assistant message in each prompt.** When Claude sends a request, `input_tokens` already represents the full conversation history in that request — it is the actual context window fill, not a running total. Divide by the model's context window size to get an exact percentage. The context window size is resolved from the `model` field on that assistant message using the model-to-window-size mapping in configuration (see [Configuration](#configuration)). If `input_tokens` is not present in the log, `ContextPct` is 0 and no context% is displayed — do not estimate. If the model name is not recognized in the mapping, use the configured default window size.

### Streaming snapshots

Assistant responses are written as **multiple JSONL lines** sharing the same `requestId` and `message.id`. Intermediate lines have `stop_reason: null`; the final line has `stop_reason: "end_turn"` or `"tool_use"`. Each line is a complete replacement of the message content (not a delta).

For parsing:
- Track lines by `requestId`
- Only use the line with `message.stop_reason != null` as the final response
- For live display, show the latest line for each `requestId` and replace when a new one arrives

### Thinking blocks

Assistant `message.content` arrays may contain `{"type": "thinking", "thinking": "...", "signature": "..."}` blocks. These contain model reasoning, not user-facing content — skip them for display purposes.

### Meta messages

User messages with `isMeta: true` have `message.content` as a **raw string** (not a list). These are local commands, not human turns. The parser must check `isMeta` first and skip these lines.

### Non-message line types

In addition to `user` and `assistant` lines, the JSONL contains these line types:

- `progress` — agent/hook progress updates (embeds subagent messages via `data.type: "agent_progress"`)
- `system` — turn duration, compact boundary, local commands
- `queue-operation` — session queue management (enqueue/dequeue/remove)
- `file-history-snapshot` — file backup tracking
- `last-prompt` — records last user prompt text

The parser should handle these gracefully — use what is useful, skip the rest.

### `/compact` handling

Claude Code's `/compact` command summarizes conversation history. It appears as a `system` line with `subtype: "compact_boundary"`. The line includes `compactMetadata.trigger` (`"auto"` for context pressure, `"manual"` for user-initiated `/compact`) and `compactMetadata.preTokens` (token count before compaction). When detected:
- Record the prompt index in `Session.CompactAt []int`
- Context% automatically reflects the smaller context after compact (since `input_tokens` on the next assistant message will be lower)
- Display a visual compact marker on the prompt where it occurred
- No manual token counter reset needed

### Partial line and error handling

Claude Code flushes complete JSON objects terminated by `\n`. Buffer bytes until `\n` before attempting JSON parse. If a line fails JSON parsing, log the line number and error to stderr, skip the line, and continue — do not crash or stop processing.

**Truncated files:** During initial replay (reading existing file content), if the file ends with a partial line (bytes with no trailing `\n`), discard the incomplete buffer — do not block waiting for more data. During live tail, incomplete trailing bytes are held in the buffer normally (new data will complete the line). This prevents hangs on crash-truncated session files.

---

## Go Data Model

These types live in `internal/model/`. All other packages import from here. Do not define domain types elsewhere.

```go
package model

import "time"

// Session represents one Claude Code session file.
type Session struct {
    ID          string
    FilePath    string    // absolute path to .jsonl file
    ProjectPath string    // inferred from ~/.claude/projects/<hash> reverse mapping
    ProjectName string    // last component of ProjectPath, for display
    ModelName   string    // primary model, from first assistant message; individual prompts may differ
    StartTime   time.Time
    EndTime     time.Time // zero if session still live (no final timestamp seen)
    Prompts     []*Prompt
    IsLive      bool      // set by watcher: true only while actively receiving new lines
    CompactAt   []int     // prompt indices where /compact occurred (may be multiple)
    Interrupted bool      // true if session ended without a final assistant turn
}

// Prompt represents one human turn and everything Claude did in response.
// Bounded by: start = this human message, end = next human message (or EOF).
type Prompt struct {
    Index       int
    HumanText   string
    StartTime   time.Time
    EndTime     time.Time // zero if this is the active/live prompt
    ModelName   string    // model from last assistant message in this prompt; may differ per prompt
    TokensIn    int       // input_tokens from last assistant message; 0 if unavailable
    TokensOut   int       // output_tokens from last assistant message; 0 if unavailable
    CacheRead   int       // cache_read_input_tokens if present; 0 if unavailable
    CacheCreate int       // cache_creation_input_tokens if present; 0 if unavailable
    ToolCalls   []*ToolCall
    Agents      []*AgentNode
    Warnings    []Warning
    // ContextPct: input_tokens of last assistant message / model's context window size * 100.
    // Window size resolved from ModelName via configuration.
    // 0 means token data was not available in the log — do not display context% in that case.
    ContextPct  int
    Interrupted bool      // true if this was the last prompt and session was cut short
}

// ToolCall represents a single tool invocation by any actor (parent or agent).
type ToolCall struct {
    ID          string
    SourceAgent string    // ID of the agent that made this call; empty = parent session
    Type        ToolType
    Timestamp   time.Time

    // File operations (Write, Read, Edit)
    Path    string
    Content string   // Write: full new content; Read: content from tool_result
    OldStr  string   // Edit
    NewStr  string   // Edit

    // Bash
    Command  string
    ExitCode int    // from tool_result; -1 if not available
    Output   string // truncated to 500 chars

    // Agent (subagent spawn) — only populated when Type == ToolAgent
    SpawnedAgentID      string // the agentId of the spawned subagent
    AgentDescription    string
    AgentPrompt         string
    SubagentType        string

    // MCP / other
    MCPToolName   string
    MCPServerName string
    MCPInput      map[string]any

    // Derived — set by classifier
    IsCLAUDEMD bool // true if path matches CLAUDE.md memory file patterns (see classify.go)
}

type ToolType string
const (
    ToolWrite     ToolType = "write"
    ToolRead      ToolType = "read"
    ToolEdit      ToolType = "edit"
    ToolBash      ToolType = "bash"
    ToolAgent     ToolType = "agent"
    ToolGlob      ToolType = "glob"
    ToolGrep      ToolType = "grep"
    ToolWebSearch ToolType = "websearch"
    ToolMCP       ToolType = "mcp"
    ToolOther     ToolType = "other"
)

// AgentNode is a subagent spawned during a prompt.
// Agents are first-class citizens in kno-trace — advanced users (multi-agent workflows,
// MCP-heavy sessions) are the primary audience. Display agent details richly.
type AgentNode struct {
    ID              string
    SessionID       string       // conversation ID used by this agent in the log
    Label           string       // generated: "subagent-1", "subagent-2"
    ModelName       string       // model used by this agent (from its assistant messages)
    SubagentType    string       // subagent_type from Agent input if present (e.g., "Explore", "Plan")
    TaskDescription string
    TaskPrompt      string
    ParentPromptIdx int
    ParentAgentID   string       // empty if direct child of prompt
    ToolCalls       []*ToolCall
    Children        []*AgentNode // nested agents
    FilesTouched    []string     // unique file paths from this agent's tool calls, deduped
    StartTime       time.Time
    EndTime         time.Time
    Duration        time.Duration // EndTime - StartTime; zero if still running
    TokensIn        int          // sum of input_tokens across this agent's assistant messages
    TokensOut       int          // sum of output_tokens across this agent's assistant messages
    TotalDurationMs int          // from toolUseResult — authoritative summary from Claude Code
    TotalTokens     int          // from toolUseResult — authoritative total token count
    TotalToolUseCount int        // from toolUseResult — authoritative tool use count
    Status          AgentStatus  // running/succeeded/failed — exact, from tool_result presence
    IsParallel      bool         // true if ran concurrently with a sibling agent
}

// AgentStatus is determined exactly from the JSONL — not heuristic.
// Running: Agent tool_use seen, but no tool_result yet for this Agent.
// Succeeded: Agent tool_result received and indicates success.
// Failed: Agent tool_result received and indicates failure.
type AgentStatus string
const (
    AgentRunning   AgentStatus = "running"
    AgentSucceeded AgentStatus = "succeeded"
    AgentFailed    AgentStatus = "failed"
)

// Warning flags a factual observation about a prompt.
// No warnings are heuristic — each fires on exact, observable conditions.
type Warning struct {
    Type    WarnType
    Message string
}

type WarnType string
const (
    WarnContextHigh     WarnType = "context_high"     // ContextPct > config.ContextHighPct
    WarnContextCritical WarnType = "context_critical" // ContextPct > config.ContextCriticalPct
    WarnMCPExternal     WarnType = "mcp_external"     // MCP tool call made — irreversible side effect
    WarnInterrupted     WarnType = "interrupted"      // session ended during this prompt
    WarnAgentConflict   WarnType = "agent_conflict"   // parallel agents wrote/edited same file path
    WarnReplayGap       WarnType = "replay_gap"       // Edit old_str not found — reconstruction incomplete
    WarnAgentUnlinked   WarnType = "agent_unlinked"   // subagent messages found but linkage unresolvable
)

// FileHistory tracks all interactions with a file across the session.
// Memory management: WriteSnapshots retains only the most recent
// config.max_snapshots_per_file (default 10) snapshots per file.
// When the cap is reached, the oldest snapshot is evicted — but never
// the most recent Write (needed as base for Edit replay).
// GetContentAt() for evicted indices falls back to replaying from
// the nearest surviving snapshot.
type FileHistory struct {
    Path           string
    PromptIdxs     []int    // prompt indices that touched this file, in order
    Ops            []string // op type per entry: "W","R","E","bash"
    SourceAgents   []string // agent label per entry: "" = parent, "subagent-1" = agent (M6)
    HeatScore      int      // count of distinct prompts with Write or Edit ops, including agent ops (exact)
    HasBaseline    bool     // true if a pre-session Read baseline is available
    WriteSnapshots map[int]string // full content after each Write op, capped per config
    ReadBaseline   string         // content from first Read op on this file
}

// SessionMeta is lightweight — used by the session picker without full parse.
// All fields are exact values derived directly from the filesystem and
// the first/last few lines of the JSONL. No approximations.
type SessionMeta struct {
    ID            string
    FilePath      string
    ProjectPath   string
    ProjectName   string
    StartTime     time.Time  // timestamp of first line in JSONL
    EndTime       time.Time  // timestamp of last line in JSONL
    FileSizeBytes int64      // exact, from os.Stat
    Duration      time.Duration // EndTime - StartTime
}
```

### Snapshot Reconstruction Algorithm

To get file content at prompt index `N`:

1. Find the latest Write op on this file at prompt index `<= N`. Use `WriteSnapshots[writeIdx]` as base; set `baseIdx = writeIdx`.
2. If no Write exists, use `ReadBaseline` as base; set `baseIdx = -1`.
3. If neither exists, return `ErrNoBaseline`.
4. Collect all Edit ops on this file with prompt index in `(baseIdx, N]`, in order.
5. Apply each Edit sequentially: find first occurrence of `OldStr`; if not found, record `WarnReplayGap` on the prompt and skip this edit; replace with `NewStr`.
6. Return resulting content.

This is the single authoritative implementation. The replay engine, diff view, and mini-diff renderer all call `replay.Engine.GetContentAt(path, promptIdx)`.

---

## Architecture

```
cmd/kno-trace/
  main.go                — flag parsing, session selection, bootstrap

internal/
  model/
    types.go             — ALL domain types — single source of truth

  discovery/             — (M1)
    pathenc.go           — project path encoding (slash-to-dash)
    scan.go              — scans projectsDir, builds []SessionMeta
    meta.go              — populates SessionMeta from first/last JSONL lines;
                           scans for timestamp and cwd fields (first line may be
                           file-history-snapshot or other non-message type)

  parser/                — (M2)
    jsonl.go             — streams JSONL lines, emits RawEvent per line (never loads full file)
    builder.go           — assembles RawEvent stream into *model.Session
    classify.go          — sets IsCLAUDEMD; identifies MCP tool calls; generates Warnings
                           NOTE: no Bash risk classification — Bash shown as-is

  replay/                — (M6)
    engine.go            — builds FileHistory; implements GetContentAt()
    diff.go              — computes unified diff between two content strings

  watcher/               — (M2)
    tail.go              — fsnotify live tailer; replays existing lines on startup;
                           emits MsgPromptSealed, MsgPromptUpdate, MsgReplayDone,
                           MsgSessionFileDeleted
                           CONCURRENCY NOTE: never access Session or model types
                           directly from the watcher goroutine — communicate only
                           via tea.Msg events

  agent/                 — (M5)
    tree.go              — builds AgentNode tree from Agent tool linkage;
                           reads subagent JSONL files for full tool call history;
                           populates agent metadata (model, tokens, duration, files touched);
                           extracts subagent_type from Agent input if present;
                           detects parallel vs sequential (timestamp-based, see M5);
                           if linkage unresolvable: emits parse warning, attaches
                           agent to Session.UnlinkedAgents, does NOT guess
    watcher.go           — live subagent file tailer; watches for subagent JSONL
                           file creation when Agent tool_use arrives; tails each
                           active agent's file; emits MsgAgentToolCall events;
                           stops tailing when agent completes (tool_result received);
                           only active during live sessions

  config/
    config.go            — configuration loading, defaults, access (M1)

  ui/
    app.go               — root Bubbletea model, view routing, keyboard, resize (M1)
    styles.go            — ALL lipgloss styles and colors — build this FIRST (M1)
    format.go            — shared display formatting utilities (M1)
    picker.go            — session picker screen (M1)
    timeline.go          — timeline: prompt list + detail pane + search/filter (M3)
    swimlane.go          — parallel agent swimlane (M5)
    heatmap.go           — file tree heatmap (M6)
    diff.go              — diff view (M7)
    components/
      promptlist.go      — scrollable prompt list (M3)
      detail.go          — prompt detail renderer (M3)
      minidiff.go        — inline diff renderer, 8 context lines max (M6)
      ticker.go          — live activity ticker strip (M4)

internal/testdata/       — fixture JSONL files (created in M0)

SCHEMA.md                — created in M0, authoritative field name reference
spec/                    — project specification (this directory)
IDEAS.md                 — deferred features and future ideas
README.md                — skeleton in M1, completed in M8
.goreleaser.yaml         — M8 only
.github/workflows/       — M8 only
```

**Add `UnlinkedAgents []*AgentNode` to `Session`** for agents whose session ID linkage could not be resolved. These are displayed in the timeline with a `⚠ agent linkage unresolved` indicator. This field is not in the type definition above — add it when implementing M5.

### Data flow

```
JSONL file
    │
    ▼
watcher.Tailer            — streams complete lines (existing replay, then live)
    │
    ▼
parser.Builder            — assembles Session/Prompt tree
    │                       incremental: emits MsgPromptSealed as each prompt seals
    │
    ���──► agent.TreeBuilder  — resolves session IDs → AgentNode tree per prompt (M5)
    │
    ├──► replay.Engine      — builds FileHistory index; GetContentAt() on demand (M6)
    │
    └──�� ui.App             — receives tea.Msg events; re-renders active view
```

**Ordering constraint:** Agent tree builder runs after each prompt seals. Replay engine builds FileHistory after full parse; GetContentAt() is lazy. Parser runs in M2, agent tree in M5, replay engine in M6 — do not pre-implement upstream interfaces before they are needed.

### Bubbletea events

```go
type MsgPromptSealed       struct{ PromptIdx int }
type MsgPromptUpdate       struct{ PromptIdx int }
type MsgReplayDone         struct{}
type MsgSessionFileDeleted struct{}
type MsgAgentToolCall      struct{ AgentID string; ToolCall *ToolCall } // M5: from subagent file tailer
```

All views receive `tea.WindowSizeMsg` and must update their internal dimensions.

---

## Session Discovery

### Path resolution

```go
homeDir, _   := os.UserHomeDir()
claudeDir    := filepath.Join(homeDir, ".claude")
projectsDir  := filepath.Join(claudeDir, "projects")
```

Never hardcode `~`. Always use `filepath.Join()`.

### Project path encoding

The directory name under `projectsDir` is **not a hash** — it is the absolute project path with `/` replaced by `-`. For example, `/Users/careykevin/code/kno-ai/kno-trace` becomes `-Users-careykevin-code-kno-ai-kno-trace`.

**Matching CWD:** Encode the CWD using the same slash-to-dash replacement and compare against directory names. Do not attempt to reverse-map directory names back to paths (ambiguous because directory names can contain `-`). Implement in `internal/discovery/hash.go`.

### CLI interface

```
kno-trace                  # auto-open latest CWD session if recent; otherwise picker
kno-trace --pick           # always open picker (all sessions, all projects)
kno-trace --session <path> # explicit JSONL path; relative paths expanded via filepath.Abs
kno-trace --version
kno-trace --help
```

**Default behavior (no flags):**
1. Find sessions for the CWD project (hash-based lookup).
2. If the most recently modified session was updated within `auto_open_max_age_hours` (default 24h, configurable): auto-open it. Show a status bar message: `"Opened latest session for <project> — P to pick a different session"`.
3. If no session is recent enough, or no sessions exist for CWD: open the full session picker.
4. If `~/.claude/projects/` does not exist: show "No sessions found — run Claude Code in a project directory first".

**`P` (shift-p) from any view:** return to the session picker. This is always available — the user is never locked into a session.

**`--session` with path outside `~/.claude/projects/` structure:** Use the filename as session ID and the parent directory name as project name. Watcher still works — it only needs the file path.

### Session picker behavior

- All sessions across all projects under `projectsDir`, sorted by mtime descending
- Grouped by date (Today / Yesterday / earlier dates)
- Per entry: project name, start time, duration, file size in bytes
- **No live indicator in the picker.** Whether a session is live is only known once the watcher is active. The picker shows factual metadata only.
- Fuzzy filter by typing (matches project name); `j`/`k` navigate; `enter` opens; `q`/`esc` quits
- When `~/.claude/projects/` does not exist or contains no JSONL files: show "No sessions found — run Claude Code in a project directory first"
- `q` exits cleanly from any screen — no confirmation prompt

---

## Key Design Decisions

All decisions are final for v1. Do not revisit during implementation.

**1. Read-only, no sidecar.** kno-trace writes nothing. Purely observational — zero risk of interfering with Claude Code.

**2. No git.** Diffs reconstructed from JSONL via the Snapshot Reconstruction Algorithm. More complete than git — captures every intermediate state within a session.

**3. Session = one JSONL file.** The data model (Session has ProjectPath, SessionMeta is a separate type) accommodates cross-session analysis in a future version but does not implement it.

**4. Prompt unit = human turn boundary.** Everything between two consecutive non-tool-result user messages belongs to the enclosing prompt.

**5. Agent tree from Agent tool linkage only — no fallback guessing.** The `agent.TreeBuilder` uses the exact Agent tool_use ID → progress line → toolUseResult linkage. If that linkage cannot be resolved for a given agent, the agent is placed in `Session.UnlinkedAgents` and marked `WarnAgentUnlinked`. We do not guess parent assignments by time proximity or any other heuristic.

**6. Parallel agents detected by timestamp — exact.** Two agents are parallel if `Agent B's tool_use timestamp` is before `Agent A's tool_result timestamp`. That is: B was spawned before A returned. This is an exact determination from log timestamps, not a heuristic.

**7. Bash commands shown as-is.** No risk classification. No mutation warning per-prompt. In the diff view, when Bash tool calls are present in the selected range, an informational note is shown: "bash commands present in this range — file effects not captured." This fires on any `ToolBash` call in the range — deterministic, not classified.

**8. Context% is exact when available.** `ContextPct = input_tokens of last assistant message in prompt / modelContextWindowSize * 100`. The window size is resolved from the model name on that assistant message via the configured model-to-window-size mapping (see [Configuration](#configuration)). This is the actual context fill reported by the API — not estimated. If `input_tokens` is absent from the log, `ContextPct = 0` and no context% is displayed for that prompt.

**9. CLAUDE.md detection is exact filename matching.** `classify.go` sets `IsCLAUDEMD = true` for writes/edits to paths matching this exact list (case-insensitive filename comparison):
```
CLAUDE.md
AGENTS.md
.claude/memory*
.claude/settings*
.claude/commands*
```
No "similar" patterns. Only this list.

**10. MCP calls are any tool name with `mcp__` prefix.** MCP tools follow the `mcp__<server>__<tool>` naming convention — use prefix match on `mcp__` to detect them. Deterministic.

**11. `Session.IsLive` is set only by the watcher.** It is `true` only while the watcher goroutine is actively tailing the file and receiving fsnotify events. It is never inferred from file modification time.

**12. `--dump` is a permanent feature.** The `--dump` CLI flag introduced in M2 for testing is a permanent diagnostic tool. Keep it functional in all subsequent milestones. Do not remove it as "scaffolding."

**13. Incremental display during startup replay.** `MsgPromptSealed` is emitted per prompt during replay. The UI renders prompts as they appear — never a blank screen while waiting for replay to complete.

**14. Zero network access.** kno-trace makes no network connections of any kind. No telemetry, analytics, update checks, crash reports, or DNS lookups. No code path exists that opens a socket. This is inviolable — see CLAUDE.md.

**15. Bounded resource usage.** kno-trace runs alongside Claude Code and must never compete for resources. JSONL files are processed as a line stream — never loaded entirely into memory. Diff computation and file replay are on-demand. Snapshot caches are capped. See CLAUDE.md Resource Discipline for full policy.

---

## Configuration

kno-trace avoids hardcoded values for anything that may vary by model, change over time, or reflect user preference. Configuration is loaded from `~/.config/kno-trace/config.yaml` (XDG-compatible, resolved via `os.UserConfigDir()`). If no config file exists, all defaults apply. Unknown keys are ignored (forward-compatible).

```yaml
# Model context window sizes (tokens). Used for ContextPct calculation.
# Key = model name substring match (e.g., "opus" matches "claude-opus-4-6").
# First match wins; more specific patterns should appear first.
# If no pattern matches, default_context_window is used.
model_context_windows:
  "opus":   1000000
  "sonnet": 200000
  "haiku":  200000
default_context_window: 200000

# Warning thresholds
context_high_pct: 70       # ContextPct above this -> WarnContextHigh
context_critical_pct: 85   # ContextPct above this -> WarnContextCritical
context_nudge_pct: 80      # ContextPct above this -> ticker strip nudge message

# Session auto-open behavior
auto_open_max_age_hours: 24 # auto-open latest CWD session if modified within this window

# Loop detection
loop_detection_threshold: 3 # same tool+path repeated this many times -> WarnLoopDetected

# Resource limits
max_snapshots_per_file: 10  # max WriteSnapshots retained per file (oldest evicted first)

# Safety limits — fail-safes against corrupted or malicious files.
# Normal sessions are far below these limits. Adjust only if you see
# "too large" warnings on legitimate session files.
max_file_size_mb: 1024      # reject session files above this (default 1GB)
max_line_size_mb: 100       # skip individual JSONL lines above this (default 100MB)
max_picker_sessions: 200    # max sessions shown in picker (most recent kept)
```

**Implementation notes:**
- Config is loaded once at startup. No hot-reload needed for v1.
- `internal/config/config.go` owns loading, defaults, and access. All other packages read from config — never hardcode these values.
- Model name matching: lowercase substring match against the `model` field from the JSONL. E.g., a model field of `"claude-opus-4-6"` matches the `"opus"` key.
- Unknown config keys are silently ignored (forward-compatible). Missing config file uses all defaults. Tool must work identically with no config file.

---

## Display Formatting

All numeric values displayed in the UI use human-readable adaptive formatting. Implementation lives in a shared `internal/ui/format.go` utility — never format inline.

| Value | Rule | Examples |
|---|---|---|
| File size | Bytes < 1 KB -> `"742 B"`; < 1 MB -> `"12.3 KB"`; < 1 GB -> `"2.4 MB"`; else `"1.1 GB"` | `742 B`, `12.3 KB`, `2.4 MB` |
| Duration | < 60s -> `"32s"`; < 60m -> `"4m 12s"`; else `"1h 14m"` | `32s`, `4m 12s`, `1h 14m` |
| Token count | < 1000 -> as-is; < 1M -> `"4.2k"`; else `"1.1M"` | `380`, `4.2k`, `1.1M` |
| Context % | Integer with `%` suffix, no decimals | `12%`, `81%` |
| Line delta | `+N` / `-M` — always signed | `+42`, `-7` |
| Prompt text | Truncated to available width with `...` — never wrap in list views | `scaffold the Go proj...` |

---

## Milestones

Implement milestones strictly in order. Each milestone has its own spec file in `spec/milestones/`. Re-read this core spec and `SCHEMA.md` before starting each milestone.

Each milestone builds progressively toward the control room vision:

| Milestone | Description | Control Room Progress | Spec |
|---|---|---|---|
| M0 | Schema Investigation | Ground truth — confirm what the JSONL gives us | [m0-schema.md](milestones/m0-schema.md) |
| M1 | Session Picker | Front door — find and open a session, see the summary card | [m1-session-picker.md](milestones/m1-session-picker.md) |
| M2 | Parser, Builder & Live Tail | Data pipeline — JSONL becomes structured data, live streaming works | [m2-parser.md](milestones/m2-parser.md) |
| M3 | Static Timeline | **Core layout** — navigable timeline with badges, search, detail pane | [m3-static-timeline.md](milestones/m3-static-timeline.md) |
| M4 | Live Timeline & Ticker | **Control room comes alive** — real-time updates, ticker, loop detection, auto-follow | [m4-live-timeline.md](milestones/m4-live-timeline.md) |
| M5 | Agent Data Layer | ✅ Agent tree builder, live subagent tailing, enrichment — data ready | [m5-agents.md](milestones/m5-agents.md) |
| M6 | Rich Detail Pane | **The value milestone** — inline diffs, file intelligence, live agent activity, drill-in everywhere | [m6-heatmap.md](milestones/m6-heatmap.md) |
| M7 | Session-Scoped Diff | Before/after comparison — what changed between any two prompts | [m7-diff.md](milestones/m7-diff.md) |
| M8 | Release Polish & Distribution | Ship it — `brew install kno-trace`, README with control room pitch | [m8-release.md](milestones/m8-release.md) |

**The control room is usable at M3.** By M4 it's a daily driver for live sessions. M5 built the agent data layer. **M6 is where kno-trace becomes genuinely useful** — inline diffs, file churn, live agent activity. This is the milestone that makes a developer want to keep it open. M7-M8 add analytical depth and polish.

---

## Testing Strategy

See [testing.md](testing.md) for full testing strategy.

---

## Distribution

```
github.com/<user>/kno-trace/
  cmd/kno-trace/
  internal/
  SCHEMA.md                          — M0
  spec/                              — project specification
  IDEAS.md
  README.md
  LICENSE
  CHANGELOG.md
  .goreleaser.yaml                   — M8
  .github/workflows/release.yaml    — M8
```

Homebrew tap: separate repo `github.com/<user>/homebrew-tap`, auto-updated by goreleaser on each release tag.

User install: `brew tap <user>/tap && brew install kno-trace`

Versioning: `v0.x.y` through M7; `v1.0.0` on M8.

### README Assets

Two assets that communicate the "control room" vision:

1. **Hero GIF (record after M4):** Split terminal — Claude Code on the left, kno-trace on the right. Shows: user sends a prompt, Claude spawns agents, kno-trace ticker lights up with color-coded agent activity, switch to swimlane view showing parallel lanes filling in real time, file conflict warning appears. The viewer should think: "I need that second terminal."

2. **Timeline GIF (record after M3):** Shows the timeline view during a live session — prompts appearing one by one, detail pane showing tool calls, context% climbing in the stats bar, active prompt timer counting up. Communicates the "glance at it and know the state" experience.

---

## Reference: Full Keybindings

Follows vim/TUI conventions (j/k, g/G, /, ?, esc). Consistent with lazygit, k9s, and similar tools that Claude Code users are likely familiar with.

| Key | Context | Action |
|---|---|---|
| `j` / `down` | All list views | Navigate down |
| `k` / `up` | All list views | Navigate up |
| `g` | All list views | Jump to top |
| `G` | All list views | Jump to bottom |
| `enter` | All list views | Expand/select; drill into agent node |
| `esc` | Anywhere | Back / collapse / clear marks / dismiss overlay |
| `/` | Timeline, Heatmap, Swimlane | Search/filter (prompts by text/file/tool; files by path; agents by label/file) |
| `?` | Anywhere | Toggle help overlay |
| `q` | Anywhere | Quit cleanly, no confirmation |
| `P` | Any session view | Return to session picker |
| `t` | Any session view | Switch to timeline view |
| `s` | Any session view | Switch to swimlane view |
| `h` | Any session view | Switch to heatmap view |
| `d` | Any session view | Switch to diff view |
| `m` | Timeline | Mark prompt for diff (`[A]` first, `[B]` second opens diff) |
| `f` | Heatmap | Open file history for selected file |
| `tab` | Views with sub-tabs | Cycle sub-tabs |
