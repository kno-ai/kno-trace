package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kno-ai/kno-trace/internal/config"
	"github.com/kno-ai/kno-trace/internal/discovery"
	"github.com/kno-ai/kno-trace/internal/model"
	"github.com/kno-ai/kno-trace/internal/parser"
	"github.com/kno-ai/kno-trace/internal/ui"
)

var version = "dev"

func main() {
	pick := flag.Bool("pick", false, "Always open session picker")
	sessionPath := flag.String("session", "", "Open a specific JSONL session file")
	dumpPath := flag.String("dump", "", "Parse session and print structured summary to stdout")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("kno-trace %s\n", version)
		os.Exit(0)
	}

	cfg := config.Load()

	// --dump: parse and print, no TUI.
	if *dumpPath != "" {
		absPath, err := filepath.Abs(*dumpPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if err := runDump(absPath, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// --session: open a specific file directly.
	if *sessionPath != "" {
		absPath, err := filepath.Abs(*sessionPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "error: file not found: %s\n", absPath)
			os.Exit(1)
		}

		projectDir := filepath.Base(filepath.Dir(absPath))
		meta, err := discovery.BuildMeta(absPath, projectDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading session: %v\n", err)
			os.Exit(1)
		}

		app := ui.NewAppWithSession(nil, meta, "", cfg)
		runTUI(app)
		return
	}

	// --pick: always show the picker.
	if *pick {
		sessions, err := scanAllCapped(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error scanning sessions: %v\n", err)
			os.Exit(1)
		}
		app := ui.NewApp(sessions, cfg)
		runTUI(app)
		return
	}

	// Default: try auto-open for CWD project.
	cwdSessions, err := discovery.FindCWDSessions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(cwdSessions) > 0 {
		latest := cwdSessions[0]
		maxAge := time.Duration(cfg.AutoOpenMaxAgeHours) * time.Hour
		if time.Since(latest.EndTime) < maxAge {
			allSessions, _ := scanAllCapped(cfg) // Error is non-fatal — picker works with nil sessions.
			statusMsg := fmt.Sprintf(
				"Opened latest session for %s — P to pick a different session",
				latest.ProjectName)
			app := ui.NewAppWithSession(allSessions, latest, statusMsg, cfg)
			runTUI(app)
			return
		}
	}

	sessions, err := scanAllCapped(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error scanning sessions: %v\n", err)
		os.Exit(1)
	}
	app := ui.NewApp(sessions, cfg)
	runTUI(app)
}

// scanAllCapped scans sessions and caps to the configured max.
func scanAllCapped(cfg *config.Config) ([]*model.SessionMeta, error) {
	sessions, err := discovery.ScanAll()
	if err != nil {
		return nil, err
	}
	if len(sessions) > cfg.MaxPickerSessions {
		sessions = sessions[:cfg.MaxPickerSessions]
	}
	return sessions, nil
}

func runTUI(app ui.App) {
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// runDump parses a session file and prints a structured summary to stdout.
func runDump(path string, cfg *config.Config) error {
	events, err := parser.ParseFile(path, cfg)
	if err != nil {
		return err
	}

	session := parser.BuildSession(events, cfg)

	// Header.
	projectName := filepath.Base(filepath.Dir(path))
	if projectName == "/" || projectName == "." {
		projectName = "unknown"
	}
	startStr := session.StartTime.Local().Format("2006-01-02 15:04")
	duration := ui.FormatDuration(session.EndTime.Sub(session.StartTime))
	var fileSize int64
	if info, err := os.Stat(path); err == nil {
		fileSize = info.Size()
	}
	size := ui.FormatFileSize(fileSize)
	modelName := session.ModelName
	if modelName == "" {
		modelName = "unknown"
	}

	fmt.Printf("Session: %s · %s · %s · %s · %s\n\n",
		projectName, startStr, duration, size, modelName)

	for _, p := range session.Prompts {
		// Branch transition divider.
		if p.BranchTransition.From != "" {
			fmt.Printf("── branch: %s → %s ──\n\n", p.BranchTransition.From, p.BranchTransition.To)
		}

		// Prompt header.
		startTime := p.StartTime.Local().Format("15:04")
		endTime := ""
		if !p.EndTime.IsZero() {
			endTime = p.EndTime.Local().Format("15:04")
		} else {
			endTime = "..."
		}

		humanText := ui.Truncate(p.HumanText, 50)
		if humanText == "" {
			humanText = "(no text)"
		}

		badges := ""
		if p.IsDurationOutlier {
			badges += "  ⏱ slow"
		}
		if p.Interrupted {
			badges += "  ⚠ interrupted"
		}

		fmt.Printf("#%d [%s–%s] %s%s\n", p.Index+1, startTime, endTime, humanText, badges)

		// Context and tokens.
		if p.TokensIn > 0 || p.TokensOut > 0 {
			ctx := ""
			if p.ContextPct > 0 {
				ctx = fmt.Sprintf("ctx: %d%%  ", p.ContextPct)
			}
			fmt.Printf("   %stokens: %s in / %s out\n",
				ctx,
				ui.FormatTokens(p.TokensIn),
				ui.FormatTokens(p.TokensOut))
		}

		// Agents.
		for _, agent := range p.Agents {
			parallel := ""
			if agent.IsParallel {
				parallel = "  [parallel]"
			}
			agentType := agent.SubagentType
			if agentType == "" {
				agentType = "agent"
			}
			modelShort := shortModelName(agent.ModelName)
			if modelShort != "" {
				modelShort = ", " + modelShort
			}
			desc := ui.Truncate(agent.TaskDescription, 40)
			fmt.Printf("   ⬡ %s (%s%s) — %q — %d tools, %s tokens — %s%s\n",
				agent.Label, agentType, modelShort, desc,
				agent.TotalToolUseCount,
				ui.FormatTokens(agent.TotalTokens),
				ui.FormatDuration(time.Duration(agent.TotalDurationMs)*time.Millisecond),
				parallel)
		}

		// Tool calls.
		for _, tc := range p.ToolCalls {
			if tc.Type == model.ToolAgent {
				continue // Already shown in agents section.
			}
			line := formatToolCall(tc)
			if line != "" {
				fmt.Printf("   %s\n", line)
			}
		}

		// Loop warnings.
		for _, w := range p.Warnings {
			if w.Type == model.WarnLoopDetected {
				fmt.Printf("   ⟳ LOOP: %s\n", w.Message)
			}
		}

		fmt.Println()
	}

	return nil
}

// formatToolCall formats a single tool call for --dump output.
func formatToolCall(tc *model.ToolCall) string {
	switch tc.Type {
	case model.ToolWrite:
		lines := strings.Count(tc.Content, "\n")
		return fmt.Sprintf("Write  %s (+%d)", tc.Path, lines)
	case model.ToolEdit:
		added := strings.Count(tc.NewStr, "\n")
		removed := strings.Count(tc.OldStr, "\n")
		return fmt.Sprintf("Edit   %s (+%d -%d)", tc.Path, added, removed)
	case model.ToolRead:
		return fmt.Sprintf("Read   %s", tc.Path)
	case model.ToolBash:
		cmd := ui.Truncate(tc.Command, 50)
		return fmt.Sprintf("Bash   %s", cmd)
	case model.ToolGlob:
		return fmt.Sprintf("Glob   %s", tc.Path)
	case model.ToolGrep:
		return fmt.Sprintf("Grep   %s", tc.Path)
	case model.ToolMCP:
		return fmt.Sprintf("MCP    %s/%s", tc.MCPServerName, tc.MCPToolName)
	case model.ToolWebSearch:
		return fmt.Sprintf("Web    %s", tc.Command)
	default:
		return ""
	}
}

// shortModelName extracts a short form like "haiku" or "sonnet" from a model ID.
func shortModelName(model string) string {
	lower := strings.ToLower(model)
	for _, name := range []string{"opus", "sonnet", "haiku"} {
		if strings.Contains(lower, name) {
			return name
		}
	}
	if model != "" {
		return model
	}
	return ""
}
