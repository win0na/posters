# AGENTS.md

## purpose

repo-wide defaults for human and ai contributors.

keep this file compact. put setup, install, and run commands in `README.md`. put deeper llm-facing reference material in `llm/` when needed.

## project overview

this repository contains **posters**, a Go Bubble Tea TUI for updating Plex movie library posters to original theatrical posters.

current program state:
- authenticates through Plex PIN login; no environment variables or user API keys
- stores Plex token, stable client id, and last server/library under `~/.config/posters/state.json`
- stores local poster-update completion metadata under `~/.config/posters/metadata.json`
- writes JSON and CSV run reports under `~/.config/posters/reports/`
- defaults to updating all eligible movie posters; specific-poster mode remains available
- supports dry-run, force refresh, status view, cancellation, and explicit Wikipedia fallback mode
- discovers primary poster candidates from IMP Awards
- uses Plex `OriginalTitle` before broad IMP search fallback when exact title probes fail
- uses Wikipedia infobox posters to confirm likely theatrical IMP candidates through visual matching
- skips ambiguous matches instead of silently guessing

do not infer deep internals from this file alone. use:
- `README.md` for setup, run, and verification commands
- nearest nested `AGENTS.md` for package-specific rules
- source files for current behavior

## working principles

- prefer small, focused changes over broad rewrites
- keep Plex API, poster sourcing/matching, persistence, and TUI responsibilities separated
- model TUI flows as explicit Bubble Tea state machines
- keep long-running work asynchronous; never block Bubble Tea `Update`
- show progress or spinner for network-bound operations
- preserve cancellation/error states for auth, loading, matching, and updates
- ask before changing data persistence, Plex metadata strategy, or poster matching policy
- if instructions conflict, prefer:
  1. direct user instructions
  2. nearest nested `AGENTS.md`
  3. this root file

## implementation invariants

- no environment variables or user-provided API keys are required
- runtime credentials live under `~/.config/posters`
- local storage is the source of truth for poster-update completion state
- poster-update completion state must not be embedded in the Plex item
- IMP Awards is the primary poster source
- Wikipedia is used to identify/confirm the original theatrical poster and may be used as an explicit opt-in fallback source
- Wikipedia fallback upload must remain off by default
- default action is updating all eligible posters, not specific posters
- network fetchers must identify ambiguous matches instead of silently guessing
- UX must remain usable on narrow terminals and support cancellation/error states

## safety baseline

- never commit secrets, credentials, Plex tokens, auth caches, `.env` files, private keys, or browser cookies
- never log auth tokens or full credential payloads
- never scrape or download poster assets from sources other than IMP Awards, except Wikipedia images used for confirmation/fallback
- respect rate limits; preserve centralized polite throttling and local cache behavior
- if a required check cannot be run, say why
- never use destructive git operations unless explicitly requested

## agent efficiency rules

- read only files needed for current task
- prefer targeted searches over broad repository dumps
- summarize findings before large changes
- preserve package boundaries when editing
- add nested `AGENTS.md` files when a directory gains durable local rules
- update docs when commands, config paths, reports, keybindings, or workflows change

## git conventions

all commits in this repository should be signed and verified when possible.

- create commits with `git commit -S`
- if signing is not configured or signing fails, do not create unsigned fallback commits
- never add co-author trailers, attribution footers, AI credit lines, or similar metadata unless explicitly requested
- keep commit messages clean and project-focused

### commit message format

use this format for short commits:

`topic(short scope): description`

examples:
- `tui(auth): add plex login screen`
- `plex(metadata): store original titles`
- `posters(impawards): match theatrical candidates`
- `docs(readme): refresh usage guide`

for larger commits, use the same first line, followed by short bullets and a final reasoning paragraph.

## verification

before considering work complete, run the smallest relevant verification available.

prefer this order:
1. targeted unit tests for changed packages
2. `go test ./...`
3. formatter/linter checks if configured
4. manual TUI smoke test when UI flow changes

if local verification cannot be run, say why.

## maintenance

update this file only when repo-wide rules change.

do not add one-off task instructions, temporary migration notes, or deep per-package details here.
