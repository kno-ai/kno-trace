package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kno-ai/kno-trace/internal/model"
)

// viewMode tracks which screen is currently displayed.
type viewMode int

const (
	viewPicker  viewMode = iota
	viewSession          // session summary card (placeholder for timeline)
)

// App is the root Bubbletea model for kno-trace.
type App struct {
	view        viewMode
	picker      pickerModel
	session     *model.SessionMeta // currently open session
	statusMsg   string             // transient status message (e.g., auto-open notice)
	allSessions []*model.SessionMeta
	width       int
	height      int
}

// NewApp creates the root model starting at the session picker.
func NewApp(sessions []*model.SessionMeta) App {
	return App{
		view:        viewPicker,
		picker:      newPicker(sessions),
		allSessions: sessions,
	}
}

// NewAppWithSession creates the root model auto-opened to a session summary.
func NewAppWithSession(sessions []*model.SessionMeta, session *model.SessionMeta, statusMsg string) App {
	return App{
		view:        viewSession,
		picker:      newPicker(sessions),
		session:     session,
		statusMsg:   statusMsg,
		allSessions: sessions,
	}
}

func (a App) Init() tea.Cmd {
	return nil
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.picker.width = msg.Width
		a.picker.height = msg.Height
		return a, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}

		// View-specific handling (views own `q` so filtering works).
		switch a.view {
		case viewPicker:
			return a.updatePicker(msg)
		case viewSession:
			return a.updateSession(msg)
		}
	}

	return a, nil
}

func (a App) updatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Let picker handle its own keys first if filtering.
	if a.picker.filtering {
		var cmd tea.Cmd
		a.picker, cmd = a.picker.Update(msg)
		return a, cmd
	}

	switch msg.String() {
	case "enter":
		sel := a.picker.selectedSession()
		if sel != nil {
			a.session = sel
			a.view = viewSession
			a.statusMsg = ""
		}
		return a, nil
	case "q":
		return a, tea.Quit
	}

	var cmd tea.Cmd
	a.picker, cmd = a.picker.Update(msg)
	return a, cmd
}

func (a App) updateSession(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return a, tea.Quit
	case "P":
		a.view = viewPicker
		a.statusMsg = ""
		return a, nil
	case "esc":
		a.view = viewPicker
		a.statusMsg = ""
		return a, nil
	}
	return a, nil
}

func (a App) View() string {
	switch a.view {
	case viewPicker:
		return a.picker.View()
	case viewSession:
		return a.viewSession()
	default:
		return ""
	}
}

// viewSession renders the session summary card — the placeholder for the timeline.
func (a App) viewSession() string {
	s := a.session
	if s == nil {
		return EmptyStateStyle.Render("No session selected")
	}

	var b strings.Builder

	// Title bar.
	title := TitleStyle.Render("kno-trace")
	b.WriteString(title)
	b.WriteString("\n\n")

	// Session summary card.
	var card strings.Builder
	card.WriteString(fmt.Sprintf("%s %s\n",
		LabelStyle.Render("Project"),
		ValueStyle.Render(s.ProjectName)))
	card.WriteString(fmt.Sprintf("%s %s\n",
		LabelStyle.Render("Path"),
		MutedStyle.Render(s.FilePath)))
	card.WriteString(fmt.Sprintf("%s %s\n",
		LabelStyle.Render("Started"),
		ValueStyle.Render(s.StartTime.Local().Format("2006-01-02 15:04:05"))))
	card.WriteString(fmt.Sprintf("%s %s\n",
		LabelStyle.Render("Duration"),
		ValueStyle.Render(FormatDuration(s.Duration))))
	card.WriteString(fmt.Sprintf("%s %s\n",
		LabelStyle.Render("File size"),
		ValueStyle.Render(FormatFileSize(s.FileSizeBytes))))

	card.WriteString("\n")
	card.WriteString(MutedStyle.Render("Timeline loading — press t when ready"))

	cardWidth := 60
	if a.width > 10 {
		cardWidth = min(a.width-4, 70)
	}
	rendered := CardStyle.Width(cardWidth).Render(card.String())
	b.WriteString(rendered)

	// Status message (e.g., auto-open notice).
	if a.statusMsg != "" {
		b.WriteString("\n\n")
		b.WriteString(MutedStyle.Render(a.statusMsg))
	}

	// Status bar.
	b.WriteString("\n\n")
	keys := []struct{ key, desc string }{
		{"P", "pick session"},
		{"t", "timeline"},
		{"q", "quit"},
	}
	var parts []string
	for _, k := range keys {
		parts = append(parts,
			KeyStyle.Render(k.key)+" "+KeyDescStyle.Render(k.desc))
	}
	b.WriteString(StatusBarStyle.Render(strings.Join(parts, "  ")))

	return b.String()
}
