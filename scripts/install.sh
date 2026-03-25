#!/usr/bin/env sh
set -eu

REPO="${ROOM_INSTALL_REPO:-jcpsimmons/room}"
VERSION="${ROOM_VERSION:-latest}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

need_cmd uname
need_cmd curl
need_cmd tar
need_cmd install

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"

case "$os" in
  darwin) os="darwin" ;;
  linux) os="linux" ;;
  *)
    echo "unsupported OS: $os" >&2
    exit 1
    ;;
esac

case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *)
    echo "unsupported architecture: $arch" >&2
    exit 1
    ;;
esac

asset="room_${os}_${arch}.tar.gz"
base="https://github.com/${REPO}/releases"
if [ "$VERSION" = "latest" ]; then
  url="${base}/latest/download/${asset}"
else
  url="${base}/download/${VERSION}/${asset}"
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT INT TERM

archive="${tmpdir}/${asset}"
curl -fsSL "$url" -o "$archive"
tar -xzf "$archive" -C "$tmpdir"

target_dir="${ROOM_INSTALL_DIR:-}"
if [ -z "$target_dir" ]; then
  if [ -w "/usr/local/bin" ]; then
    target_dir="/usr/local/bin"
  else
    target_dir="${HOME}/.local/bin"
  fi
fi

mkdir -p "$target_dir"
install -m 0755 "${tmpdir}/room" "${target_dir}/room"

echo "installed room to ${target_dir}/room"
if ! printf '%s' "${PATH}" | tr ':' '\n' | grep -Fx "${target_dir}" >/dev/null 2>&1; then
  echo "note: ${target_dir} is not currently on PATH" >&2
fi
