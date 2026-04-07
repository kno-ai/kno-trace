// Package model defines all domain types for kno-trace.
// This is the single source of truth — all other packages import from here.
package model

import "time"

// Session represents one Claude Code session file.
type Session struct {
	ID          string
	FilePath    string    // absolute path to .jsonl file
	ProjectPath string    // inferred from cwd field in JSONL messages
	ProjectName string    // last component of ProjectPath, for display
	ModelName   string    // primary model, from first assistant message; individual prompts may differ
	StartTime   time.Time
	EndTime     time.Time    // zero if session still live (no final timestamp seen)
	Prompts        []*Prompt
	UnlinkedAgents []*AgentNode // agents whose session ID linkage could not be resolved
	IsLive         bool         // set by watcher: true only while actively receiving new lines
	CompactAt      []int        // prompt indices where /compact occurred (may be multiple)
	Interrupted    bool         // true if session ended without a final assistant turn
}

// Prompt represents one human turn and everything Claude did in response.
// Bounded by: start = this human message, end = next human message (or EOF).
type Prompt struct {
	Index        int
	HumanText    string
	ResponseText string // assistant's text response (from last assistant message text blocks)
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
	// 0 means token data was not available — do not display context% in that case.
	ContextPct        int
	Interrupted       bool              // true if this was the last prompt and session was cut short
	BranchTransition  BranchTransition  // non-zero when gitBranch changed during this prompt
	IsDurationOutlier bool              // true if duration >2σ above mean (only computed with ≥5 prompts)
}

// BranchTransition records a git branch change observed during a prompt.
// Zero value means no transition occurred.
type BranchTransition struct {
	From string
	To   string
}

// ToolCall represents a single tool invocation by any actor (parent or agent).
type ToolCall struct {
	ID          string
	SourceAgent string // ID of the agent that made this call; empty = parent session
	Type        ToolType
	Timestamp   time.Time

	// File operations (Write, Read, Edit)
	Path    string
	Content string // Write: full new content; Read: content from tool_result
	OldStr  string // Edit
	NewStr  string // Edit

	// Bash
	Command  string
	ExitCode int    // from tool_result; -1 if not available
	Output   string // truncated to 500 chars

	// Agent (subagent spawn) — only populated when Type == ToolAgent
	SpawnedAgentID   string
	AgentDescription string
	AgentPrompt      string
	SubagentType     string

	// MCP / other
	MCPToolName   string
	MCPServerName string
	MCPInput      map[string]any

	// Derived — set by classifier
	IsCLAUDEMD bool // true if path matches CLAUDE.md memory file patterns
}

// ToolType classifies a tool invocation.
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
type AgentNode struct {
	ID                string
	ToolUseID         string       // the tool_use block ID that spawned this agent — used for result matching
	SessionID         string       // conversation ID used by this agent in the log
	Label             string       // generated: "subagent-1", "subagent-2"
	ModelName         string       // model used by this agent (from its assistant messages)
	SubagentType      string       // e.g., "Explore", "Plan"
	TaskDescription   string
	TaskPrompt        string
	ParentPromptIdx   int
	ParentAgentID     string       // empty if direct child of prompt
	ToolCalls         []*ToolCall
	Children          []*AgentNode // nested agents
	FilesTouched      []string     // unique file paths from this agent's tool calls, deduped
	StartTime         time.Time
	EndTime           time.Time
	Duration          time.Duration // EndTime - StartTime; zero if still running
	TokensIn          int           // sum of input_tokens across this agent's assistant messages
	TokensOut         int           // sum of output_tokens across this agent's assistant messages
	TotalDurationMs   int           // from toolUseResult — authoritative
	TotalTokens       int           // from toolUseResult — authoritative
	TotalToolUseCount int           // from toolUseResult — authoritative
	Status            AgentStatus
	IsParallel        bool // true if ran concurrently with a sibling agent
}

// AgentStatus is determined exactly from the JSONL — not heuristic.
type AgentStatus string

const (
	AgentRunning   AgentStatus = "running"
	AgentSucceeded AgentStatus = "succeeded"
	AgentFailed    AgentStatus = "failed"
)

// Warning flags a factual observation about a prompt.
type Warning struct {
	Type    WarnType
	Message string
}

// WarnType identifies a specific warning condition.
type WarnType string

const (
	WarnContextHigh     WarnType = "context_high"
	WarnContextCritical WarnType = "context_critical"
	WarnMCPExternal     WarnType = "mcp_external"
	WarnInterrupted     WarnType = "interrupted"
	WarnAgentConflict   WarnType = "agent_conflict"
	WarnReplayGap       WarnType = "replay_gap"
	WarnAgentUnlinked   WarnType = "agent_unlinked"
	WarnLoopDetected    WarnType = "loop_detected"   // same tool+path repeated ≥threshold times
)

// FileHistory tracks all interactions with a file across the session.
type FileHistory struct {
	Path           string
	PromptIdxs     []int    // prompt indices that touched this file, in order
	Ops            []string // op type per entry: "W","R","E","bash"
	SourceAgents   []string // agent label per entry: "" = parent, "subagent-1" = agent
	HeatScore      int      // count of distinct prompts with Write or Edit ops
	HasBaseline    bool     // true if a pre-session Read baseline is available
	WriteSnapshots map[int]string // full content after each Write op, capped per config
	ReadBaseline   string         // content from first Read op on this file
}

// SessionMeta is lightweight — used by the session picker without full parse.
// All fields are exact values derived from the filesystem and first/last JSONL lines.
type SessionMeta struct {
	ID            string
	FilePath      string
	ProjectDir    string        // encoded directory name under ~/.claude/projects/
	ProjectPath   string        // real project path, from cwd field in JSONL
	ProjectName   string        // last component of ProjectPath, for display
	StartTime     time.Time     // earliest timestamp found in first ~10 lines
	EndTime       time.Time     // latest timestamp found in last ~10 lines
	FileSizeBytes int64         // exact, from os.Stat
	Duration      time.Duration // EndTime - StartTime
}
