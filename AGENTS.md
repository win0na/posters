# AGENTS.md

## purpose

this file defines repo-wide defaults for human and ai contributors.

keep this file compact. it should contain only durable, repo-wide rules. put setup, install, and run commands in `README.md`. put deeper llm-facing reference material in `llm/`.

## project overview

this repository contains **posters**, a Go TUI for updating Plex movie library posters to original theatrical posters.

core goals:
- authenticate with Plex without environment variables or API keys
- store Plex credentials under `~/.config/posters`
- list Plex libraries and default update mode to all posters
- source poster candidates exclusively from `impawards.com`
- cross-reference IMP Awards candidates with the movie's Wikipedia main poster
- save completion metadata locally under `~/.config/posters`
- provide clear Bubble Tea progress feedback for auth, selection, matching, and updates

do not infer deep internals from this file alone. use:
- `README.md` for setup, run, and verification commands
- `llm/ARCHITECTURE.md` for subsystem map when present
- nearest nested `AGENTS.md` for package-specific rules
- source files for current behavior

## working principles

these rules keep changes small, grounded, and easy to verify.

- prefer small, focused changes over broad rewrites
- keep Plex API, poster sourcing, matching, and TUI responsibilities separated
- design TUI flows as explicit Bubble Tea state machines
- keep long-running work asynchronous; never block Bubble Tea `Update`
- show progress or spinner for any network-bound operation
- ask before changing data persistence, Plex metadata strategy, or poster matching policy
- if instructions conflict, prefer:
  1. direct user instructions
  2. nearest nested `AGENTS.md`
  3. this root file

## implementation invariants

these are repo-wide facts worth preserving unless the user explicitly changes them.

- no environment variables or user-provided API keys are required
- runtime credentials live under `~/.config/posters`
- local storage is the source of truth for poster-update completion state
- poster-update completion state must not be embedded in the Plex item
- IMP Awards is the exclusive poster source
- Wikipedia is used only to identify/confirm the original theatrical poster
- default action is updating all eligible posters, not specific posters
- network fetchers must identify ambiguous matches instead of silently guessing
- UX must remain usable on narrow terminals and support cancellation/error states

## token optimization

this file is always-loaded minimum context.

- keep only repo-wide, durable defaults here
- do not put temporary plans or large file trees here
- prefer exact file references over pasted excerpts
- put detailed protocol notes in `llm/`

## agent efficiency rules

use smallest context that still allows correct work.

- read only files needed for current task
- prefer targeted searches over broad repository dumps
- summarize findings before large changes
- preserve package boundaries when editing
- add nested `AGENTS.md` files when a directory gains durable local rules
- update docs when commands, config paths, or workflows change

## safety baseline

these constraints apply across repository.

- never commit secrets, credentials, Plex tokens, auth caches, `.env` files, private keys, or browser cookies
- never log auth tokens or full credential payloads
- never scrape or download from sources other than IMP Awards for poster assets
- respect rate limits and add polite throttling for web requests
- if a required check cannot be run, say so explicitly
- never use destructive git operations unless explicitly requested

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
- `plex(metadata): embed poster completion marker`
- `posters(impawards): match theatrical candidates`

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
