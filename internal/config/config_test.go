package config

import "testing"

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.DefaultContextWindow != 200000 {
		t.Errorf("DefaultContextWindow = %d, want 200000", cfg.DefaultContextWindow)
	}
	if cfg.AutoOpenMaxAgeHours != 24 {
		t.Errorf("AutoOpenMaxAgeHours = %d, want 24", cfg.AutoOpenMaxAgeHours)
	}
}

func TestContextWindowSize(t *testing.T) {
	cfg := Default()

	tests := []struct {
		model string
		want  int
	}{
		{"claude-opus-4-6", 1000000},
		{"claude-sonnet-4-5", 200000},
		{"claude-haiku-4-5-20251001", 200000},
		{"unknown-model-xyz", 200000}, // falls back to default
	}
	for _, tt := range tests {
		got := cfg.ContextWindowSize(tt.model)
		if got != tt.want {
			t.Errorf("ContextWindowSize(%q) = %d, want %d", tt.model, got, tt.want)
		}
	}
}

func TestLoadMissingFile(t *testing.T) {
	// Load should return defaults when no config file exists.
	cfg := Load()
	if cfg.ContextHighPct != 70 {
		t.Errorf("ContextHighPct = %d, want 70", cfg.ContextHighPct)
	}
}
