#!/usr/bin/env sh
set -eu

repo="boringstackoverflow/skillmux"
version=""
dist_dir="dist"
publish="false"
push_tag="false"
skip_checks="false"
dry_run="false"

usage() {
  cat <<'EOF'
Build and optionally publish a Skillmux release.

Usage:
  scripts/release.sh v0.1.0 [options]

Options:
  --dist DIR       Output directory. Defaults to dist.
  --push-tag       Push the created git tag to origin.
  --publish        Create a GitHub Release with gh and upload artifacts.
  --skip-checks    Skip go test, go vet, and go test -race.
  --dry-run        Print the release plan without building, tagging, or publishing.
  -h, --help       Show help.

Examples:
  scripts/release.sh v0.1.0 --dry-run
  scripts/release.sh v0.1.0
  scripts/release.sh v0.1.0 --push-tag --publish
EOF
}

if [ "$#" -eq 0 ]; then
  usage >&2
  exit 2
fi

version="$1"
shift

while [ "$#" -gt 0 ]; do
  case "$1" in
    --dist)
      dist_dir="${2:?missing dist dir}"
      shift 2
      ;;
    --push-tag)
      push_tag="true"
      shift
      ;;
    --publish)
      publish="true"
      push_tag="true"
      shift
      ;;
    --skip-checks)
      skip_checks="true"
      shift
      ;;
    --dry-run)
      dry_run="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

case "$version" in
  v[0-9]*.[0-9]*.[0-9]*)
    ;;
  *)
    echo "version must look like v0.1.0" >&2
    exit 2
    ;;
esac

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

run() {
  echo "+ $*"
  if [ "$dry_run" = "false" ]; then
    "$@"
  fi
}

archive_name() {
  os="$1"
  arch="$2"
  echo "skillmux_${os}_${arch}.tar.gz"
}

build_archive() {
  goos="$1"
  goarch="$2"
  archive_os="$3"
  archive_arch="$4"
  archive="$(archive_name "$archive_os" "$archive_arch")"
  work_dir="$dist_dir/work/${goos}_${archive_arch}"
  binary="$work_dir/skillmux"

  run mkdir -p "$work_dir"
  echo "+ GOOS=$goos GOARCH=$goarch CGO_ENABLED=0 go build -trimpath -ldflags -s -w -o $binary ./cmd/skillmux"
  if [ "$dry_run" = "false" ]; then
    GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$binary" ./cmd/skillmux
  fi
  if [ "$dry_run" = "false" ]; then
    (cd "$work_dir" && tar -czf "../../$archive" skillmux)
  else
    echo "+ (cd $work_dir && tar -czf ../../$archive skillmux)"
  fi
}

checksum_file() {
  if command -v shasum >/dev/null 2>&1; then
    (cd "$dist_dir" && shasum -a 256 skillmux_*.tar.gz > checksums.txt)
  elif command -v sha256sum >/dev/null 2>&1; then
    (cd "$dist_dir" && sha256sum skillmux_*.tar.gz > checksums.txt)
  else
    echo "missing required command: shasum or sha256sum" >&2
    exit 1
  fi
}

need git
need go
need tar

if [ "$publish" = "true" ]; then
  need gh
fi

echo "Release plan"
echo "  version:   $version"
echo "  dist:      $dist_dir"
echo "  push tag:  $push_tag"
echo "  publish:   $publish"
echo "  dry run:   $dry_run"

if [ "$dry_run" = "true" ]; then
  echo
fi

if [ "$skip_checks" = "false" ]; then
  run go test ./...
  run go vet ./...
  run go test -race ./...
fi

if [ "$dry_run" = "false" ]; then
  if ! git diff --quiet || ! git diff --cached --quiet; then
    echo "working tree has uncommitted changes; commit before releasing" >&2
    exit 1
  fi
fi

if git rev-parse "$version" >/dev/null 2>&1; then
  echo "tag already exists: $version" >&2
  exit 1
fi

run rm -rf "$dist_dir"
run mkdir -p "$dist_dir"

build_archive "darwin" "arm64" "Darwin" "arm64"
build_archive "darwin" "amd64" "Darwin" "x86_64"
build_archive "linux" "arm64" "Linux" "arm64"
build_archive "linux" "amd64" "Linux" "x86_64"

echo "+ generate $dist_dir/checksums.txt"
if [ "$dry_run" = "false" ]; then
  checksum_file
fi

run rm -rf "$dist_dir/work"
run git tag -a "$version" -m "Release $version"

if [ "$push_tag" = "true" ]; then
  run git push origin "$version"
fi

if [ "$publish" = "true" ]; then
  run gh release create "$version" "$dist_dir"/skillmux_*.tar.gz "$dist_dir"/checksums.txt \
    --repo "$repo" \
    --title "Skillmux $version" \
    --notes "See CHANGELOG.md for release notes."
fi

echo "Release artifacts:"
if [ "$dry_run" = "false" ]; then
  ls -1 "$dist_dir"
else
  echo "  $dist_dir/skillmux_Darwin_arm64.tar.gz"
  echo "  $dist_dir/skillmux_Darwin_x86_64.tar.gz"
  echo "  $dist_dir/skillmux_Linux_arm64.tar.gz"
  echo "  $dist_dir/skillmux_Linux_x86_64.tar.gz"
  echo "  $dist_dir/checksums.txt"
fi
