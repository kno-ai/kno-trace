# kno-trace — Deferred Ideas & Future Features

Ideas for post-v1, organized by priority. Priority reflects: strength of user demand signal, uniqueness vs. existing tools, and alignment with kno-trace's "control room" vision.

**Guiding principle:** The strongest ideas make the invisible visible without adding opinion. kno-trace shows facts the JSONL already contains but that nobody can see today.

**Priority framework:**
- **Priority 1** — Broad audience, high daily-driver value, achievable with existing infrastructure
- **Priority 2** — Clear demand, distinct kno-trace angle, moderate effort
- **Priority 3** — Useful but narrower audience or higher effort
- **Priority 4** — Speculative, constrained, or niche

---

## Promoted to v1 Milestones

The following ideas were close enough to existing milestone work that they've been incorporated directly. They are documented here for context and evidence of demand — see the milestone specs for implementation details.

### Loop / Spin Detection → M2 (classify) + M4 (ticker)

Detect when the agent is stuck repeating the same failing approach. Same tool+file pair repeating N times triggers `WarnLoopDetected`. Ticker shows `⟳ possible loop` indicator. Configurable via `loop_detection_threshold` in config (default 3).

**Why it fit:** M2's classifier already inspects every tool call. M4's ticker already streams them. Detection is ~30 lines in the classifier; display is a conditional in the ticker. The infrastructure was 90% there.

