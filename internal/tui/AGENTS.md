# AGENTS.md

## scope

rules for Bubble Tea TUI code.

## file layout

- `app.go`: Bubble Tea model, screen/message types, constructor, `Init`, and core `Update` dispatch.
- `keys.go`: keyboard handling by screen and navigation commands.
- `commands.go`: async auth/server/library/movie loading commands plus small mode/status helpers.
- `run.go`: run orchestration, concurrent worker queue, pending/skipped filtering, report recording, auth recovery.
- `poster_update.go`: per-movie update command, force-refresh context handoff, result/error line formatting.
- `views.go`: top-level screen bodies.
- `dashboard.go`: dashboard panes, status/blacklist helpers, shared summary widgets.
- `running_view.go`: running/done report viewports and recent activity formatting.
- `styles.go`: ANSI/lipgloss styling helpers for lists, key/value rows, reports, and activity lines.
- `shell.go`: card shell, sizing, URL linkification, and generic list/movie renderers.
- `wrapping.go`: wrapping, viewport, row-count, and indentation helpers.
- `selection.go`: movie selection engine.
- test files mirror these areas; keep view/format/run regressions in matching files.

## current behavior

- screens: login, auth wait, servers, libraries, update mode, movie selection, blacklist, running, status, done, error.
- default mode updates all eligible posters; specific-poster selection remains available.
- dry-run, force, and Wikipedia fallback can be toggled from mode/movie screens.
- movies can be blacklisted persistently from the movie screen and managed from the blacklist screen.
- update progress runs multiple workers and shows active worker rows plus recent activity.
- successful update rows use compact visual-match wording: `✓ updated Title (Year), visual match NN.N%`.
- ambiguous results are labeled `AMBIGUOUS`; missing IMP results are labeled `SKIP`/`skipped`.
- run completion writes JSON and CSV reports.

## invariants

- model screens as explicit states.
- preserve the modular TUI layout above; do not fold rendering, key handling, and run orchestration back into `app.go`.
- never block `Update`; network and disk work must return `tea.Cmd`.
- show spinner or progress for network-bound commands.
- support `q`, `ctrl+c`, and escape/back/cancel paths where practical.
- preserve narrow-terminal layouts and wrapping.
- keep styles centralized.
- do not log Plex tokens, full auth payloads, or credential JSON.
- keep persistence and poster matching policy out of TUI code; call config/poster/Plex packages instead.
- update TUI tests when changing keybindings, labels, status summaries, reports, or progress row wording.
- prefer files under 500 lines; split by screen area or rendering responsibility if a file grows too large.
