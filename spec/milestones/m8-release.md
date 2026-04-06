# M8: Release Polish & Distribution

**Prerequisites:** M7 complete (all detail pane features working).

**Goal:** Ship the control room. v1.0.0 — `brew install kno-trace` works.

**Deliverable:** Public release with working install instructions.

**Use cases served:** All — this milestone polishes the experience.

---

## Scope

- Context pressure:
  - Sparkline in stats bar showing context% trajectory across prompts
  - Ticker nudge when context exceeds threshold
  - Compact markers visible in sparkline
- Polish:
  - Terminal resize propagation (all views)
  - Empty states with clear messages everywhere
  - Corrupt JSONL: error message, no crash
  - `?` help overlay with keybindings
  - `NO_COLOR=1` support
- Distribution:
  - `.goreleaser.yaml`: darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, windows/amd64
  - `.github/workflows/release.yaml`: trigger on `git tag v*`
  - Homebrew tap formula
  - README with hero GIF, install instructions, quick start

---

## Acceptance Criteria

- `goreleaser build --snapshot --clean` produces all targets
- `brew install` works on macOS
- Context sparkline renders with ≥3 data points
- Help overlay complete and dismissible
- All empty states graceful
- `NO_COLOR=1` readable
