#!/usr/bin/env sh
set -eu

REPO="boringstackoverflow/skillmux"
DEFAULT_INSTALL_DIR="/usr/local/bin"

version="${SKILLMUX_VERSION:-latest}"
install_dir="${SKILLMUX_INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"

usage() {
  cat <<'EOF'
Install Skillmux from GitHub Releases.

Usage:
  ./install.sh [--version v0.1.1] [--install-dir /usr/local/bin]

Environment:
  SKILLMUX_VERSION       Release version to install. Defaults to latest.
  SKILLMUX_INSTALL_DIR   Destination directory. Defaults to /usr/local/bin.

Examples:
  curl -fsSL https://raw.githubusercontent.com/boringstackoverflow/skillmux/main/install.sh | sh
  SKILLMUX_VERSION=v0.1.1 sh install.sh
  sh install.sh --install-dir "$HOME/.local/bin"
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      version="${2:?missing version}"
      shift 2
      ;;
    --install-dir)
      install_dir="${2:?missing install dir}"
      shift 2
      ;;
    --help|-h)
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

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

need uname
need tar
need mktemp

if command -v curl >/dev/null 2>&1; then
  fetch_cmd="curl"
elif command -v wget >/dev/null 2>&1; then
  fetch_cmd="wget"
else
  echo "missing required command: curl or wget" >&2
  exit 1
fi

os="$(uname -s)"
arch="$(uname -m)"

case "$os" in
  Darwin)
    os="Darwin"
    ;;
  Linux)
    os="Linux"
    ;;
  *)
    echo "unsupported OS: $os" >&2
    exit 1
    ;;
esac

case "$arch" in
  arm64|aarch64)
    arch="arm64"
    ;;
  x86_64|amd64)
    arch="x86_64"
    ;;
  *)
    echo "unsupported architecture: $arch" >&2
    exit 1
    ;;
esac

if [ "$version" = "latest" ]; then
  version_url="https://github.com/$REPO/releases/latest/download"
else
  version_url="https://github.com/$REPO/releases/download/$version"
fi

archive="skillmux_${os}_${arch}.tar.gz"
url="$version_url/$archive"
checksums_url="$version_url/checksums.txt"

tmp="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp"
}
trap cleanup EXIT INT TERM

download() {
  src="$1"
  dst="$2"
  if [ "$fetch_cmd" = "curl" ]; then
    curl -fL "$src" -o "$dst"
  else
    wget -O "$dst" "$src"
  fi
}

echo "Downloading $url"
download "$url" "$tmp/$archive"

if download "$checksums_url" "$tmp/checksums.txt"; then
  if command -v shasum >/dev/null 2>&1; then
    expected="$(awk -v file="$archive" '$2 == file {print $1}' "$tmp/checksums.txt")"
    if [ -n "$expected" ]; then
      actual="$(shasum -a 256 "$tmp/$archive" | awk '{print $1}')"
      if [ "$expected" != "$actual" ]; then
        echo "checksum mismatch for $archive" >&2
        exit 1
      fi
      echo "Checksum verified"
    fi
  elif command -v sha256sum >/dev/null 2>&1; then
    expected="$(awk -v file="$archive" '$2 == file {print $1}' "$tmp/checksums.txt")"
    if [ -n "$expected" ]; then
      actual="$(sha256sum "$tmp/$archive" | awk '{print $1}')"
      if [ "$expected" != "$actual" ]; then
        echo "checksum mismatch for $archive" >&2
        exit 1
      fi
      echo "Checksum verified"
    fi
  else
    echo "Checksum file downloaded, but no shasum or sha256sum command is available; skipping verification" >&2
  fi
else
  echo "No checksums.txt found for this release; skipping checksum verification" >&2
fi

tar -xzf "$tmp/$archive" -C "$tmp"

if [ ! -f "$tmp/skillmux" ]; then
  echo "archive did not contain a skillmux binary" >&2
  exit 1
fi

chmod 0755 "$tmp/skillmux"
mkdir -p "$install_dir"

if [ -w "$install_dir" ]; then
  install -m 0755 "$tmp/skillmux" "$install_dir/skillmux"
else
  echo "Installing to $install_dir requires elevated permissions"
  sudo install -m 0755 "$tmp/skillmux" "$install_dir/skillmux"
fi

echo "Installed skillmux to $install_dir/skillmux"
"$install_dir/skillmux" --help >/dev/null
echo "Run: skillmux --help"
