# AGENTS.md

## scope

rules for poster discovery, matching, image download, and source parsing code.

## current behavior

- IMP Awards is the primary poster source.
- Wikipedia is used to locate/confirm the likely theatrical poster through infobox image data.
- Wikipedia image upload is allowed only through explicit fallback mode when IMP is missing or ambiguous.
- Plex `OriginalTitle` is a first-class discovery input and must be tried before broad IMP search fallback.
- visual matching compares multiple fingerprints, including original image, border-trimmed image, and small inset variants.
- ambiguous matches must return `AmbiguousMatchError` instead of choosing silently.

## invariants

- keep all source-specific parsing out of TUI code.
- keep IMP, Wikipedia, and Plex network fetches centrally throttled and cache-friendly.
- do not add new poster source domains without explicit user approval.
- never treat Wikipedia fallback as default behavior.
- preserve adjacent-year IMP lookup and international IMP path support.
- preserve nominee/search-result link following while filtering to IMP movie pages.
- use title normalization carefully; avoid making prefix matches so broad that wrong sequels/franchises win.
- prefer exact film/year evidence from Wikipedia search results over sequels, franchises, TV series, soundtracks, albums, or video games.
- add regression tests for matching-policy changes.
