# M8: Release Polish & Distribution

**Prerequisites:** M7 complete (file intelligence working).

**Goal:** Ship it. `brew install kno-trace` works.

---

## Scope

- Context sparkline in status bar
- `?` help overlay
- `/` search at all levels
- `NO_COLOR=1` support
- goreleaser, GitHub Actions, Homebrew tap
- README with install instructions

---

## Acceptance Criteria

- `goreleaser build --snapshot --clean` produces all targets
- `brew install` works on macOS
- Help overlay shows correct keys for current level
- All empty states graceful
