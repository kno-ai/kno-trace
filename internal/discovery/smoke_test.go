package discovery

import (
	"fmt"
	"testing"
)

// TestSmokeRealSessions verifies discovery works against real ~/.claude/projects/ data.
// This test is informational — it prints what it finds but doesn't fail if no sessions exist.
func TestSmokeRealSessions(t *testing.T) {
	sessions, err := ScanAll()
	if err != nil {
		t.Logf("ScanAll error (may be expected): %v", err)
		return
	}
	t.Logf("Found %d sessions across all projects", len(sessions))
	for i, s := range sessions {
		if i >= 5 {
			break
		}
		t.Logf("  %s | %s | %s", s.ProjectName, s.StartTime.Local().Format("Jan 2 15:04"), s.Duration)
	}

	cwdSessions, err := FindCWDSessions()
	if err != nil {
		t.Logf("FindCWDSessions error: %v", err)
		return
	}
	t.Logf("Found %d sessions for CWD project", len(cwdSessions))
	if len(cwdSessions) > 0 {
		s := cwdSessions[0]
		fmt.Printf("  Latest CWD session: %s | %s\n", s.ProjectName, s.Duration)
	}
}
