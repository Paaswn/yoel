#!/bin/sh
# Standalone piped installer for the v0.6.0 preview. Keep verification and PATH
# handling aligned with install.sh; never install this channel as "yoel".

set -euf

REPOSITORY="Paaswn/yoel"
DEFAULT_VERSION="v0.6.0-alpha.1"
DEFAULT_DOWNLOAD_BASE_URL="https://github.com/${REPOSITORY}/releases/download"

fail() {
	printf '%s\n' "yoel-preview installer: $*" >&2
	exit 1
}

case "$(uname -s)" in
	Darwin) os="darwin" ;;
	Linux) os="linux" ;;
	*) fail "unsupported operating system: $(uname -s)" ;;
esac

case "$(uname -m)" in
	x86_64|amd64) arch="amd64" ;;
	arm64|aarch64) arch="arm64" ;;
	*) fail "unsupported CPU architecture: $(uname -m)" ;;
esac

valid_preview_version() {
	case "$1" in
		v0.6.0-[0-9A-Za-z]*)
			case "$1" in *[!0-9A-Za-z.-]*) return 1 ;; esac
			return 0
			;;
		*) return 1 ;;
	esac
}

# This installer intentionally does not follow /releases/latest, which excludes
# prereleases. Overrides must stay on the v0.6.0 prerelease line.
version="${YOEL_VERSION:-$DEFAULT_VERSION}"
valid_preview_version "$version" || fail "expected a v0.6.0 prerelease tag such as $DEFAULT_VERSION"

download_base_url="$DEFAULT_DOWNLOAD_BASE_URL"
if [ "${YOEL_TEST_MODE:-}" = "1" ]; then
	download_base_url="${YOEL_RELEASE_DOWNLOAD_BASE_URL:-$download_base_url}"
fi

fetch() {
	if [ "${YOEL_TEST_MODE:-}" = "1" ]; then
		curl --fail --silent --show-error --location "$1" -o "$2"
	else
		curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 "$1" -o "$2"
	fi
}

install_dir="${YOEL_INSTALL_DIR:-$HOME/.local/bin}"
[ -n "$install_dir" ] || fail "install directory is empty"
archive="yoel_${version}_${os}_${arch}.tar.gz"

tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/yoel-preview-install.XXXXXX") || fail "could not create a temporary directory"
stage_path=""
cleanup() {
	rm -rf "$tmp_dir"
	[ -z "$stage_path" ] || rm -f "$stage_path"
}
trap cleanup EXIT HUP INT TERM

archive_path="$tmp_dir/$archive"
checksums_path="$tmp_dir/checksums.txt"
download_url="$download_base_url/$version"

fetch "$download_url/$archive" "$archive_path" || fail "could not download $archive"
fetch "$download_url/checksums.txt" "$checksums_path" || fail "could not download checksums.txt"

expected_sum=$(awk -v name="$archive" '$2 == name { print $1; exit }' "$checksums_path")
[ -n "$expected_sum" ] || fail "checksum for $archive is missing"
case "$expected_sum" in *[!0-9A-Fa-f]*) fail "invalid checksum file" ;; esac
[ "${#expected_sum}" -eq 64 ] || fail "invalid checksum file"

if command -v sha256sum >/dev/null 2>&1; then
	actual_sum=$(sha256sum "$archive_path" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
	actual_sum=$(shasum -a 256 "$archive_path" | awk '{print $1}')
else
	fail "SHA-256 tool not found (need sha256sum or shasum)"
fi
[ "$actual_sum" = "$expected_sum" ] || fail "checksum verification failed for $archive"

tar -tzf "$archive_path" >/dev/null 2>&1 || fail "downloaded archive is invalid"
if ! tar -tzf "$archive_path" | grep -Fx 'yoel' >/dev/null 2>&1; then
	fail "archive does not contain the expected yoel executable"
fi
tar -xOf "$archive_path" yoel > "$tmp_dir/yoel" || fail "could not extract yoel"
[ -f "$tmp_dir/yoel" ] || fail "archive did not extract the expected executable"

mkdir -p "$install_dir" || fail "could not create $install_dir"
stage_path=$(mktemp "$install_dir/.yoel-preview.new.XXXXXX") || fail "could not stage the executable"
cp "$tmp_dir/yoel" "$stage_path" || fail "could not stage the executable"
chmod 755 "$stage_path" || fail "could not mark yoel-preview executable"
mv -f "$stage_path" "$install_dir/yoel-preview" || fail "could not replace $install_dir/yoel-preview"
stage_path=""

path_contains_dir() {
	old_ifs=$IFS
	IFS=:
	for entry in $PATH; do
		[ "$entry" = "$install_dir" ] && IFS=$old_ifs && return 0
	done
	IFS=$old_ifs
	return 1
}

shell_quote() {
	printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\"'\"'/g")"
}

shell_name=$(basename "${SHELL:-}")
profile=""
if ! path_contains_dir; then
	case "$shell_name" in
		zsh) profile="$HOME/.zprofile" ;;
		bash)
			for candidate in "$HOME/.bash_profile" "$HOME/.bash_login" "$HOME/.profile"; do
				if [ -f "$candidate" ]; then profile="$candidate"; break; fi
			done
			[ -n "$profile" ] || profile="$HOME/.bash_profile"
			;;
	esac

	# Use the stable installer's marker so a shared bin directory is added once.
	path_line="export PATH=$(shell_quote "$install_dir"):\$PATH # yoel installer"
	if [ -n "$profile" ]; then
		if ! grep -Fqx "$path_line" "$profile" 2>/dev/null; then
			printf '\n%s\n' "$path_line" >> "$profile" || fail "could not update $profile"
		fi
		printf 'Added %s to PATH in %s. Open a new terminal, or run: . %s\n' "$install_dir" "$profile" "$profile"
	else
		printf 'Add this line to your shell profile, then open a new terminal:\n%s\n' "$path_line"
	fi
fi

printf 'Installed Yoel preview %s to %s\n' "$version" "$install_dir/yoel-preview"
printf 'Verify the installation with: yoel-preview --version\n'
printf 'The stable yoel executable was not changed. Install the VS Code extension separately.\n'
