package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kno-ai/kno-trace/internal/config"
	"github.com/kno-ai/kno-trace/internal/discovery"
	"github.com/kno-ai/kno-trace/internal/ui"
)

var version = "dev"

func main() {
	pick := flag.Bool("pick", false, "Always open session picker")
	sessionPath := flag.String("session", "", "Open a specific JSONL session file")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("kno-trace %s\n", version)
		os.Exit(0)
	}

	cfg := config.Load()

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

		// Derive project info from parent directory.
		projectDir := filepath.Base(filepath.Dir(absPath))
		meta, err := discovery.BuildMeta(absPath, projectDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading session: %v\n", err)
			os.Exit(1)
		}

		app := ui.NewAppWithSession(nil, meta, "")
		runTUI(app)
		return
	}

	// --pick: always show the picker.
	if *pick {
		sessions, err := discovery.ScanAll()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error scanning sessions: %v\n", err)
			os.Exit(1)
		}
		app := ui.NewApp(sessions)
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
		latest := cwdSessions[0] // Already sorted by EndTime desc.
		maxAge := time.Duration(cfg.AutoOpenMaxAgeHours) * time.Hour
		if time.Since(latest.EndTime) < maxAge {
			// Auto-open the most recent session.
			allSessions, _ := discovery.ScanAll()
			statusMsg := fmt.Sprintf(
				"Opened latest session for %s — P to pick a different session",
				latest.ProjectName)
			app := ui.NewAppWithSession(allSessions, latest, statusMsg)
			runTUI(app)
			return
		}
	}

	// No recent CWD session — fall back to picker.
	sessions, err := discovery.ScanAll()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error scanning sessions: %v\n", err)
		os.Exit(1)
	}
	app := ui.NewApp(sessions)
	runTUI(app)
}

func runTUI(app ui.App) {
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
