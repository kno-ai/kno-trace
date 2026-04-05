# M7: Release Polish & Distribution

**Prerequisites:** Read [spec/README.md](../README.md) (core spec). M6 must be complete (all views working).

**Goal:** Ship the control room. v1.0.0 — `brew install kno-trace` works.

**Deliverable:** Public release with working install instructions.

**Use cases served:** All — this milestone polishes the experience across every use case.

---

## Scope

- Interrupted session polish:
  - `⚡ interrupted` badge on last prompt in prompt list
  - Stats bar shows `interrupted` indicator
- Context pressure nudge:
  - Ticker strip message when `ContextPct > config.context_nudge_pct`: `context 81% — consider /compact or new session`
  - This fires only when `ContextPct > 0` (i.e., token data available)
- Polish:
  - All views respond correctly to terminal resize (propagate `tea.WindowSizeMsg`)
  - Empty states with clear messages for all views
  - Corrupt/unreadable JSONL: show error message with file path, do not crash
  - Help overlay: `?` opens floating keybindings panel; `?` or `esc` dismisses
  - `g` / `G`: jump to top / bottom of prompt list
  - `NO_COLOR=1` and 8-color fallback: verify readable output
- `README.md` completed: description, hero GIF + timeline GIF (see Distribution in core spec), install instructions, quick start, keybindings table, link to kno-lens. Opening paragraph should lead with the control room pitch: "Open a second terminal. See everything Claude is doing."
- `.goreleaser.yaml`: darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, windows/amd64; `.tar.gz` for unix, `.zip` for windows; checksums; Homebrew tap formula; Scoop manifest; changelog from git log
- `.github/workflows/release.yaml`: trigger on `git tag v*`; run goreleaser; push tap formula

---

## Acceptance Criteria

- `goreleaser build --snapshot --clean` produces all 5 targets
- `brew install` works on macOS
- Interrupted badge correct
- Context nudge fires only when token data available and exceeds configured threshold
- Help overlay complete and dismissible
- `g`/`G` navigation works
- All empty states graceful
- `NO_COLOR=1 kno-trace` produces readable output
