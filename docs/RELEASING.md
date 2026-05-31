# Releasing

This document describes the public release flow for Skillmux. The current
published version is `v0.1.1`.

## Install Channels

Skillmux supports these install paths:

1. Homebrew through `boringstackoverflow/homebrew-tap`.
2. `install.sh`, backed by GitHub Release binaries.
3. Direct GitHub Release binary downloads.
4. `go install` for Go users.
5. Source builds for contributors.

Homebrew is the default recommendation because users do not need Go installed.

## Standard Release Flow

Start from a clean `main` branch:

```bash
git status --short --branch
GOCACHE=/private/tmp/skillmux-gocache go test ./...
GOCACHE=/private/tmp/skillmux-gocache go vet ./...
GOCACHE=/private/tmp/skillmux-gocache go test -race ./...
git diff --check
```

Move the changelog entry from `Unreleased` to the new version and update any
user-facing version references in the README, install examples, and docs site.

Build, tag, push, and publish the GitHub Release:

```bash
scripts/release.sh v0.1.2 --push-tag --publish
```

The release script:

- Runs `go test ./...`, `go vet ./...`, and `go test -race ./...`.
- Requires a clean working tree.
- Builds macOS and Linux archives into `dist/`.
- Writes `dist/checksums.txt`.
- Creates an annotated git tag.
- Pushes the tag when `--push-tag` is set.
- Creates the GitHub Release and uploads artifacts when `--publish` is set.

If `gh release create` selects the wrong authenticated host, rerun the publish
step explicitly:

```bash
GH_HOST=github.com gh release create v0.1.2 \
  dist/skillmux_Darwin_arm64.tar.gz \
  dist/skillmux_Darwin_x86_64.tar.gz \
  dist/skillmux_Linux_arm64.tar.gz \
  dist/skillmux_Linux_x86_64.tar.gz \
  dist/checksums.txt \
  --repo boringstackoverflow/skillmux \
  --title "Skillmux v0.1.2" \
  --notes "See CHANGELOG.md for release notes."
```

## GitHub Release Artifacts

Each release must include:

- `skillmux_Darwin_arm64.tar.gz`
- `skillmux_Darwin_x86_64.tar.gz`
- `skillmux_Linux_arm64.tar.gz`
- `skillmux_Linux_x86_64.tar.gz`
- `checksums.txt`

Each archive must contain a single `skillmux` binary at the archive root. The
install script and Homebrew formula both depend on these names.

Verify the published release:

```bash
GH_HOST=github.com gh release view v0.1.2 \
  --repo boringstackoverflow/skillmux \
  --json tagName,url,assets,isDraft,isPrerelease
```

## Homebrew Tap

The tap repo is:

```text
https://github.com/boringstackoverflow/homebrew-tap
```

Formula path:

```text
Formula/skillmux.rb
```

Users install with:

```bash
brew install boringstackoverflow/tap/skillmux
```

After every Skillmux release, update the tap formula:

1. Change `version`.
2. Update all four release URLs.
3. Update all four SHA-256 values from `dist/checksums.txt`.
4. Commit and push the tap.

Use this verification flow from the tap checkout:

```bash
brew uninstall skillmux || true
brew install boringstackoverflow/tap/skillmux
skillmux --help
brew test boringstackoverflow/tap/skillmux
brew audit --strict boringstackoverflow/tap/skillmux
```

For `v0.1.1`, the formula was published in
`boringstackoverflow/homebrew-tap` at commit `a9eadfa`.

## Install Script

The repository includes `install.sh`, which downloads the correct archive for
the user's OS and CPU.

Default install:

```bash
curl -fsSL https://raw.githubusercontent.com/boringstackoverflow/skillmux/main/install.sh | sh
```

Pinned install:

```bash
curl -fsSL https://raw.githubusercontent.com/boringstackoverflow/skillmux/main/install.sh | SKILLMUX_VERSION=v0.1.1 sh
```

Install without `sudo`:

```bash
curl -fsSL https://raw.githubusercontent.com/boringstackoverflow/skillmux/main/install.sh | SKILLMUX_INSTALL_DIR="$HOME/.local/bin" sh
```

The script downloads `checksums.txt` from the same release and verifies SHA-256
checksums when `shasum` or `sha256sum` is available.

## Go Install

Stable install:

```bash
go install github.com/boringstackoverflow/skillmux/cmd/skillmux@v0.1.1
```

Latest default branch:

```bash
go install github.com/boringstackoverflow/skillmux/cmd/skillmux@latest
```

## Pre-Release Checklist

- CI is green on `main`.
- `CHANGELOG.md` has the release entry.
- README install instructions and version examples are accurate.
- `go.mod` module path matches the public GitHub repo.
- No local `.codex/`, `.skillmux/`, `dist/`, or generated binaries are staged.
- The GitHub Release has all five artifacts.
- The Homebrew tap formula has been updated and verified.
