# AGENTS.md

## scope

rules for Plex API integration code in this directory.

## invariants

- never log Plex tokens or full auth payloads
- keep Plex.tv auth separate from Plex Media Server calls
- use a stable local `X-Plex-Client-Identifier` from `~/.config/posters`
- prefer explicit request structs over ad-hoc maps once endpoints stabilize
- handle ambiguous or unauthorized server responses as recoverable UI errors
- do not add environment-variable based auth paths
