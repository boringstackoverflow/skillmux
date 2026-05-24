# Skillmux

A CLI profile manager for Claude, Codex, and `.agent`/`.agents` skill folders.

Skillmux is a profile manager for coding-agent skills. It keeps agent-visible skills small by exposing only the active profile through the native folders that existing agents and marketplace installers already use.

It is designed for developers who use many skills across different workflows and want clean separation between what is installed and what an agent can discover at startup.

## Features

- Profile-scoped skills for Claude, Codex, and direct `.agent(s)` skill folders.
- Compatibility with native folders such as `~/.claude/skills`, `~/.codex/skills`, `~/.agents/skills`, and `~/.agent/skills`.
- Preservation of shared symlink setups like `~/.claude/skills -> ~/.agents/skills`.
- Manifest-backed backups before managed paths are relinked.
- `doctor`, `repair`, `restore`, and `uninstall` workflows for recovery.
- Shell completions for commands, profiles, agents, and backup IDs.
- Temp-home integration tests so real agent folders are not touched during development.

## Install

For most users, install with Homebrew after the first tagged release:

```bash
brew install boringstackoverflow/tap/skillmux
```

Users who do not use Homebrew can use the install script:

```bash
curl -fsSL https://raw.githubusercontent.com/boringstackoverflow/skillmux/main/install.sh | sh
```

Install a specific version:

```bash
curl -fsSL https://raw.githubusercontent.com/boringstackoverflow/skillmux/main/install.sh | SKILLMUX_VERSION=v0.1.0 sh
```

Install somewhere that does not need `sudo`:

```bash
curl -fsSL https://raw.githubusercontent.com/boringstackoverflow/skillmux/main/install.sh | SKILLMUX_INSTALL_DIR="$HOME/.local/bin" sh
```

You can also download a prebuilt binary from GitHub Releases manually:

```bash
curl -L https://github.com/boringstackoverflow/skillmux/releases/download/v0.1.0/skillmux_Darwin_arm64.tar.gz -o skillmux.tar.gz
tar -xzf skillmux.tar.gz
install -m 0755 skillmux /usr/local/bin/skillmux
```

Replace `Darwin_arm64` with the archive for your OS and CPU.

Go users can install directly from the module:

```bash
go install github.com/boringstackoverflow/skillmux/cmd/skillmux@latest
```

For a pinned Go install:

```bash
go install github.com/boringstackoverflow/skillmux/cmd/skillmux@v0.1.0
```

From a local checkout:

```bash
go install ./cmd/skillmux
```

See [Releasing](docs/RELEASING.md) for the recommended release and packaging flow.

## Quick Start

Preview what Skillmux will manage:

```bash
skillmux init --profile work --dry-run
```

Initialize and import existing skills:

```bash
skillmux init --profile work --yes
```

Create and switch profiles:

```bash
skillmux profile create frontend
skillmux use frontend
skillmux current
```

Create while switching when you mean it:

```bash
skillmux use research --create
```

Inspect and repair:

```bash
skillmux profile show frontend
skillmux scan --profile frontend
skillmux doctor
skillmux repair --dry-run
```

## Shell Completion

Skillmux includes Cobra shell completions. They complete subcommands, flags, profiles, agents, and backup IDs.

Load completion for the current shell session:

```bash
source <(skillmux completion zsh)
source <(skillmux completion bash)
skillmux completion fish | source
skillmux completion powershell | Out-String | Invoke-Expression
```

Install completion for future sessions:

```bash
skillmux completion zsh > "$(brew --prefix)/share/zsh/site-functions/_skillmux"
skillmux completion bash > "$(brew --prefix)/etc/bash_completion.d/skillmux"
skillmux completion fish > ~/.config/fish/completions/skillmux.fish
```

Run `skillmux completion <shell> --help` for shell-specific setup notes.

## Supported Skill Roots

Skillmux detects and manages these skill roots:

```text
~/.claude/skills
~/.codex/skills
~/.agents/skills
~/.agent/skills
```

If an agent root already points at a shared direct root, for example:

```text
~/.claude/skills -> ~/.agents/skills
```

Skillmux preserves that topology. It imports the skills once, keeps `~/.agents/skills` as the primary active profile view, and keeps `~/.claude/skills` as an alias to it.

## Project-Local Profiles

Project-local profile switching is opt-in with `.skillmux.toml`:

```toml
profile = "work"
agents = ["claude", "codex", "agents"]
```

Then run:

```bash
skillmux enter
```

If the configured profile does not exist yet, create it explicitly:

```bash
skillmux enter --create
```

## Safety Model

Skillmux manages custom skill assets and link topology. It does not manage agent sessions, logs, caches, telemetry, auth files, histories, or runtime databases.

Before `init`, `repair`, `restore`, `uninstall`, or risky relinking, Skillmux writes a TOML backup manifest under:

```text
~/.skillmux/backups/
```

`skillmux uninstall` restores the latest pre-init backup by default and keeps `~/.skillmux` for audit.

List backup IDs before restore or targeted uninstall:

```bash
skillmux backup list
skillmux restore <backup-id> --yes
skillmux uninstall --backup-id <backup-id> --yes
```

## Development

```bash
go test ./...
go vet ./...
go test -race ./...
```

## Documentation

- [Project guide](docs/PROJECT.md)
- [Releasing](docs/RELEASING.md)
- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)
- [Changelog](CHANGELOG.md)

## License

Skillmux is released under the MIT License. See [LICENSE](LICENSE).
