# Changelog

All notable changes to Skillmux will be documented here.

## Unreleased

- Add CI for tests, vet, race tests, formatting, whitespace, and shell scripts.
- Add experimental Skillmux Cloud preview client commands for auth, org invite/join, and team profile push/pull/diff/version workflows.
- Improve init and dry-run output with planned link targets and restore guidance.
- Refresh release documentation for the published Homebrew tap flow.

## v0.1.1 - 2026-05-27

- Initial Go CLI implementation.
- Profile switching for Claude, Codex, Cursor, and direct `.agent(s)` skill roots.
- Optional Cursor adapter support for `~/.cursor/skills`.
- Topology preservation for setups such as `~/.claude/skills -> ~/.agents/skills`.
- Manifest-backed backup, repair, restore, and uninstall workflows.
- Temp-home integration tests for supported filesystem layouts.
