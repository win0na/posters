# AGENTS.md

## scope

rules for poster discovery, matching, image download, and source parsing code.

## file layout

- `finder.go`: service types, public poster-finder entrypoints, force-refresh context plumbing.
- `fetch_cache.go`: HTTP fetches, image downloads, cache paths, negative cache, singleflight, and polite per-host throttling.
- `impawards.go`: IMP Awards discovery flow, exact probes, original-title retry, search fallback, and candidate ordering by year/version.
- `impawards_parse.go`: IMP HTML parsing, candidate extraction, search-result links, page-link filtering, and poster image URL upgrades.
- `wikipedia.go`: Wikipedia search/page lookup, title selection, and explicit fallback poster candidate.
- `candidate_matching.go`: structured candidate choice, ambiguity handling, descriptive token matching, visual candidate scoring helpers.
- `visual_match.go`: visual matching orchestration between Wikipedia and IMP candidates.
- `visual.go`: image fingerprint generation and similarity scoring.
- `title_normalization.go`: title normalization, slug generation, URL helpers, version parsing, and image ranking.
- test files mirror these areas; keep new regressions near the policy being tested.

## current behavior

- IMP Awards is the primary poster source.
- Wikipedia is used to locate/confirm the likely theatrical poster through infobox image data.
- Wikipedia image upload is allowed only through explicit fallback mode when IMP is missing or ambiguous.
- Plex `OriginalTitle` is a first-class discovery input and must be tried before broad IMP search fallback.
- visual matching compares multiple fingerprints, including original image, border-trimmed image, and small inset variants.
- ambiguous matches must return `AmbiguousMatchError` instead of choosing silently.

## invariants

- keep all source-specific parsing out of TUI code.
- keep source-specific logic in the matching/fetching files above; do not collapse responsibilities back into one large service file.
- keep IMP, Wikipedia, and Plex network fetches centrally throttled and cache-friendly.
- do not add new poster source domains without explicit user approval.
- never treat Wikipedia fallback as default behavior.
- preserve adjacent-year IMP lookup and international IMP path support.
- preserve nominee/search-result link following while filtering to IMP movie pages.
- use title normalization carefully; avoid making prefix matches so broad that wrong sequels/franchises win.
- prefer exact film/year evidence from Wikipedia search results over sequels, franchises, TV series, soundtracks, albums, or video games.
- add regression tests for matching-policy changes.
- prefer files under 500 lines; split by source or matching phase if a file grows too large.
