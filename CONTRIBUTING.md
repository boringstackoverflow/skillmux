# Contributing

Thanks for helping improve Skillmux.

## Development

Requirements:

- Go 1.25 or newer
- macOS or Linux for filesystem behavior that matches the supported runtime

Run checks before opening a pull request:

```bash
go test ./...
go vet ./...
go test -race ./...
```

Build the CLI:

```bash
go build ./cmd/skillmux
```

## Safety Expectations

Skillmux manipulates user-owned agent folders. Changes should be conservative:

- Use temp-home tests for filesystem behavior.
- Do not write tests that touch real `~/.claude`, `~/.codex`, `~/.agent`, `~/.agents`, or `~/.skillmux`.
- Back up before replacing, moving, or relinking native paths.
- Preserve existing symlink topology unless the user explicitly chooses otherwise.
- Keep runtime data out of scope: sessions, logs, caches, auth files, telemetry, histories, and local databases.

## Pull Requests

Good pull requests include:

- A clear description of the behavior change.
- Tests for new filesystem layouts, migration rules, or repair behavior.
- Notes on any compatibility risk for existing agent setups.

Use small focused changes where possible. For larger design changes, open an issue first and describe the migration and rollback behavior.
