#!/bin/sh
# POSIX installer for Linux, macOS, and Git Bash on Windows.
#
#   curl -fsSL https://raw.githubusercontent.com/omartelo/youtrack-tui/main/install.sh | sh

set -eu

REPO="omartelo/youtrack-tui"
API_URL="${YOUTRACK_TUI_API_URL:-https://api.github.com/repos/${REPO}/releases/latest}"
RELEASE_URL="${YOUTRACK_TUI_RELEASE_URL:-https://github.com/${REPO}/releases/download}"

info() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

latest_tag() {
  json="$(curl -fsSL "$API_URL")" || return 1
  printf '%s\n' "$json" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | sed -n '1p'
}

sha256() {
  if have sha256sum; then
    sha256sum "$1" | awk '{print $1}'
  elif have shasum; then
    shasum -a 256 "$1" | awk '{print $1}'
  elif have openssl; then
    openssl dgst -sha256 "$1" | awk '{print $NF}'
  else
    fail "sha256sum, shasum, or openssl is required"
  fi
}

platform() {
  case "$(uname -s)" in
    Linux) printf 'linux' ;;
    Darwin) printf 'darwin' ;;
    MINGW* | MSYS* | CYGWIN*) printf 'windows' ;;
    *) fail "unsupported operating system: $(uname -s)" ;;
  esac
}

architecture() {
  case "$(uname -m)" in
    x86_64 | amd64) printf 'amd64' ;;
    arm64 | aarch64) printf 'arm64' ;;
    *) fail "unsupported architecture: $(uname -m)" ;;
  esac
}

main() {
  have curl || fail "curl is required"
  have tar || fail "tar is required"

  os="$(platform)"
  arch="$(architecture)"
  tag="$(latest_tag)" || fail "could not resolve latest release from ${API_URL}"
  [ -n "$tag" ] || fail "could not resolve latest release from ${API_URL}"
  version="${tag#v}"
  archive="youtrack-tui-${version}-${os}-${arch}.tar.gz"
  binary="youtrack-tui"
  [ "$os" = windows ] && binary="${binary}.exe"

  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' 0
  trap 'exit 1' HUP INT TERM

  base="${RELEASE_URL}/${tag}"
  info "downloading ${archive}"
  curl -fsSL -o "${tmp}/${archive}" "${base}/${archive}"
  curl -fsSL -o "${tmp}/checksums.txt" "${base}/checksums.txt"

  expected="$(awk -v file="$archive" '$2 == file {print $1}' "${tmp}/checksums.txt")"
  [ -n "$expected" ] || fail "checksum for ${archive} is missing"
  actual="$(sha256 "${tmp}/${archive}")"
  [ "$actual" = "$expected" ] || fail "checksum mismatch for ${archive}"

  tar -xzf "${tmp}/${archive}" -C "$tmp"
  [ -f "${tmp}/${binary}" ] || fail "${binary} is missing from ${archive}"

  install_dir="${INSTALL_DIR:-${HOME}/.local/bin}"
  mkdir -p "$install_dir"
  cp "${tmp}/${binary}" "${install_dir}/${binary}"
  chmod 755 "${install_dir}/${binary}"

  info "installed ${install_dir}/${binary}"
  case ":${PATH}:" in
    *":${install_dir}:"*) ;;
    *) info "add ${install_dir} to PATH" ;;
  esac
}

main "$@"
