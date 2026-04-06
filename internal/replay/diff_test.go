package replay

import (
	"strings"
	"testing"
)

func TestComputeLineDiff_AddLines(t *testing.T) {
	old := "line1\nline2\n"
	new := "line1\nline2\nline3\n"
	lines := ComputeLineDiff(old, new)

	hasAdd := false
	for _, l := range lines {
		if l.Type == DiffAdd && strings.Contains(l.Content, "line3") {
			hasAdd = true
		}
	}
	if !hasAdd {
		t.Error("expected added line for line3")
	}
}

func TestComputeLineDiff_RemoveLines(t *testing.T) {
	old := "line1\nline2\nline3\n"
	new := "line1\nline3\n"
	lines := ComputeLineDiff(old, new)

	hasDel := false
	for _, l := range lines {
		if l.Type == DiffDel && strings.Contains(l.Content, "line2") {
			hasDel = true
		}
	}
	if !hasDel {
		t.Error("expected deleted line for line2")
	}
}

func TestComputeLineDiff_NoChange(t *testing.T) {
	text := "same\ntext\n"
	lines := ComputeLineDiff(text, text)

	for _, l := range lines {
		if l.Type != DiffContext {
			t.Errorf("expected all context lines, got %d", l.Type)
		}
	}
}

func TestComputeLineDiff_EmptyInputs(t *testing.T) {
	lines := ComputeLineDiff("", "")
	if len(lines) != 0 {
		t.Errorf("expected 0 lines for empty inputs, got %d", len(lines))
	}
}

func TestComputeLineDiff_EmptyToContent(t *testing.T) {
	lines := ComputeLineDiff("", "hello\nworld\n")
	addCount := 0
	for _, l := range lines {
		if l.Type == DiffAdd {
			addCount++
		}
	}
	if addCount < 2 {
		t.Errorf("expected at least 2 added lines, got %d", addCount)
	}
}

func TestFormatMiniDiff_Truncation(t *testing.T) {
	old := "a\nb\nc\nd\ne\nf\n"
	new := "A\nB\nC\nD\nE\nF\n"

	result := FormatMiniDiff(old, new, 4, 80)
	lines := strings.Split(strings.TrimSpace(result), "\n")

	// Should have at most 4 change lines + 1 "... more" line.
	if len(lines) > 5 {
		t.Errorf("expected at most 5 output lines (4 changes + more), got %d", len(lines))
	}
	if !strings.Contains(result, "more") {
		t.Error("expected '... more' truncation message")
	}
}

func TestFormatMiniDiff_SmallChange(t *testing.T) {
	old := "hello\n"
	new := "world\n"
	result := FormatMiniDiff(old, new, 4, 80)
	if !strings.Contains(result, "-") || !strings.Contains(result, "+") {
		t.Error("expected both + and - lines in mini diff")
	}
}

func TestFormatFullDiff(t *testing.T) {
	old := "line1\nline2\nline3\n"
	new := "line1\nmodified\nline3\n"
	result := FormatFullDiff(old, new, 80)

	if !strings.Contains(result, "+") {
		t.Error("expected + lines in full diff")
	}
	if !strings.Contains(result, "-") {
		t.Error("expected - lines in full diff")
	}
}

func TestFormatFullDiff_NoChanges(t *testing.T) {
	result := FormatFullDiff("same\n", "same\n", 80)
	if strings.Contains(result, "+") || strings.Contains(result, "-") {
		t.Error("expected no +/- lines for identical content")
	}
}