**Evidence of demand:**
- [DEV: "How to Tell If Your AI Agent Is Stuck (With Real Data From 220 Loops)"](https://dev.to/boucle2026/how-to-tell-if-your-ai-agent-is-stuck-with-real-data-from-220-loops-4d4h)
- [AWS DEV: "Why AI Agents Fail: 3 Failure Modes That Cost You Tokens and Time"](https://dev.to/aws/why-ai-agents-fail-3-failure-modes-that-cost-you-tokens-and-time-1flb)
- [QuickLeap: "AI Agents Stuck in Reasoning Loops: Why Claude Burns Tokens"](https://quickleap.io/blog/ai-agents-reasoning-loops-token-problem)

### Git Branch Tracking → M2 (builder) + M3 (timeline)

Detect `gitBranch` transitions between messages. Show divider in timeline: `── branch: main → feature/auth ──`.

**Why it fit:** The data is already parsed on every message. Detection is comparing consecutive values. Display is a conditional divider in the prompt list. Essentially free.

### Prompt Duration Outliers → M2 (builder) + M3 (timeline)

Flag prompts with duration >2σ above session mean. `⏱ slow` badge in timeline. Only computed when ≥5 prompts exist.

**Why it fit:** `turn_duration` is already parsed. Math is trivial. Badge rendering follows existing patterns.

### Agent Scope Drift Detection → M5 (agent detail)

Scope summary line in expanded agent view: `"Asked: <prompt> → Touched: N files"` with file list. Juxtaposition of intent vs. action — no heuristics.

**Why it fit:** M5's expanded agent view already shows full task prompt and files touched. This just makes the juxtaposition explicit with a summary line. A formatting addition, not a feature.

### Context Pressure Sparkline → M8 (polish)

Unicode sparkline (▁▂▃▄▅▆▇█) showing context% trajectory across all prompts in the stats bar. Compaction points marked with distinct glyph. Shown when ≥3 prompts have context% data.

**Why it fit:** M8 is release polish. The data (per-prompt context%, compaction points) is already computed. The sparkline is a self-contained component that enhances the existing stats bar.

**Evidence of demand:**
- [MindStudio: "What Is Context Rot in Claude Code?"](https://www.mindstudio.ai/blog/what-is-context-rot-claude-code)
- [ksred: "I Built a Cost Tracker — Was Max Worth It?"](https://www.ksred.com/i-built-a-cost-tracker-for-claude-code-to-see-if-my-subscription-was-worth-it/)
- [SitePoint: "Claude Code Context Management Guide"](https://www.sitepoint.com/claude-code-context-management/)

### File Ownership Conflict Map → M5 (already covered)

M5 already detects `WarnAgentConflict` when parallel agents write to the same path and highlights shared files in the swimlane. The core idea is implemented. A richer standalone visualization (Venn diagram of agent file scopes) could be added post-v1 but the essential signal is there.

### Subagent File Reading + Live Tailing → M5 (promoted)

**Promoted to v1.** M5 now reads subagent JSONL files for full tool call history AND tails them in real-time during live sessions. Without this, the swimlane would show empty lanes for agents that don't emit progress lines — which undermines the flagship view. M6's replay engine includes agent tool calls in its file history (interleaved by timestamp), so the heatmap and diff view reflect agent modifications too. See M5 and M6 specs for details.

---

## Priority 1 — Broad audience, high daily-driver value, fully deterministic

These serve every kno-trace user, not just niche workflows. Every value displayed is derived exactly from the JSONL — no external data, no editorial judgment, no configurable thresholds that change the meaning of what's shown.

**Top 3 — build these first, in this order:**

---

### 1. Notification / Alert Hooks

System notifications when key events fire: context exceeds threshold, loop detected, agent failed, session idle too long. Makes the control room work even when the developer isn't looking at it.

**Why #1:** The control room metaphor breaks if you have to keep watching it. A real control room has alarms. Without notifications, kno-trace is a dashboard you must actively stare at — which defeats the "leave it running in a second terminal" vision. This is the lowest-effort, highest-impact feature on the list because every trigger condition is already computed by v1.

**Determinism:** Fully deterministic. Alert triggers are exact thresholds on exact values already computed: `ContextPct >= N`, `WarnLoopDetected` presence, `AgentStatus == AgentFailed`, ticker quiet state timer. No inference, no heuristics.

**Design:**
- Config-driven alert rules:
```yaml
alerts:
  context_high:
    enabled: true
    # uses existing config.ContextHighPct threshold — no new threshold needed
  context_critical:
    enabled: true
  loop_detected:
    enabled: true
  agent_failed:
    enabled: true
  idle:
    enabled: false
    seconds: 120
  cooldown_seconds: 60  # per alert type, prevents spam
  # Optional: custom command instead of system notification
  # command: "my-custom-notifier"
```

**Implementation plan:**

```
internal/alert/
├── alert.go      # AlertManager type, event dispatch, cooldown tracking
├── notify.go     # platform-native notification delivery
└── alert_test.go
```

**`internal/alert/alert.go`** — the core dispatcher:
```go
// AlertManager receives session events and fires notifications.
// It is owned by App and called from handleWatcherMsg / ticker update paths.
type AlertManager struct {
    cfg       *config.Config
    lastFired map[AlertType]time.Time // cooldown tracking per type
    notifier  Notifier
}

type AlertType string
const (
    AlertContextHigh     AlertType = "context_high"
    AlertContextCritical AlertType = "context_critical"
    AlertLoopDetected    AlertType = "loop_detected"
    AlertAgentFailed     AlertType = "agent_failed"
    AlertIdle            AlertType = "idle"
)

// Check is called after each event cycle. It inspects the current session
// state and fires alerts that haven't been fired within the cooldown window.
func (am *AlertManager) Check(session *model.Session, prompt *model.Prompt) {
    // Context alerts: check prompt.ContextPct against config thresholds
    // Loop alerts: check prompt.Warnings for WarnLoopDetected
    // Agent alerts: check prompt.Agents for AgentFailed status
    // All checks are simple field reads on existing model types
}
```

**`internal/alert/notify.go`** — platform delivery:
```go
// Notifier sends a system notification. No external dependencies.
type Notifier struct {
    customCmd string // if set, exec this instead of platform default
}

// Send delivers a notification via the platform-native mechanism.
// Falls back silently if the notification command is unavailable.
func (n *Notifier) Send(title, body string) error {
    if n.customCmd != "" {
        return exec.Command("sh", "-c", n.customCmd).Run()
    }
    switch runtime.GOOS {
    case "darwin":
        // osascript -e 'display notification "body" with title "title"'
    case "linux":
        // notify-send "title" "body"
    case "windows":
        // powershell -Command "New-BurntToastNotification ..."
    }
}
```

**Integration points in existing code:**
- `App.handleWatcherMsg()` in [app.go](internal/ui/app.go) — after `RebuildActivePrompt` returns and after `extractTickerEntries`, call `am.Check(a.session, activePrompt)`
- `Ticker` quiet state — when the ticker's `timeSinceLastActivity` exceeds the idle threshold, fire `AlertIdle`
- Config: add `Alerts` struct to `config.Config`, loaded with the same merge-defaults pattern

**What hooks into what (existing infrastructure):**
- `classifyPrompt()` in [classify.go](internal/parser/classify.go) already generates `WarnContextHigh`, `WarnContextCritical`, `WarnLoopDetected` — alerts simply check for their presence
- `AgentNode.Status` in [types.go](internal/model/types.go) already tracks `AgentFailed` — alerts check this field
- Ticker already tracks quiet state with timestamp — alerts read the gap

**Effort estimate:** ~200 lines new code + ~30 lines config additions. No new dependencies. No model changes. One new package, three integration points in existing code.

---

### 2. Compaction Diff — What Got Forgotten

When `/compact` runs, context is summarized and detail is lost. The JSONL has `compact_boundary` with `preTokens`. kno-trace shows **what prompts existed before vs. after the compaction boundary** — helping the user understand what the agent "forgot."

**Why #2:** Nobody visualizes this. Users know compaction happened but have no way to understand its impact without reading raw JSONL. The lowest-effort feature on the list — M3 already marks compact boundaries with `── compact ──` dividers in [promptlist.go](internal/ui/promptlist.go). This extends that visual treatment to actually be informative.

**Determinism:** Fully deterministic. `Session.CompactAt` stores exact prompt indices from `compact_boundary` lines in the JSONL. Pre/post classification is index comparison. `preTokens` is an exact value from the JSONL. No inference.

**Design:**
- Prompts before the compaction boundary are visually dimmed — they exist in the JSONL but are outside Claude's active memory
- The compact divider is enhanced with token delta: `── compact ── 142k → 28k tokens ──`
- Dimmed prompts are still navigable (user can drill in) but their "forgotten" status is visible at a glance
- Multiple compactions: each boundary dims everything before it. Only prompts after the *last* compaction are "in memory"

**Implementation plan:**

**`internal/model/types.go`** — add pre-tokens to compaction data:
```go
// CompactEvent records a compaction point in the session.
type CompactEvent struct {
    PromptIdx int   // prompt index where compact occurred
    PreTokens int   // token count before compaction (from JSONL)
    PostTokens int  // token count after compaction (from next assistant message)
}

// Replace CompactAt []int with:
// CompactEvents []CompactEvent
```

**`internal/parser/builder.go`** — extract `preTokens` from `compact_boundary`:
```go
case "system":
    if evt.Subtype == "compact_boundary" && currentPrompt != nil {
        ce := model.CompactEvent{
            PromptIdx: currentPrompt.Index,
            PreTokens: evt.CompactPreTokens, // already in RawEvent from JSONL parse
        }
        s.CompactEvents = append(s.CompactEvents, ce)
    }
```

**`internal/ui/promptlist.go`** — dim pre-compaction prompts:
```go
// In renderPromptItem():
// Check if this prompt is before the last compaction boundary
isPreCompact := false
if len(pl.CompactEvents) > 0 {
    lastCompact := pl.CompactEvents[len(pl.CompactEvents)-1]
    isPreCompact = p.Index < lastCompact.PromptIdx
}

// Apply DimStyle to the entire prompt line if pre-compact
if isPreCompact {
    line = DimStyle.Render(line)
}

// Enhanced compact divider with token info:
// ── compact ── 142,031 → 28,412 tokens ──
```

**`internal/ui/detail.go`** — pre-compact indicator in detail pane:
- When viewing a pre-compact prompt, show a subtle header: `⚠ this prompt is outside Claude's active context (pre-compact)`
- Factual, not a warning — just making the boundary visible

**What hooks into what (existing infrastructure):**
- `builder.go` already handles `compact_boundary` at line 143 and 508 — extend to extract `preTokens`
- `promptlist.go` already has `CompactAt map[int]bool` at line 19 — replace with `CompactEvents`
- `promptlist.go` already renders `── compact ──` divider at line 108 — enhance with token delta
- `timeline.go` already syncs `CompactAt` at line 196 — update to sync `CompactEvents`
- `RawEvent` in [jsonl.go](internal/parser/jsonl.go) likely already parses `preTokens` from system lines — verify, add if missing

**Effort estimate:** ~80 lines changed across 4 files. No new packages. No new dependencies. Mostly modifying existing rendering logic.

---

### 3. "What Did I Approve?" — Permission Decision Log

Claude Code prompts for permissions and people click through them fast. kno-trace surfaces a clean, filterable log of every mutation the user approved: "You approved Write to `auth/middleware.go` at 14:32."

**Why #3:** Answers the question "wait, did I let it edit that?" after a session. Fully deterministic — if a `tool_result` exists in the JSONL, the `tool_use` was approved and executed. No inference, no heuristic. Implementable as a filter mode on the existing timeline, not a new view.

**Determinism:** Fully deterministic. The JSONL records every tool_use and its tool_result. A tool_result's existence is proof of approval. The pairing logic already exists in `builder.go`'s `pairToolResult()` and `toolCallsByID` map. This feature just surfaces what's already parsed.

**Design:**
- `a` key in timeline view toggles "approvals" filter mode
- Filter shows only prompts that contain Write, Edit, Bash, or Agent tool calls (the mutation types that require approval)
- Within each prompt, the detail pane highlights only the approved mutations — Reads and Greps are hidden
- Stats bar shows approval count: `12 writes, 3 edits, 8 bash approved`
- The filter is a lens on existing data, not a separate data structure

**Implementation plan:**

**`internal/ui/timeline.go`** — add approval filter mode:
```go
type filterMode int
const (
    filterNone      filterMode = iota
    filterSearch               // existing `/` search
    filterApprovals            // new `a` toggle
)

// In Update(), handle 'a' key:
case "a":
    if t.filterMode == filterApprovals {
        t.filterMode = filterNone
        t.restoreFullList()
    } else {
        t.filterMode = filterApprovals
        t.filteredIdxs = t.computeApprovalIdxs()
        t.applyFilter()
    }
```

**`internal/ui/timeline.go`** — compute approval indices:
```go
// computeApprovalIdxs returns prompt indices that contain mutation tool calls.
func (t *timelineModel) computeApprovalIdxs() []int {
    var idxs []int
    for i, p := range t.session.Prompts {
        for _, tc := range p.ToolCalls {
            if tc.Type == model.ToolWrite || tc.Type == model.ToolEdit ||
               tc.Type == model.ToolBash || tc.Type == model.ToolAgent {
                idxs = append(idxs, i)
                break
            }
        }
    }
    return idxs
}
```

**`internal/ui/detail.go`** — approval-focused detail rendering:
```go
// When filterMode == filterApprovals, the detail pane renders tool calls
// with an approval-focused layout:
//
//   14:32:05  ✓ Write  internal/parser/builder.go
//   14:32:08  ✓ Edit   internal/model/types.go (old_str: 12 chars → new_str: 45 chars)
//   14:32:15  ✓ Bash   go test ./... (exit 0)
//   14:33:01  ✓ Agent  subagent-1 "Explore codebase for..."
//
// Read/Grep/Glob calls are hidden. Each line shows timestamp, op type, path.
// This is a rendering mode, not new data — ToolCall already has Type, Path,
// Timestamp from the parser.
```

**`internal/ui/promptlist.go`** — approval badge:
```go
// When approval filter is active, show mutation count per prompt:
// #7 [14:32-14:35]  3W 1E 2B
// The W/E/B counts already exist as badge logic — reuse formatBadges()
```

**What hooks into what (existing infrastructure):**
- `builder.go` `pairToolResult()` already pairs tool_use → tool_result and populates `ToolCall` fields — approvals are implicit in this data
- `timeline.go` already has filter infrastructure: `filtering bool`, `filter string`, `filteredIdxs []int`, `restoreFullList()` — the approval filter reuses this exact pattern
- `detail.go` already renders tool calls with type, path, and timestamp — the approval view is a subset rendering
- `promptlist.go` already renders W/R/E/B badges — reuse for the approval count display
- The `a` keybinding is currently unused in the timeline view

**Effort estimate:** ~150 lines new code across 3 existing files. No new packages. No new dependencies. No model changes. Pure UI filtering on existing parsed data.

---

### Session Handoff Summary (`--handoff`)

People lose 4-hour sessions and can't recover context. Context loss between sessions is a top complaint. `kno-trace --handoff <session>` generates a structured summary designed to be pasted into a *new* Claude Code session: files modified, key decisions, where it left off.

**Why P1:** This is not a log viewer — it's a **context restoration artifact** for developers. Existing tools show you what happened; this gives you something actionable to carry forward. The distinction is output format: optimized for LLM consumption, not human browsing. Every developer who uses Claude Code across multiple sessions hits this pain point.

**Design:**
- Output to stdout (stays read-only)
- Format: Markdown summary optimized for pasting into a new Claude Code session
- Includes: files modified (with op counts), agents spawned and their outcomes, final prompt's context%, branch at end of session
- Deterministic: every field is an exact value from the JSONL. No summarization, no content analysis, no "key decisions" — just structured facts. The LLM receiving the handoff does the synthesis.

**Evidence of demand:**
- [DEV: "Claude Code Lost My 4-Hour Session. Here's the $0 Fix"](https://dev.to/gonewx/claude-code-lost-my-4-hour-session-heres-the-0-fix-that-actually-works-24h6)
- [Towards AI: "The Forgetting Problem — Engineering Persistent Intelligence in Claude Code"](https://pub.towardsai.net/the-forgetting-problem-engineering-persistent-intelligence-in-claude-code-bd2e4c59711a)

---

### Token Burn Rate / Velocity

Show **burn rate** — tokens/minute over the session, spiking when agents are running, flatlining when stuck. A sudden spike means agents just spawned. A sustained plateau means the agent is churning.

**Why P1:** This is the "control room" metaphor fully realized — like watching CPU utilization, not just total CPU-seconds. Rate of change is more informative than cumulative totals for live monitoring. Natural companion to loop detection — high burn rate + repetition = almost certainly stuck. Fully deterministic: exact token counts divided by exact timestamp intervals.

**Implementation notes:**
- Derive from `input_tokens` + `output_tokens` on assistant messages, bucketed by time
- Display as sparkline in stats bar or as overlay on context pressure sparkline (M8)
- Per-agent burn rate in swimlane gives immediate signal: "this agent is burning 10x the tokens of the others"

---

## Priority 2 — Clear demand, distinct kno-trace angle, moderate effort or mild determinism caveats

Real pain points where kno-trace's architecture gives it an advantage. May involve external data, editorial choices, or pattern matching that introduces some fragility.

### Prompt-Level Git Commit Tracking

Show which git commits were created during each prompt by detecting `git commit` in Bash tool calls and extracting commit hashes from the output. Bridges the gap between "what Claude did" and "what ended up in version control."

**Why useful:** No tool connects Claude Code's prompt-level activity to the git history. Developers currently have to cross-reference `git log` timestamps with their memory of which prompt did what. This makes the connection explicit: "Prompt #7 created commits `a3f2b1c` and `e8d4f6a`."

**Determinism caveat:** Pattern-matching Bash output for `git commit` is fragile. Commits via aliases (`gc`), scripts, MCP tools, or non-standard wrappers won't be detected. The feature only captures what it can see — but it can't declare "no commits happened" with certainty, only "these commits were observed." Must be framed as "observed commits" not "all commits."

**Design:**
- Parse Bash tool call commands for `git commit` patterns
- Extract commit hash from Bash output (first line of `git commit` output contains the short hash)
- Display commit badges on prompts in the timeline: `#7 [14:32-14:35] ✦ a3f2b1c`
- Expanded view shows full commit message
- No heuristics — only explicit `git commit` Bash calls with successful output (exit code 0)

**Implementation notes:**
- Bash output is already captured (truncated to 500 chars, but commit hash is in the first line)
- Could miss commits made via MCP tools or non-standard git wrappers — document this limitation
- No `git log` calls from kno-trace itself — purely derived from JSONL data

---

### Cross-Session File History

Answer "what happened to this file across the last N sessions?" by stitching file histories from multiple sessions for the same project.

**Why useful:** Developers doing multi-session refactors currently have no way to see the full arc of changes to a file. The heatmap (M6) answers this within a single session; cross-session extends the lens. This is the natural evolution of UC2.

**Design:**
- Project-grouped session list (session picker already groups by project)
- File history view gains a "cross-session" toggle: shows file operations across all sessions for the current project
- Timeline: `Session A #3: Write → Session A #7: Edit → Session B #2: Edit → Session B #5: Write`
- Diffs between session-boundary states (file at end of session A vs. start of session B)

**Implementation notes:**
- Requires parsing multiple session files — lazy loading with metadata scan first
- Memory bounded: only load FileHistory for the selected file, not full session parse
- Could be expensive for projects with dozens of sessions — cap at configurable `max_sessions_cross_ref` (default 10)

---

### Cost Tracking

Per-prompt and per-session cost estimation based on configurable model pricing. Token counts (which v1 already shows) are concrete and verifiable; cost is a pure derivation on top.

**Determinism caveat:** Pricing is external, non-deterministic data. It varies by plan (API vs. Max vs. Pro vs. Team), changes over time, has different cache read/write tiers, and kno-trace cannot know the user's plan. Any number shown could be wrong. This directly conflicts with the "no guesses, no approximations" principle — a displayed cost IS an approximation unless the user configures their exact rates.

**Design (if pursued):**
- Add `model_pricing` config section with per-model input/output/cache rates — NO defaults baked in. User must configure pricing explicitly, or cost is not shown.
- Alternatively: show only raw token counts (already done in v1) and let external tools do cost calculation. kno-trace's `--dump` or `--manifest` output provides the data; the user pipes it through their own pricing calculator.
- This "provide data, not interpretation" approach is more aligned with kno-trace's principles

**Evidence of demand:**
- [ksred: "I Built a Cost Tracker — Was Max Worth It?"](https://www.ksred.com/i-built-a-cost-tracker-for-claude-code-to-see-if-my-subscription-was-worth-it/)
- claudetop (popular tool) exists primarily for cost visibility
- Every "is Claude Code worth it?" thread on Reddit/HN asks about per-session costs

---

### Compliance Zones — Real-Time Sensitive File Awareness

User-defined path patterns mapped to compliance labels (PCI-DSS, HIPAA, Auth, etc.). When the agent touches a file in a zone, it's flagged in the timeline, heatmap, and ticker in real-time. This is **awareness, not enforcement** — kno-trace doesn't block the agent, it tells you what happened.

**Why unique:** Existing tools either block before it happens (Knostic, Claude Code hooks/permissions) or scan after commit (Snyk, GitGuardian, Semgrep). Nothing provides real-time, in-session awareness that an agent just touched compliance-relevant code.

**Evidence of demand:**
- NIST published a concept paper (Feb 2026) on AI agent identity and authorization
- Knostic (funded startup) built an entire platform around AI coding assistant governance — validates the market but focuses on prevention, not observability
- A book ("Who Approved This Agent?") was written on authorizing AI-generated code
- SOC2 Type II, HIPAA, and PCI-DSS require documented evidence of security controls — AI-generated code with no audit trail fails these requirements
- No existing Claude Code observability tool (claude-esp, agents-observe, claude-devtools, etc.) addresses compliance awareness

**Design — phased approach:**

*Phase 1: Config-driven zones*
```yaml
zones:
  - name: "PCI-DSS"
    paths: ["internal/payments/**", "**/billing/**"]
    badge: "PCI"
  - name: "Auth"
    paths: ["**/auth/**", "**/middleware/auth*"]
    badge: "AUTH"
  - name: "Database Migrations"
    paths: ["migrations/**", "db/migrate/**"]
    badge: "MIGRATE"
```
- Badge appears on tool calls in the timeline and ticker when a matched file is touched
- Heatmap view highlights zone-flagged files distinctly
- Zero dependencies, fits existing config model

*Phase 2: Shared zone definitions*
- `.kno-trace-zones.yaml` checked into the repo — teams share compliance zone definitions
- kno-trace reads from repo root if present, merges with user config (user config wins on conflict)
- Still just config, not plugins

*Phase 3: Custom zone evaluators (plugin layer)*
- Shell commands or scripts that receive file paths on stdin and return labels on stdout
- No SDK, no API surface — just stdin/stdout convention
- Teams can plug in internal classification tools, policy engines, or custom logic

**Implementation notes:**
- Zone matching is glob-based (`filepath.Match` or `doublestar` library for `**` support)
- Zone badges are a new field on `ToolCall` in the model — `Zones []string`
- Matching runs at parse time when file paths are extracted from tool calls
- Cross-cutting: touches parser, timeline, heatmap, and ticker — better as a focused post-v1 addition than scattered across milestones

---

### Session Change Manifest (`--manifest`)

On-demand structured summary of everything that happened in a session: every file touched, every tool used, every agent spawned, with compliance zone annotations. This is the **audit artifact** that SOC2/PCI/HIPAA auditors want.

**Why unique:** Existing log viewers show session content for humans to read. Nobody produces a structured, machine-readable audit artifact. This is the bridge to enterprise compliance tooling.

**Design:**
- `kno-trace --manifest <session>` writes JSON to stdout (consistent with read-only principle — no files written)
- Includes: session metadata, prompt count, files modified (with zone labels), agents spawned, tools used, token usage
- Optionally filterable: `--manifest --zone PCI` to show only PCI-relevant changes
- Machine-readable, pipeable — can feed into existing compliance tooling, SIEM, or approval workflows
- Subsumes `--dump --json` (see Priority 4)

**Implementation notes:**
- Pure derivation from the parsed `Session` model — no new data collection
- Zone annotations come from the Compliance Zones feature
- Consider a `--manifest --summary` mode for human-readable output (Markdown table)
- Best shipped after Compliance Zones — zones make the manifest dramatically more useful

---

## Priority 3 — Useful, low effort, narrower audience

Good ideas that round out the product. Lower priority because they're either partially addressed by other tools or serve fewer users daily.

### Session Health Score

A single glanceable composite indicator: time since last tool call (stalled?), repetition rate (spinning?), context% (pressure?), agent count (overwhelmed?). Green/yellow/red in the stats bar.

**Determinism caveat:** The individual metrics are exact, but combining them into a single score requires weighting — which is inherently opinionated. What ratio of context%/idle/repetition = "yellow"? Any composite threshold is a judgment call, not a derivation. Could be mitigated by making the score purely config-driven (user defines their own thresholds) and showing component metrics alongside the composite.

**Implementation notes:**
- Pure derivation from metrics already computed for other features (loop detection, context%, quiet state)
- Thresholds configurable in config
- Degrades gracefully: if any input metric is unavailable, omit it from the composite

---

### Read-Heavy vs. Write-Heavy Prompt Classification

Tag each prompt with its read/write ratio based on tool calls. A prompt that's 90% Reads and Greps is exploration. One that's 80% Writes and Edits is implementation. No content heuristics — pure tool call counting.

**Why not promoted to v1:** The existing per-prompt badges (write/read/edit/bash counts) already tell this story if the user reads them. The ratio is a convenience, not new information. Could be added as a subtle color shift on existing badges.

**Implementation notes:**
- Already have tool call types per prompt
- Display as a small icon or color-coded bar in the timeline list
- Useful for understanding session flow at a glance: explore → implement → explore → fix

---

### Session Annotations / Bookmarks

Allow users to bookmark specific prompts or add notes during a session for later reference.

**Design — resolving the read-only tension:**
- Annotations stored in kno-trace's own config directory: `~/.config/kno-trace/annotations/<session-id>.yaml`
- This respects the inviolable rule ("never write to the user's filesystem outside kno-trace's own config directory") while adding persistence
- `b` to bookmark current prompt, `B` to list bookmarks, named bookmarks via text input
- Bookmarks visible as badges in the timeline: `★ "started refactor"`
- Survives kno-trace restart (unlike in-memory marks from M7's `m` workflow)

**Implementation notes:**
- Small YAML file per session — bounded by prompt count
- Distinct from M7's `m` marks which are ephemeral and purpose-built for diff selection

---

### File Content Preview

Preview full file content at any point in the session (not just diffs). Useful for understanding what a file looked like at prompt N.

**Why deferred:** `replay.Engine.GetContentAt()` already supports this. The missing piece is a syntax-highlighted file viewer in the TUI, which is a non-trivial component.

---

### Export / Share

Export a session summary, diff, or timeline as HTML, Markdown, or image for sharing.

**Design directions:**
- `--export-html <session>` → self-contained HTML file with embedded CSS, viewable in any browser
- `--export-md <session>` → Markdown summary for pasting into PRs, docs, or Slack
- Useful for code review ("here's what Claude did"), incident postmortems, or team knowledge sharing
- Shares infrastructure with `--handoff` (P1) and `--manifest` (P2) — different output formats of the same parsed data

**Why deferred:** Pure feature scope. No architectural blocker.

---

### Session Analytics — Patterns Across Sessions

Factual aggregates across sessions for a project: average session duration, most-edited files, typical context% at compaction, token usage trends. Not heuristic classification — observable metrics over time.

**Design:**
- `kno-trace --stats [project-path]` — CLI output, no TUI required
- Metrics: session count, avg/median duration, total tokens, most-modified files (top 10), compaction frequency, agent spawn rate
- Data derived from lightweight meta-scan of session files (same as session picker), plus selective full parse for token/file data
- Helps developers tune their workflow: "my sessions average 45 minutes and hit compaction at prompt 12"

**Implementation notes:**
- Reuses `SessionMeta` scanning from discovery package
- Full parse only for token/file metrics — expensive, so optional (`--stats --full`)
- No persistence — computed on demand each time

---

## Priority 4 — Speculative or constrained

Ideas with real use cases but significant design constraints, open questions, or tension with core principles.

### Prompt×File Matrix View

A grid visualization: columns = prompts, rows = files, cells colored by operation type (Write/Read/Edit/Bash). At-a-glance map of which prompts touched which files.

**Why deprioritized:** Requires lazy bi-directional scrolling, cell rendering at scale, and navigation integration — substantial UI complexity. The heatmap view + timeline search already cover most workflows.

**Implementation notes:**
- Would live in `internal/ui/matrix.go`, sub-tab in heatmap view
- Cells: orange=Write, blue=Read, yellow=Edit, dim=Bash, empty=none
- `enter` on a cell navigates to that prompt in timeline view
- Lazy rendering — only render visible cells

---

### Cross-Session Comparison

Compare file states, token usage, or agent patterns across multiple sessions for the same project.

**Why deprioritized:** Data model already stores `ProjectPath` on sessions, so grouping is possible. But the UI for selecting and comparing sessions is a significant design surface. Narrow audience until kno-trace has enough adoption for users to have many sessions worth comparing.

---

### Bash Command Risk Classification

Classify Bash commands by risk level (read-only, mutation, destructive) and surface warnings.

**Why deprioritized:** Any useful classification is heuristic — commands can be aliased, piped, or wrapped in scripts. Showing commands as-is is honest; classifying them risks false confidence.

---

### Agent Retry Detection

Detect when a subagent was spawned with a similar task description to a previous agent (likely a retry after failure).

**Why deprioritized:** "Similar" is inherently heuristic. Would need fuzzy text comparison with a tuned threshold. Loop detection (now in M2/M4) covers the observable symptom without requiring content comparison.

---

### `--update-pricing` / Remote Pricing Fetch

Fetch current Anthropic pricing and update local config automatically.

**Why deprioritized:** Requires network access, which violates the zero-network principle. Only relevant if cost tracking is implemented. Would need to be explicit, user-initiated, with clear consent.

---

### `--dump --json` Machine-Readable Output

Structured JSON output of a parsed session to stdout.

**Why deprioritized:** Subsumed by `--manifest` (Priority 2). If `--manifest` ships first, this becomes redundant. If `--manifest` is scoped differently, this remains a simpler alternative for scripting.

**Implementation notes:**
- Emits full `Session` struct as JSON
- Consider `--dump --json --compact` for minimal output (no file content snapshots)
- The human-readable `--dump` format may change between versions without notice
