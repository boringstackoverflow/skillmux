# Releasing

This document describes the recommended public install and release flow for Skillmux.

## Install Channels

Skillmux should support four install paths:

1. Homebrew through a project-owned tap for macOS and Linux users.
2. `install.sh`, backed by GitHub Release binaries, for users without Go or Homebrew.
3. Direct GitHub Release binary downloads.
4. `go install` for Go users and early adopters.
5. Source builds for contributors.

The default user-facing path should not require users to have Go installed.

## Homebrew

Homebrew should be the simplest install path for macOS and Linux users:

```bash
brew install boringstackoverflow/tap/skillmux
```

Do not start with `homebrew/core`. A new niche CLI should first publish its own tap.

Recommended tap:

```text
github.com/boringstackoverflow/homebrew-tap
```

Formula path:

```text
Formula/skillmux.rb
```

Users can also install after tapping explicitly:

```bash
brew tap boringstackoverflow/tap
brew install skillmux
```

For the first public version, prefer a formula that installs release binaries so users do not need Go installed locally:

```ruby
class Skillmux < Formula
  desc "Profile manager for coding-agent skills"
  homepage "https://github.com/boringstackoverflow/skillmux"
  version "0.1.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/boringstackoverflow/skillmux/releases/download/v0.1.0/skillmux_Darwin_arm64.tar.gz"
      sha256 "<darwin-arm64-sha256>"
    else
      url "https://github.com/boringstackoverflow/skillmux/releases/download/v0.1.0/skillmux_Darwin_x86_64.tar.gz"
      sha256 "<darwin-amd64-sha256>"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/boringstackoverflow/skillmux/releases/download/v0.1.0/skillmux_Linux_arm64.tar.gz"
      sha256 "<linux-arm64-sha256>"
    else
      url "https://github.com/boringstackoverflow/skillmux/releases/download/v0.1.0/skillmux_Linux_x86_64.tar.gz"
      sha256 "<linux-amd64-sha256>"
    end
  end

  def install
    bin.install "skillmux"
    generate_completions_from_executable(bin/"skillmux", "completion")
  end

  test do
    system "#{bin}/skillmux", "--help"
  end
end
```

This depends on publishing release archives with a single `skillmux` binary inside each archive.

## GitHub Release Binaries

GitHub Releases should be the canonical no-Go fallback. Each release should include:

- A semantic version tag such as `v0.1.0`.
- Release notes.
- Checksums.
- Prebuilt archives for:
  - `skillmux_Darwin_arm64.tar.gz`
  - `skillmux_Darwin_x86_64.tar.gz`
  - `skillmux_Linux_arm64.tar.gz`
  - `skillmux_Linux_x86_64.tar.gz`

Example manual install for macOS Apple Silicon:

```bash
curl -L https://github.com/boringstackoverflow/skillmux/releases/download/v0.1.0/skillmux_Darwin_arm64.tar.gz -o skillmux.tar.gz
tar -xzf skillmux.tar.gz
install -m 0755 skillmux /usr/local/bin/skillmux
skillmux --help
```

Example manual install for Linux x86_64:

```bash
curl -L https://github.com/boringstackoverflow/skillmux/releases/download/v0.1.0/skillmux_Linux_x86_64.tar.gz -o skillmux.tar.gz
tar -xzf skillmux.tar.gz
sudo install -m 0755 skillmux /usr/local/bin/skillmux
skillmux --help
```

A future `install.sh` can wrap this OS/architecture detection, but direct binary archives should exist first.

## Install Script

The repository includes `install.sh`, which downloads the correct release archive for the user's OS and CPU.

Default install:

```bash
curl -fsSL https://raw.githubusercontent.com/boringstackoverflow/skillmux/main/install.sh | sh
```

Pinned install:

```bash
curl -fsSL https://raw.githubusercontent.com/boringstackoverflow/skillmux/main/install.sh | SKILLMUX_VERSION=v0.1.0 sh
```

Install without `sudo`:

```bash
curl -fsSL https://raw.githubusercontent.com/boringstackoverflow/skillmux/main/install.sh | SKILLMUX_INSTALL_DIR="$HOME/.local/bin" sh
```

The script expects release archives named:

```text
skillmux_Darwin_arm64.tar.gz
skillmux_Darwin_x86_64.tar.gz
skillmux_Linux_arm64.tar.gz
skillmux_Linux_x86_64.tar.gz
```

Each archive must contain a `skillmux` binary at the archive root.

The script also tries to download `checksums.txt` from the same release and verifies SHA-256 checksums when `shasum` or `sha256sum` is available.

The install script only installs the binary. Users can enable shell completion with
`skillmux completion <shell> --help`; Homebrew formulas should install generated
completion files automatically.

## Source Install

After the repository is public, Go users can install from the default branch with:

```bash
go install github.com/boringstackoverflow/skillmux/cmd/skillmux@latest
```

For stable usage, prefer a tagged version:

```bash
go install github.com/boringstackoverflow/skillmux/cmd/skillmux@v0.1.0
```

This is the simplest first public install path because it only requires the repo to be public and the module path to match `go.mod`.

Recommended first-release flow:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Then create a GitHub Release for the tag and attach binaries/checksums. This can start as a manual process; automate it later with GitHub Actions or GoReleaser.

The repository includes `scripts/release.sh` for the manual release flow:

```bash
scripts/release.sh v0.1.0 --dry-run
scripts/release.sh v0.1.0
scripts/release.sh v0.1.0 --push-tag --publish
```

By default, the script:

- Runs `go test ./...`, `go vet ./...`, and `go test -race ./...`.
- Requires a clean working tree.
- Builds macOS and Linux archives into `dist/`.
- Writes `dist/checksums.txt`.
- Creates an annotated local Git tag.

With `--push-tag`, it pushes the tag to `origin`. With `--publish`, it also uses the GitHub CLI to create the GitHub Release and upload artifacts.

## Recommended Order

1. Publish this repository publicly.
2. Run `scripts/release.sh v0.1.0 --dry-run`.
3. Run `scripts/release.sh v0.1.0 --push-tag --publish`.
4. Verify install script and direct binary install.
5. Verify Go install:

```bash
go install github.com/boringstackoverflow/skillmux/cmd/skillmux@v0.1.0
```

6. Create `boringstackoverflow/homebrew-tap`.
7. Add `Formula/skillmux.rb`.
8. Verify:

```bash
brew install boringstackoverflow/tap/skillmux
skillmux --help
```

9. Add release automation once the manual flow is proven.

## Pre-Release Checklist

Run:

```bash
go test ./...
go vet ./...
go test -race ./...
go build ./cmd/skillmux
scripts/release.sh v0.1.0 --dry-run
```

Review:

- README install instructions are accurate.
- `CHANGELOG.md` has the release entry.
- `go.mod` module path matches the public GitHub repo.
- No local `.codex/`, `.skillmux/`, or generated binaries are staged.
