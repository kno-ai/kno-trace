// Package replay provides file state reconstruction and diff computation.
package replay

import (
	"fmt"
	"strings"

	dmp "github.com/sergi/go-diff/diffmatchpatch"
)

// DiffLine represents one line of a unified diff.
type DiffLine struct {
	Type    DiffLineType
	Content string
}

// DiffLineType marks a diff line as added, deleted, or context.
type DiffLineType int

const (
	DiffContext DiffLineType = iota
	DiffAdd
	DiffDel
)

// ComputeLineDiff computes a line-level diff between two strings.
// Returns a slice of DiffLines suitable for rendering.
func ComputeLineDiff(oldText, newText string) []DiffLine {
	d := dmp.New()

	// Use line-level diffing for readability.
	oldLines, newLines, lineArray := d.DiffLinesToChars(oldText, newText)
	diffs := d.DiffMain(oldLines, newLines, false)
	diffs = d.DiffCharsToLines(diffs, lineArray)
	diffs = d.DiffCleanupSemantic(diffs)

	var lines []DiffLine
	for _, diff := range diffs {
		text := diff.Text
		// Split into individual lines for rendering.
		splitLines := strings.Split(text, "\n")
		for i, line := range splitLines {
			// Skip trailing empty string from split.
			if i == len(splitLines)-1 && line == "" {
				continue
			}
			switch diff.Type {
			case dmp.DiffInsert:
				lines = append(lines, DiffLine{Type: DiffAdd, Content: line})
			case dmp.DiffDelete:
				lines = append(lines, DiffLine{Type: DiffDel, Content: line})
			case dmp.DiffEqual:
				lines = append(lines, DiffLine{Type: DiffContext, Content: line})
			}
		}
	}
	return lines
}

// FormatMiniDiff renders a truncated inline diff suitable for the turn detail.
// Shows at most maxLines of add/del lines. Context lines between changes are
// compressed. Returns the formatted string.
func FormatMiniDiff(oldText, newText string, maxLines int, width int) string {
	if maxLines <= 0 {
		maxLines = 4
	}
	if width < 20 {
		width = 20
	}

	lines := ComputeLineDiff(oldText, newText)
	if len(lines) == 0 {
		return ""
	}

	var b strings.Builder
	shown := 0
	totalChanges := 0
	for _, l := range lines {
		if l.Type != DiffContext {
			totalChanges++
		}
	}

	for _, l := range lines {
		if l.Type == DiffContext {
			continue // Skip context in mini-diffs — just show changes.
		}
		if shown >= maxLines {
			remaining := totalChanges - shown
			if remaining > 0 {
				b.WriteString(fmt.Sprintf("      ... +%d more changes\n", remaining))
			}
			break
		}

		content := l.Content
		if len(content) > width-8 {
			content = content[:width-8]
		}

		switch l.Type {
		case DiffAdd:
			b.WriteString("      + " + content + "\n")
		case DiffDel:
			b.WriteString("      - " + content + "\n")
		}
		shown++
	}
	return b.String()
}

// FormatFullDiff renders a complete diff with context lines for drill-in views.
func FormatFullDiff(oldText, newText string, width int) string {
	if width < 20 {
		width = 20
	}
	lines := ComputeLineDiff(oldText, newText)
	if len(lines) == 0 {
		return "  (no changes)\n"
	}

	var b strings.Builder
	for _, l := range lines {
		content := l.Content
		if len(content) > width-4 {
			content = content[:width-4]
		}
		switch l.Type {
		case DiffAdd:
			b.WriteString("  + " + content + "\n")
		case DiffDel:
			b.WriteString("  - " + content + "\n")
		case DiffContext:
			b.WriteString("    " + content + "\n")
		}
	}
	return b.String()
}
