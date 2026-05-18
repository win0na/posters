# AGENTS.md

## scope

rules for poster discovery, matching, and download code.

## invariants

- use IMP Awards as only poster image source
- IMP Awards search is allowed for discovery; keep it polite and throttled
- use Wikipedia only to identify or confirm original theatrical poster
- skip ambiguous matches instead of guessing
- preserve polite fetch behavior and centralize throttling
- keep source-specific parsing outside TUI code
