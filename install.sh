#!/bin/sh
# Installs standup from GitHub Releases on macOS and Linux:
#   curl -fsSL https://raw.githubusercontent.com/jjinendra3/standup/master/install.sh | sh
# STANDUP_VERSION picks a release (default: the latest). STANDUP_INSTALL_DIR picks the folder (default: ~/.local/bin).
set -eu

repo="jjinendra3/standup"
install_dir="${STANDUP_INSTALL_DIR:-$HOME/.local/bin}"

fail() {
	echo "standup install: $*" >&2
	exit 1
}

case "$(uname -s)" in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) fail "unsupported system $(uname -s). On Windows, download the zip from https://github.com/$repo/releases/latest" ;;
esac

case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) fail "unsupported CPU $(uname -m)" ;;
esac

version="${STANDUP_VERSION:-}"
if [ -z "$version" ]; then
	# /releases/latest redirects to /releases/tag/<tag>, which avoids the GitHub API rate limit.
	latest=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest") ||
		fail "could not look up the latest release at https://github.com/$repo/releases/latest"
	version="${latest##*/}"
fi
case "$version" in
v[0-9]*) ;;
[0-9]*) version="v$version" ;;
*) fail "no release found at https://github.com/$repo/releases" ;;
esac

# Matches archives.name_template in .goreleaser.yaml, which drops the v.
archive="standup_${version#v}_${os}_${arch}.tar.gz"
base="https://github.com/$repo/releases/download/$version"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading standup $version for $os/$arch"
curl -fsSL -o "$tmp/$archive" "$base/$archive" || fail "could not download $base/$archive"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || fail "could not download $base/checksums.txt"

expected=$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")
[ -n "$expected" ] || fail "checksums.txt has no entry for $archive"
if command -v sha256sum >/dev/null 2>&1; then
	actual=$(sha256sum "$tmp/$archive" | awk '{ print $1 }')
else
	actual=$(shasum -a 256 "$tmp/$archive" | awk '{ print $1 }')
fi
[ "$actual" = "$expected" ] || fail "checksum mismatch for $archive"

tar -xzf "$tmp/$archive" -C "$tmp" standup
mkdir -p "$install_dir"
chmod 0755 "$tmp/standup"
mv -f "$tmp/standup" "$install_dir/standup"
echo "Installed standup $version to $install_dir/standup"

case ":$PATH:" in
*":$install_dir:"*) ;;
*)
	echo
	echo "$install_dir is not on your PATH. Add this line to your shell profile, such as ~/.zshrc:"
	echo "  export PATH=\"$install_dir:\$PATH\""
	;;
esac

echo
echo "Run standup setup to choose your agent and repo folders."
