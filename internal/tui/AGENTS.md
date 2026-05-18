# AGENTS.md

## scope

rules for Bubble Tea TUI code.

## invariants

- model screens as explicit states
- never block `Update`; network and disk work must return `tea.Cmd`
- show spinner or progress for network-bound commands
- support `q`, `ctrl+c`, and escape/back paths where practical
- keep styles centralized and usable on narrow terminals
