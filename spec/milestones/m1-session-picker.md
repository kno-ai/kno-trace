# M1: Session Picker

**Prerequisites:** Read [spec/README.md](../README.md) (core spec). M0 must be complete (`SCHEMA.md` exists).

**Goal:** The front door to the control room — find a session and see its summary immediately.

**Deliverable:** A working binary any Claude Code user can run right now to browse and open sessions.

**Use cases served:** UC7 (quickly check a past session)

---

## Scope

- Project scaffold: Go modules, directory structure per Architecture
  - Go module name: `github.com/<GITHUB_USERNAME>/kno-trace` — **replace `<GITHUB_USERNAME>` with the actual GitHub username before running `go mod init`**
  - Go version: `go 1.24` or later in go.mod (required by Bubbletea)
  - Directory structure: create all directories listed in the Architecture section, even those used in later milestones, so the tree is established from the start
- `internal/model/types.go` — complete data model as specified in core spec
- `internal/discovery/` — pathenc.go (project path encoding: slash-to-dash), scan.go, meta.go:
  - `SessionMeta` populated from first line (StartTime) and last ~5 lines (EndTime) of each JSONL
  - `FileSizeBytes` from `os.Stat`
  - `Duration` from `EndTime - StartTime`
  - No prompt count, no live indicator — only exact values
- `internal/ui/styles.go` — **build this first, before any other UI file** — all lipgloss styles, colors, brand palette. Every UI file imports from here.
- `internal/ui/picker.go` — full session picker TUI:
  - Sessions grouped by date, sorted by mtime descending
  - Per entry: project name, start time, duration, file size
  - No live indicator
  - Fuzzy filter, `j`/`k` navigation, `enter` to open, `q`/`esc` to quit
  - Empty state: "No sessions found — run Claude Code in a project directory first"
- `internal/config/config.go` — configuration loading with defaults (see Configuration in core spec)
- `internal/ui/format.go` — shared display formatting utilities (see Display Formatting in core spec)
- `internal/ui/app.go` — root Bubbletea model with view routing stub
- On `enter`: session summary card — the first hint of the control room. This is not a blank "loading..." screen. It shows what can be derived from `SessionMeta` without a full parse:
  - Project name and session file path
  - Start time, duration, file size
  - Status bar with `P` to switch sessions, `q` to quit
  - Center message: `"Timeline loading — press t when ready"` (M3 will replace this with the real timeline)
  - This card establishes the pattern: open kno-trace, immediately see session state. Even before M3 builds the live feed, the user should feel oriented.
- Auto-open behavior: if CWD project's latest session is within `auto_open_max_age_hours`, open it directly with status message; otherwise open picker
- `--pick`, `--session <path>`, `--version`, `--help` flags
- `--session` expands relative paths via `filepath.Abs`
- `README.md` skeleton: project name, one-line description, placeholder for install instructions and demo GIF

---

## Acceptance Criteria

- `kno-trace` from a project directory auto-opens latest recent session with status message
- `kno-trace` from a project directory with no recent sessions opens picker
- `kno-trace --pick` shows all sessions across all projects
- No live indicator present anywhere — picker shows factual metadata only
- Fuzzy filter narrows list by project name
- `enter` opens to session summary card showing project name, time, duration, file size; `q` exits cleanly with no confirmation
- `P` from placeholder main view returns to picker
- `--session` accepts relative and absolute paths
- Empty state displays correctly when no sessions exist
- Picker displays duration and file size in human-readable format (see Display Formatting)
- Builds and runs correctly; Windows path logic in place via `os.UserHomeDir()` and `filepath.Join()`

---

## Notes

- Build `styles.go` first — every other UI file depends on it
- No `.goreleaser.yaml` until M7
