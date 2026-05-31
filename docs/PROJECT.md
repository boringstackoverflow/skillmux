# Skillmux Project Guide

Skillmux is an open-source profile manager for coding-agent skills. It helps developers keep agent startup context small by exposing only the skills for the active profile while remaining compatible with existing agent-native skill folders and third-party installers.

## Problem

Coding-agent users often accumulate skills across different projects, teams, and workflows. When every skill lives in a global agent folder, agents may discover too much at startup:

- Startup context grows unnecessarily.
- Skill selection gets noisy.
- Irrelevant or conflicting skills become more likely.
- Marketplace installs pollute the global namespace.
- Users have no clean separation between installed skills and visible skills.

Skillmux changes the model from:

```text
installed skills = visible skills = discoverable skills
```

to:

```text
installed skills != active profile skills != visible agent skills
```

## Core Model

A profile is a named set of skills and custom agent assets for a purpose such as `frontend`, `backend`, `infra`, `research`, or `writing`.

Skillmux exposes the active profile through the folders agents already read:

```text
~/.claude/skills
~/.codex/skills
~/.cursor/skills
~/.agents/skills
~/.agent/skills
```

Third-party installers that write to those native folders continue to work. When Skillmux links are active, installer writes land in the active profile instead of a global shared folder.

## Supported Layouts

Skillmux supports separate roots:

```text
~/.claude/skills
~/.codex/skills
~/.cursor/skills
~/.agents/skills
```

It also supports shared-root setups:

```text
~/.claude/skills -> ~/.agents/skills
~/.codex/skills  -> ~/.agents/skills
~/.cursor/skills -> ~/.agents/skills
~/.claude/skills -> ~/.agent/skills
```

During initialization, Skillmux resolves symlinks and groups roots that already point to the same place. Shared layouts are preserved rather than silently split.

Cursor support manages the user-level `~/.cursor/skills` root. Project-level Cursor skills such as `.cursor/skills` are intentionally left under repository control.

## Safety Principles

Skillmux is intentionally conservative because it manages user-owned agent folders.

- Create a manifest-backed backup before replacing, moving, or relinking managed paths.
- Preserve existing symlink topology by default.
- Fail safely on conflicting skill names with different contents.
- Keep runtime data out of scope.
- Make `doctor`, `repair`, `restore`, and `uninstall` understandable and reversible.

Skillmux manages custom assets such as skills, Claude commands, Codex rules, and Codex instructions. It does not manage sessions, logs, caches, telemetry, auth files, shell history, or local databases.

## Directory Layout

Skillmux stores profiles and state under `~/.skillmux`:

```text
~/.skillmux/
  profiles/
    work/
      roots/
        claude/skills/
        codex/skills/
        cursor/skills/
        agents/skills/
        shared-claude-agents/skills/
      assets/
        claude/commands/
        codex/rules/
        codex/instructions.md

  current/
    roots/
    assets/

  backups/
    <timestamp>-<reason>/
      manifest.toml
      files/

  state/
    active.toml
    root_groups.toml
    assets.toml
```

## CLI Surface

Common commands:

```bash
skillmux init --profile work --dry-run
skillmux init --profile work --yes
skillmux init --profile work --enable cursor --yes
skillmux enable cursor --profile work --yes
skillmux profile create frontend
skillmux profile list
skillmux profile show work
skillmux profile push work --org acme --message "Initial profile"
skillmux profile pull acme/work --profile work-next --yes
skillmux profile diff acme/work
skillmux profile versions acme/work
skillmux profile rollback acme/work --to v1
skillmux use frontend
skillmux use research --create
skillmux use work --agent codex
skillmux current
skillmux login --email you@example.com
skillmux org create acme
skillmux org invite acme --email teammate@example.com
skillmux org join acme --code skmi_...
skillmux org sync
skillmux scan --profile work
skillmux doctor
skillmux repair --dry-run
skillmux backup
skillmux backup list
skillmux restore <backup-id> --yes
skillmux uninstall --yes
```

The `login`, `org`, and cloud-backed `profile push/pull/diff/versions/rollback`
commands are an experimental preview client for the separate `skillmux-cloud`
prototype. They are optional and are not required for local Skillmux usage.

Project-local profile switching is opt-in with `.skillmux.toml`:

```toml
profile = "work"
agents = ["claude", "codex", "cursor", "agents"]
```

Then run:

```bash
skillmux enter
skillmux enter --create
```

The plain `enter` and `use` commands require profiles to already exist. Use
`--create` when creating a profile as part of switching is intentional.

Shell completion is available through Cobra:

```bash
skillmux completion zsh
skillmux completion bash
skillmux completion fish
skillmux completion powershell
```

Completions include subcommands, flags, profiles, agents, and backup IDs.

## Current Scope

Implemented:

- Go CLI for macOS and Linux.
- Claude, Codex, Cursor, and direct `.agent(s)` skill roots.
- Post-init optional adapter enablement with `skillmux enable cursor`.
- Root discovery and topology preservation.
- Profile initialization, creation, switching, listing, showing, and scanning.
- Manifest-backed backup, restore, repair, and uninstall.
- Backup listing and shell completions for profile, agent, and backup values.
- Conservative aliases such as `switch`, `status`, `check`, `profile ls`, and `profile rm`.
- Reserved `.system` skill preservation.
- Experimental Skillmux Cloud preview client commands for login, orgs, profile push/pull/diff/version listing, and rollback.
- Temp-home integration tests for supported layouts.

Not yet implemented:

- Safe-install staging flow.
- Registry or package-management behavior.
- Skill dependency resolution.
- Production Skillmux Cloud service, billing, invites, email delivery, and hosted object storage.
- Windows support.
- Security scanning of third-party skills.
- True concurrent profiles for the same agent.

## Roadmap

Near-term:

- Improve interactive prompts and dry-run summaries.
- Add CI and release guardrails.
- Expand tests for repair and restore edge cases.
- Harden the team profile sync prototype with invites, real magic-link email delivery, billing, and remote object storage.
- Document real-world migration examples.

Later:

- Add `install-safe` staging for marketplace installs.
- Add optional import/export for team profile sharing.
- Add provenance metadata for installed skills.
- Add adapter hooks for more coding agents.

## Non-Goals

Skillmux is not intended to replace native agent skill systems, define a universal skill format, sandbox skill code, or become a general dotfile manager.
