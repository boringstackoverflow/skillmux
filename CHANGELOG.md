# Changelog

All notable changes to Skillmux will be documented here.

## Unreleased

## v0.1.1 - 2026-05-27

- Initial Go CLI implementation.
- Profile switching for Claude, Codex, Cursor, and direct `.agent(s)` skill roots.
- Optional Cursor adapter support for `~/.cursor/skills`.
- Topology preservation for setups such as `~/.claude/skills -> ~/.agents/skills`.
- Manifest-backed backup, repair, restore, and uninstall workflows.
- Temp-home integration tests for supported filesystem layouts.
