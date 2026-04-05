# kno-trace — Deferred Ideas & Future Features

Features considered for v1 but deferred to keep scope manageable. Each can be added without architectural changes — the data model and parser already support them.

---

## Prompt×File Matrix View

A grid visualization: columns = prompts, rows = files, cells colored by operation type (Write/Read/Edit/Bash). Provides an at-a-glance map of which prompts touched which files.

**Why deferred:** Requires lazy bi-directional scrolling, cell rendering at scale, and navigation integration — substantial UI complexity. The heatmap view + timeline search (`/`) already answer "which files were hot?" and "which prompts touched file X?" for most workflows.

**Implementation notes:**
- Would live in `internal/ui/matrix.go`, accessible as a sub-tab in the heatmap view (`tab` to switch)
- Cells: orange=Write, blue=Read, yellow=Edit, dim=Bash, empty=none
- `enter` on a cell navigates to that prompt in timeline view
- Scrollable both axes with position indicators
- Lazy rendering — only render visible cells

---

## Cross-Session Comparison

Compare file states, token usage, or agent patterns across multiple sessions for the same project. Useful for understanding how a codebase evolved over multiple Claude Code interactions.

**Why deferred:** Data model already stores `ProjectPath` on sessions, so grouping is possible. But the UI for selecting and comparing sessions is a significant design surface.

---

## Bash Command Risk Classification

Classify Bash commands by risk level (read-only, mutation, destructive) and surface warnings on potentially dangerous operations.

**Why deferred:** Any useful classification is heuristic — commands can be aliased, piped, or wrapped in scripts. Showing commands as-is is honest; classifying them risks false confidence. Could revisit with a well-tested heuristic set.

---

## Agent Retry Detection

Detect when a subagent was spawned with a similar task description to a previous agent (likely a retry after failure).

**Why deferred:** "Similar" is inherently heuristic. Would need fuzzy text comparison with a tuned threshold. Not worth the false positive risk for v1.

---

## `--dump --json` Machine-Readable Output

Add a `--dump --json` flag that emits the parsed session as structured JSON to stdout. The human-readable `--dump` format remains the default and is explicitly unstable (not a scripting interface).

**Why deferred:** JSON output is straightforward but adds a compatibility commitment — once scripts depend on the shape, it's hard to change. Worth doing once the data model is stable post-v1.

**Implementation notes:**
- `--dump --json` emits the full `Session` struct as JSON (prompts, tool calls, agents, file histories)
- Consider `--dump --json --compact` for minimal output (no file content snapshots)
- The human-readable `--dump` format may change between versions without notice

---

## `--update-pricing` / Remote Pricing Fetch

If cost tracking is added, a command to fetch current Anthropic pricing and update the local config automatically.

**Why deferred:** Requires network access, which violates the zero-network principle. Would need to be an explicit, user-initiated command with clear consent. Not needed unless cost tracking is implemented.

---

## Session Annotations / Bookmarks

Allow users to bookmark specific prompts or add notes to a session for later reference. Would require breaking the read-only principle (writing a sidecar file).

**Why deferred:** Conflicts with the core "read-only, no sidecar" design principle. Could be implemented as a separate annotation file that kno-trace reads but doesn't require.

---

## Cost Tracking

Per-prompt and per-session cost estimation based on model pricing. Would require a configurable rate table (model → price per million tokens) and display of cost in timeline, stats bar, and a potential cost dashboard view.

**Why deferred:** Pricing data is complex — it varies by model, changes over time, and includes cache read/write tiers. A bug in cost calculation could be disheartening for users. Token counts (which v1 already shows) are concrete and verifiable. Cost tracking is better served by dedicated tools. If added later, token data is already in the model — cost is a pure derivation.

**Implementation notes:**
- Add `CostUSD float64` to `Prompt` model
- Add `model_pricing` config section with per-model input/output/cache rates
- Display in timeline header, stats bar, `--dump` output
- Aggregate views: cost by model, by agent vs parent, over time

---

## File Content Preview

Preview full file content at any point in the session (not just diffs). Useful for understanding what a file looked like at prompt N.

**Why deferred:** `replay.Engine.GetContentAt()` already supports this. The missing piece is a syntax-highlighted file viewer in the TUI, which is a non-trivial component.

---

## Export / Share

Export a session summary, diff, or timeline as HTML, Markdown, or image for sharing.

**Why deferred:** Pure feature scope. No architectural blocker.
