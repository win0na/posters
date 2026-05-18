# AGENTS.md

## scope

rules for Bubble Tea TUI code.

## current behavior

- screens: login, auth wait, servers, libraries, update mode, movie selection, running, status, done, error.
- default mode updates all eligible posters; specific-poster selection remains available.
- dry-run, force, and Wikipedia fallback can be toggled from mode/movie screens.
- update progress runs multiple workers and shows active worker rows plus recent activity.
- successful update rows use compact visual-match wording: `✓ updated Title (Year), visual match NN.N%`.
- ambiguous results are labeled `AMBIGUOUS`; missing IMP results are labeled `SKIP`/`skipped`.
- run completion writes JSON and CSV reports.

## invariants

- model screens as explicit states.
- never block `Update`; network and disk work must return `tea.Cmd`.
- show spinner or progress for network-bound commands.
- support `q`, `ctrl+c`, and escape/back/cancel paths where practical.
- preserve narrow-terminal layouts and wrapping.
- keep styles centralized.
- do not log Plex tokens, full auth payloads, or credential JSON.
- keep persistence and poster matching policy out of TUI code; call config/poster/Plex packages instead.
- update TUI tests when changing keybindings, labels, status summaries, reports, or progress row wording.
