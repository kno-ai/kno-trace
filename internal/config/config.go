// Package config handles configuration loading with sensible defaults.
// Config is loaded from ~/.config/kno-trace/config.yaml (via os.UserConfigDir).
// If no file exists, all defaults apply. Unknown keys are silently ignored.
package config

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds all configurable values for kno-trace.
type Config struct {
	// Model context window sizes (tokens). Key = model name substring (lowercase).
	// First match wins; more specific patterns should appear first.
	ModelContextWindows map[string]int `yaml:"model_context_windows"`

	// Fallback window size when no model pattern matches.
	DefaultContextWindow int `yaml:"default_context_window"`

	// Warning thresholds (percentage of context window).
	ContextHighPct     int `yaml:"context_high_pct"`
	ContextCriticalPct int `yaml:"context_critical_pct"`
	ContextNudgePct    int `yaml:"context_nudge_pct"`

	// Auto-open latest CWD session if modified within this many hours.
	AutoOpenMaxAgeHours int `yaml:"auto_open_max_age_hours"`

	// Same tool+path repeated this many times triggers loop warning.
	LoopDetectionThreshold int `yaml:"loop_detection_threshold"`

	// Max WriteSnapshots retained per file (oldest evicted first).
	MaxSnapshotsPerFile int `yaml:"max_snapshots_per_file"`
}

// Default returns a Config with all default values.
func Default() *Config {
	return &Config{
		ModelContextWindows: map[string]int{
			"opus":   1000000,
			"sonnet": 200000,
			"haiku":  200000,
		},
		DefaultContextWindow:   200000,
		ContextHighPct:         70,
		ContextCriticalPct:     85,
		ContextNudgePct:        80,
		AutoOpenMaxAgeHours:    24,
		LoopDetectionThreshold: 3,
		MaxSnapshotsPerFile:    10,
	}
}

// Load reads config from the standard path, falling back to defaults for
// any missing values. Returns defaults if the file doesn't exist.
func Load() *Config {
	cfg := Default()

	configDir, err := os.UserConfigDir()
	if err != nil {
		return cfg
	}

	data, err := os.ReadFile(filepath.Join(configDir, "kno-trace", "config.yaml"))
	if err != nil {
		return cfg
	}

	// Parse into a separate struct so we can merge non-zero values.
	var override Config
	if err := yaml.Unmarshal(data, &override); err != nil {
		return cfg
	}

	// Merge overrides — only replace defaults for fields the user actually set.
	if override.ModelContextWindows != nil {
		cfg.ModelContextWindows = override.ModelContextWindows
	}
	if override.DefaultContextWindow != 0 {
		cfg.DefaultContextWindow = override.DefaultContextWindow
	}
	if override.ContextHighPct != 0 {
		cfg.ContextHighPct = override.ContextHighPct
	}
	if override.ContextCriticalPct != 0 {
		cfg.ContextCriticalPct = override.ContextCriticalPct
	}
	if override.ContextNudgePct != 0 {
		cfg.ContextNudgePct = override.ContextNudgePct
	}
	if override.AutoOpenMaxAgeHours != 0 {
		cfg.AutoOpenMaxAgeHours = override.AutoOpenMaxAgeHours
	}
	if override.LoopDetectionThreshold != 0 {
		cfg.LoopDetectionThreshold = override.LoopDetectionThreshold
	}
	if override.MaxSnapshotsPerFile != 0 {
		cfg.MaxSnapshotsPerFile = override.MaxSnapshotsPerFile
	}

	return cfg
}

// ContextWindowSize returns the context window size for a given model name.
// Uses lowercase substring matching against ModelContextWindows keys.
// Falls back to DefaultContextWindow if no pattern matches.
func (c *Config) ContextWindowSize(model string) int {
	lower := strings.ToLower(model)
	for pattern, size := range c.ModelContextWindows {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return size
		}
	}
	return c.DefaultContextWindow
}
