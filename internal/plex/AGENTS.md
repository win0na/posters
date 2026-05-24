# AGENTS.md

## scope

rules for Plex.tv and Plex Media Server API integration code.

## current behavior

- Plex PIN auth starts through Plex.tv and polls until a token is available or auth expires.
- saved tokens are reused on startup and cleared after unauthorized/expired auth flows.
- servers, movie libraries, and movie items are fetched from Plex APIs.
- movie records include rating key, title, original title, year, and GUID.
- poster uploads target Plex Media Server poster endpoints.
- last selected server/library is persisted by the config package, not Plex.

## invariants

- never log Plex tokens or full auth payloads.
- keep Plex.tv auth separate from Plex Media Server calls.
- keep Plex API integration modular if it grows; prefer files under 500 lines and split auth, discovery, and uploads by responsibility.
- use a stable local `X-Plex-Client-Identifier` from `~/.config/posters`.
- do not add environment-variable based auth paths.
- keep unauthorized, expired, unavailable, and ambiguous server responses recoverable by the TUI.
- preserve `OriginalTitle` extraction; poster discovery depends on it.
- prefer explicit request/response structs over ad-hoc maps once endpoints stabilize.
