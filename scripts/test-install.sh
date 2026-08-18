#!/bin/sh
set -eu

root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' 0
trap 'exit 1' HUP INT TERM

tag="v0.0.0-test"
version="${tag#v}"

case "$(uname -s)" in
  Linux) os=linux; binary=youtrack-tui ;;
  Darwin) os=darwin; binary=youtrack-tui ;;
  MINGW* | MSYS* | CYGWIN*) os=windows; binary=youtrack-tui.exe ;;
  *) exit 0 ;;
esac

case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) exit 0 ;;
esac

release="${tmp}/${tag}"
payload="${tmp}/payload"
install_dir="${tmp}/bin"
archive="youtrack-tui-${version}-${os}-${arch}.tar.gz"
mkdir -p "$release" "$payload"

cat > "${payload}/${binary}" <<'EOF'
#!/bin/sh
printf 'installed\n'
EOF
chmod 755 "${payload}/${binary}"
tar -czf "${release}/${archive}" -C "$payload" "$binary"

if command -v sha256sum >/dev/null 2>&1; then
  sum="$(sha256sum "${release}/${archive}" | awk '{print $1}')"
else
  sum="$(shasum -a 256 "${release}/${archive}" | awk '{print $1}')"
fi
printf '%s  %s\n' "$sum" "$archive" > "${release}/checksums.txt"
printf '{"tag_name":"%s"}\n' "$tag" > "${tmp}/latest.json"

YOUTRACK_TUI_API_URL="file://${tmp}/latest.json" \
YOUTRACK_TUI_RELEASE_URL="file://${tmp}" \
INSTALL_DIR="$install_dir" \
  sh "${root}/install.sh" >/dev/null

[ "$("${install_dir}/${binary}")" = installed ]
printf 'installer smoke test passed\n'
