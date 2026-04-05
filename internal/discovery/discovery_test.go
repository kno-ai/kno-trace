package discovery

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestEncodePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/Users/kevin/code/myproject", "-Users-kevin-code-myproject"},
		{"/home/user/project", "-home-user-project"},
	}
	// On Windows, path separator differs — skip slash-specific tests.
	if runtime.GOOS == "windows" {
		t.Skip("slash-to-dash encoding tests are Unix-specific")
	}
	for _, tt := range tests {
		got := EncodePath(tt.input)
		if got != tt.want {
			t.Errorf("EncodePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func testdataDir() string {
	// testdata is at internal/testdata/, two levels up from discovery/.
	return filepath.Join("..", "testdata")
}

func TestBuildMeta_Simple(t *testing.T) {
	path := filepath.Join(testdataDir(), "simple.jsonl")
	meta, err := BuildMeta(path, "test-project")
	if err != nil {
		t.Fatalf("BuildMeta failed: %v", err)
	}

	if meta.ID != "simple" {
		t.Errorf("ID = %q, want %q", meta.ID, "simple")
	}
	if meta.StartTime.IsZero() {
		t.Error("StartTime is zero")
	}
	if meta.EndTime.IsZero() {
		t.Error("EndTime is zero")
	}
	if meta.Duration <= 0 {
		t.Errorf("Duration = %v, want > 0", meta.Duration)
	}
	if meta.FileSizeBytes <= 0 {
		t.Error("FileSizeBytes is 0")
	}
	// simple.jsonl has cwd="/home/user/project" on first user message.
	if meta.ProjectName != "project" {
		t.Errorf("ProjectName = %q, want %q", meta.ProjectName, "project")
	}
}

func TestBuildMeta_Interrupted(t *testing.T) {
	path := filepath.Join(testdataDir(), "interrupted.jsonl")
	meta, err := BuildMeta(path, "test-project")
	if err != nil {
		t.Fatalf("BuildMeta failed: %v", err)
	}
	if meta.StartTime.IsZero() {
		t.Error("StartTime is zero")
	}
	if meta.EndTime.IsZero() {
		t.Error("EndTime is zero")
	}
}

func TestBuildMeta_EdgeCases(t *testing.T) {
	path := filepath.Join(testdataDir(), "edge_cases.jsonl")
	meta, err := BuildMeta(path, "test-project")
	if err != nil {
		t.Fatalf("BuildMeta failed: %v", err)
	}
	// Should not crash on malformed JSON, blank lines, isMeta messages.
	if meta.StartTime.IsZero() {
		t.Error("StartTime is zero")
	}
}

func TestBuildMeta_AllFixtures(t *testing.T) {
	fixtures := []string{
		"simple.jsonl",
		"interrupted.jsonl",
		"with_agent.jsonl",
		"with_compact.jsonl",
		"mcp_calls.jsonl",
		"parallel_agents.jsonl",
		"replay_chain.jsonl",
		"edge_cases.jsonl",
		"advanced_session.jsonl",
	}
	for _, f := range fixtures {
		t.Run(f, func(t *testing.T) {
			path := filepath.Join(testdataDir(), f)
			meta, err := BuildMeta(path, "test-project")
			if err != nil {
				t.Fatalf("BuildMeta(%s) failed: %v", f, err)
			}
			if meta.StartTime.IsZero() {
				t.Errorf("%s: StartTime is zero", f)
			}
		})
	}
}
