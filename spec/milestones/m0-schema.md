# M0: Schema Investigation

**Prerequisites:** Read [spec/README.md](../README.md) (core spec)

**Goal:** Ground truth before any code is written.

**Deliverable:** `SCHEMA.md` and fixture JSONL files. No Go code written.

**Before starting:** Create a new empty directory named `kno-trace` and run all work from within it. This is the project root for all milestones.

---

## Scope

### Step 1 — Explore real JSONL files

List `~/.claude/projects/` and identify sessions covering as many of these as available:
- A simple session (no agents, Write/Edit/Read/Bash)
- A session with at least one subagent (look for `"name":"Task"` in tool_use blocks)
- A session with parallel subagents (multiple Task calls before any tool_result returns)
- A session where `/compact` was used
- A session with MCP tool calls (tool_use names not in the known list)

For each, confirm exact field names for:
- The field distinguishing human turns from tool results (both have `role: "user"`)
- Session/conversation ID field name
- The field linking subagent messages to parent Task tool_use
- Token usage field names on assistant messages (including cache_read_input_tokens, cache_creation_input_tokens, or equivalent)
- Model name field
- Any non-message metadata lines that appear in the JSONL
- How `/compact` appears in the JSONL (or note "not observed")
- Any additional per-message metadata fields that may be useful (stop reason, etc.)

Confirm the hash algorithm: given a known project path, verify the hash matches the directory name under `~/.claude/projects/`. Also check whether each `~/.claude/projects/<hash>/` directory contains a metadata file that records the original project path.

### Step 2 — Write SCHEMA.md

Create `SCHEMA.md` in the project root using exactly this structure:

```markdown
# kno-trace — Confirmed JSONL Schema

## Confirmed Field Names

### Human turn identification
- Field path: message.content[0].type
- Value that identifies a human turn (not tool result): [confirmed value]
- Value that identifies a tool result: [confirmed value]

### Session / conversation ID
- Field name: [exact field name]
- Example value structure: [e.g., "uuid string" or "nested object"]

### Subagent parent linkage
- Field name on child message that links to parent Task tool_use: [exact field name]
- Field name on parent Task tool_use that child references: [exact field name]
- Which message type carries this field: [e.g., "appears on user-turn messages from subagent"]
- Example: [annotated JSON snippet showing parent Task and child linkage]

### Token usage
- Input tokens field: [exact path, e.g., usage.input_tokens]
- Output tokens field: [exact path, e.g., usage.output_tokens]
- Cache read tokens field: [exact path, or "not present"]
- Cache creation tokens field: [exact path, or "not present"]
- Which message type carries this field: [e.g., "assistant messages only"]

### Model name
- Field name: [exact field name]
- Example value: [e.g., "claude-sonnet-4-5"]

### Non-message metadata lines
- [list any JSONL lines that are not user/assistant messages, with their structure]
- [or "none observed"]

### /compact representation
- [exact JSON structure of the compact message]
- [or "not observed in available sessions"]

## Hash Algorithm
- Algorithm: [e.g., "SHA256 of absolute project path, lowercase hex, full length"]
- Metadata file present at hash directory: [yes/no ��� if yes, filename and structure]
- Confirmed: project path [X] produces hash [Y] matching directory [Z]

## Schema Discrepancies vs SPEC.md
- [list any field names that differ from what SPEC.md assumed]
- [or "none — spec matches observed schema"]
```

### Step 3 — Create fixture JSONL files

Create `internal/testdata/` directory and populate it with fixtures from real sessions.

**Anonymization rules — apply to every fixture:**
- Human turn text: replace with a short bracketed description, e.g. `"[implement the parser]"`
- File paths in tool inputs: replace with realistic Go-style paths, e.g. `"internal/foo/bar.go"`
- Write tool `content` field: replace with `"// content redacted\n"`
- Read tool result content: replace with `"// content redacted\n"`
- Bash commands: keep read-only commands as-is (e.g. `go build ./...`, `go test ./...`); replace any command containing real credentials, tokens, or absolute user paths with `"[redacted command]"`
- UUIDs and session IDs: replace with sequential placeholders — `"uuid-001"`, `"uuid-002"`, `"session-001"`, `"toolu-001"`, etc. — preserving the structural relationship between linked IDs
- Timestamps: keep real timestamps (no PII)
- Tool names, JSON keys, and JSON structure: never modify — the structure is the point of the fixture

**Size limit:** Extract a maximum of 5 prompts per fixture. Use the earliest prompts that demonstrate the required characteristic.

**Required fixtures:**
- `simple.jsonl` — 3 prompts, no agents; must include at least one Write, one Edit, one Read, one Bash
- `interrupted.jsonl` — any session where the last human turn has no following assistant turn

**Conditional fixtures — create only if a real session exists; otherwise note absence in SCHEMA.md:**
- `with_agent.jsonl` — 2 prompts, one containing a Task tool call with a subagent
- `parallel_agents.jsonl` — 1 prompt containing 2 parallel Task calls that touch the same file path
- `mcp_calls.jsonl` — any session containing at least one MCP tool call
- `with_compact.jsonl` — any session where `/compact` was used

Do not create synthetic fixtures. If a required fixture type (`simple.jsonl`, `interrupted.jsonl`) cannot be sourced from real sessions, stop and report the problem — do not invent JSONL.

---

## After completing M0 — STOP.

Do not proceed to M1. Return control to the developer. The developer will:
1. Review `SCHEMA.md` and compare against this spec
2. Update the data model or parser notes in this spec if any field names differ
3. Explicitly initiate M1 in a new session

---

## Acceptance Criteria

- `SCHEMA.md` exists with all sections populated per the template above
- `simple.jsonl` and `interrupted.jsonl` exist in `internal/testdata/`
- All conditional fixtures that were available are present; unavailable ones are noted in SCHEMA.md
- Hash algorithm documented and confirmed with a real example
- Subagent linkage mechanism documented with a real JSON example (if subagent session available)
- Schema discrepancies section completed (even if "none")
