package ui

import (
	"fmt"
	"strings"

	"github.com/kno-ai/kno-trace/internal/model"
	"github.com/kno-ai/kno-trace/internal/replay"
)

// buildComparison renders a diff view comparing file changes between two turn indices.
// It collects all Write and Edit operations from turns in the range (fromIdx, toIdx]
// and shows a unified diff for each file.
func buildComparison(session *model.Session, fromIdx, toIdx int, width int) string {
	if session == nil || fromIdx >= toIdx {
		return MutedStyle.Render("  Select two turns to compare (earlier first)")
	}

	// Collect file modifications in the range.
	type fileChange struct {
		path      string
		firstOld  string // earliest old content (for Edit) or empty
		lastNew   string // latest new content
		ops       int
		hasEdit   bool
		hasWrite  bool
		hasBash   bool
		agents    []string
	}
	files := make(map[string]*fileChange)

	collectOps := func(tc *model.ToolCall, agentLabel string) {
		if tc.Path == "" {
			return
		}
		switch tc.Type {
		case model.ToolEdit:
			fc, ok := files[tc.Path]
			if !ok {
				fc = &fileChange{path: tc.Path, firstOld: tc.OldStr}
				files[tc.Path] = fc
			}
			fc.lastNew = tc.NewStr
			fc.ops++
			fc.hasEdit = true
		case model.ToolWrite:
			fc, ok := files[tc.Path]
			if !ok {
				fc = &fileChange{path: tc.Path}
				files[tc.Path] = fc
			}
			fc.lastNew = tc.Content
			fc.ops++
			fc.hasWrite = true
		case model.ToolBash:
			fc, ok := files[tc.Path]
			if !ok {
				return // Bash doesn't track paths, skip
			}
			fc.hasBash = true
		}
		if agentLabel != "" {
			fc := files[tc.Path]
			if fc == nil {
				return
			}
			found := false
			for _, a := range fc.agents {
				if a == agentLabel {
					found = true
					break
				}
			}
			if !found {
				fc.agents = append(fc.agents, agentLabel)
			}
		}
	}

	for _, p := range session.Prompts {
		if p.Index <= fromIdx || p.Index > toIdx {
			continue
		}
		for _, tc := range p.ToolCalls {
			collectOps(tc, "")
		}
		for _, ag := range p.Agents {
			for _, tc := range ag.ToolCalls {
				collectOps(tc, ag.Label)
			}
		}
	}

	if len(files) == 0 {
		return MutedStyle.Render(fmt.Sprintf("  No file changes between #%d and #%d", fromIdx+1, toIdx+1))
	}

	var b strings.Builder
	b.WriteString(SelectedStyle.Render(fmt.Sprintf("  Comparing #%d → #%d", fromIdx+1, toIdx+1)))
	b.WriteString("\n\n")

	// Summary.
	totalOps := 0
	for _, fc := range files {
		totalOps += fc.ops
	}
	b.WriteString(MutedStyle.Render(fmt.Sprintf("  %d files changed, %d operations", len(files), totalOps)))
	b.WriteString("\n\n")

	// Sort files alphabetically.
	var sortedPaths []string
	for path := range files {
		sortedPaths = append(sortedPaths, path)
	}
	for i := 1; i < len(sortedPaths); i++ {
		for j := i; j > 0 && sortedPaths[j] < sortedPaths[j-1]; j-- {
			sortedPaths[j], sortedPaths[j-1] = sortedPaths[j-1], sortedPaths[j]
		}
	}

	for _, path := range sortedPaths {
		fc := files[path]

		// File header.
		agentStr := ""
		if len(fc.agents) > 0 {
			agentStr = " ⬡ " + strings.Join(fc.agents, ", ")
		}
		b.WriteString(SelectedStyle.Render("  "+path) + DimStyle.Render(agentStr))
		b.WriteString("\n")

		// Show diff if we have both old and new content.
		if fc.hasEdit && fc.firstOld != "" && fc.lastNew != "" {
			renderColoredDiff(&b, replay.FormatFullDiff(fc.firstOld, fc.lastNew, width))
		} else if fc.hasWrite {
			lines := strings.Count(fc.lastNew, "\n")
			b.WriteString(DimStyle.Render(fmt.Sprintf("    written (%d lines)", lines)) + "\n")
		}

		if fc.hasBash {
			b.WriteString(DimStyle.Render("    ⚠ bash commands present — file effects not captured") + "\n")
		}

		b.WriteString("\n")
	}

	return b.String()
}
