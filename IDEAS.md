# kno-trace — Deferred Ideas & Future Features

Ideas for post-v1, organized by priority. Priority reflects: strength of user demand signal, uniqueness vs. existing tools, and alignment with kno-trace's "control room" vision.

**Guiding principle:** The strongest ideas make the invisible visible without adding opinion. kno-trace shows facts the JSONL already contains but that nobody can see today.

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

---

## Priority 1 — High demand, unique to kno-trace, first post-v1 targets

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
- Example: pipe paths through an internal tool that checks a service registry for data classification

**Implementation notes:**
- Zone matching is glob-based (`filepath.Match` or `doublestar` library for `**` support)
- Zone badges are a new field on `ToolCall` in the model — `Zones []string`
- Matching runs at parse time when file paths are extracted from tool calls
- No heuristics — pure path matching against user-defined patterns
- Cross-cutting: touches parser, timeline, heatmap, and ticker — better as a focused post-v1 addition than scattered across milestones

---

### Session Change Manifest (`--manifest`)

On-demand structured summary of everything that happened in a session: every file touched, every tool used, every agent spawned, with compliance zone annotations. This is the **audit artifact** that SOC2/PCI/HIPAA auditors want.

Turns kno-trace from a developer convenience into something a compliance officer cares about. The manifest is the answer to "what did the AI do to our codebase?" in a format that can be attached to a change request, audit log, or approval workflow.

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

## Priority 2 — Clear demand, unique kno-trace angle

Real pain points where some competition exists, but kno-trace's architecture gives it a distinct advantage.

### Session Handoff Summary (`--handoff`)

People lose 4-hour sessions and can't recover context. Context loss between sessions is a top complaint. `kno-trace --handoff <session>` generates a structured summary designed to be pasted into a *new* Claude Code session: files modified, key decisions, where it left off.

**Why unique:** This is not a log viewer — it's a **context restoration artifact** for developers. Existing tools show you what happened; this gives you something actionable to carry forward. The distinction is output format: optimized for LLM consumption, not human browsing.

**Design:**
- Output to stdout (stays read-only)
- Format: Markdown summary optimized for pasting into a new Claude Code session
- Includes: files modified with brief description of changes, agents spawned and their outcomes, final state summary
- Not a full transcript — a curated, compressed handoff

**Evidence of demand:**
- [DEV: "Claude Code Lost My 4-Hour Session. Here's the $0 Fix"](https://dev.to/gonewx/claude-code-lost-my-4-hour-session-heres-the-0-fix-that-actually-works-24h6)
- [Towards AI: "The Forgetting Problem — Engineering Persistent Intelligence in Claude Code"](https://pub.towardsai.net/the-forgetting-problem-engineering-persistent-intelligence-in-claude-code-bd2e4c59711a)

---

### Token Burn Rate / Velocity

claudetop shows token counts as a point-in-time number. kno-trace could show **burn rate** — tokens/minute over the session, spiking when agents are running, flatlining when stuck. A sudden spike means agents just spawned. A sustained plateau means the agent is churning.

**Why unique:** This is the "control room" metaphor realized — like watching CPU utilization, not just total CPU-seconds. Rate of change is more informative than cumulative totals for live monitoring.

**Implementation notes:**
- Derive from `input_tokens` + `output_tokens` on assistant messages, bucketed by time
- Display as sparkline in stats bar or as overlay on context pressure sparkline (M8)
- Natural companion to loop detection — high burn rate + repetition = almost certainly stuck

---

### Compaction Diff — What Got Forgotten

When `/compact` runs, context is summarized and detail is lost. The JSONL has `compact_boundary` with `preTokens`. kno-trace could show **what prompts existed before vs. after the compaction boundary** — helping the user understand what the agent "forgot."

**Why unique:** Nobody visualizes this. Users know compaction happened but have no way to understand its impact without reading raw JSONL. This makes the lossy compression visible.

**Implementation notes:**
- Mark prompts as pre/post compaction in the timeline
- Show a visual divider at compaction points
- Optional: dim or collapse pre-compaction prompts to indicate they're "outside active memory"

---

### "What Did I Approve?" — Permission Decision Log

Claude Code prompts for permissions and people click through them fast. kno-trace could surface a clean log: "You approved Write to `auth/middleware.go` at 14:32" — reconstructed from tool_use/tool_result pairs.

**Why unique:** Useful for post-session review ("wait, did I let it edit that?"). Especially powerful paired with compliance zones — "you approved 3 writes to PCI-scoped files."

**Implementation notes:**
- Reconstruct from tool_use (request) → tool_result (approved, since result exists) pairs
- Filter view: show only write/edit/bash approvals (reads are less interesting)
- Could be a filter mode in the timeline rather than a separate view

---

## Priority 3 — Useful, low effort, less unique

Good ideas that round out the product. Lower priority because they're either partially addressed by other tools or have narrower audiences.

### Session Health Score

A single glanceable composite indicator: time since last tool call (stalled?), repetition rate (spinning?), context% (pressure?), agent count (overwhelmed?). Not a heuristic classification — a composite of observable metrics. Green/yellow/red in the stats bar.

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

### Cost Tracking

Per-prompt and per-session cost estimation based on configurable model pricing. Token counts (which v1 already shows) are concrete and verifiable; cost is a pure derivation on top.

**Why deferred:** Pricing data is complex — varies by model, changes over time, includes cache read/write tiers. A bug in cost calculation could be misleading. Better served by dedicated tools like claudetop. If added, token data is already in the model.

**Implementation notes:**
- Add `CostUSD float64` to `Prompt` model
- Add `model_pricing` config section with per-model input/output/cache rates
- Display in timeline header, stats bar, `--dump` output
- Aggregate views: cost by model, by agent vs parent, over time

---

### File Content Preview

Preview full file content at any point in the session (not just diffs). Useful for understanding what a file looked like at prompt N.

**Why deferred:** `replay.Engine.GetContentAt()` already supports this. The missing piece is a syntax-highlighted file viewer in the TUI, which is a non-trivial component.

---

### Export / Share

Export a session summary, diff, or timeline as HTML, Markdown, or image for sharing.

**Why deferred:** Pure feature scope. No architectural blocker.

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

### Subagent File Reading + Live Tailing → M5 (promoted)

**Promoted to v1.** M5 now reads subagent JSONL files for full tool call history AND tails them in real-time during live sessions. Without this, the swimlane would show empty lanes for agents that don't emit progress lines — which undermines the flagship view. M6's replay engine includes agent tool calls in its file history (interleaved by timestamp), so the heatmap and diff view reflect agent modifications too. See M5 and M6 specs for details.

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

### Session Annotations / Bookmarks

Allow users to bookmark specific prompts or add notes to a session for later reference.

**Why deprioritized:** Requires breaking the read-only principle (writing a sidecar file). Could be implemented as a separate annotation file that kno-trace reads but doesn't require. Tension with core design.

---

### `--update-pricing` / Remote Pricing Fetch

Fetch current Anthropic pricing and update local config automatically.

**Why deprioritized:** Requires network access, which violates the zero-network principle. Only relevant if cost tracking is implemented. Would need to be explicit, user-initiated, with clear consent.

---

### `--dump --json` Machine-Readable Output

Structured JSON output of a parsed session to stdout.

**Why deprioritized:** Subsumed by `--manifest` (Priority 1). If `--manifest` ships first, this becomes redundant. If `--manifest` is scoped differently, this remains a simpler alternative for scripting.

**Implementation notes:**
- Emits full `Session` struct as JSON
- Consider `--dump --json --compact` for minimal output (no file content snapshots)
- The human-readable `--dump` format may change between versions without notice
