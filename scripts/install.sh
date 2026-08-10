#!/bin/sh
# Install a verified Yoel release for the current macOS or Linux user.

set -euf

REPOSITORY="Paaswn/yoel"
DEFAULT_RELEASE_API_URL="https://api.github.com/repos/${REPOSITORY}/releases/latest"
DEFAULT_DOWNLOAD_BASE_URL="https://github.com/${REPOSITORY}/releases/download"

fail() {
	printf '%s\n' "yoel installer: $*" >&2
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

valid_version() {
	case "$1" in
		v[0-9]* )
			case "$1" in *[!0-9A-Za-z._-]*) return 1 ;; esac
			return 0
			;;
		*) return 1 ;;
	esac
}

release_api_url="$DEFAULT_RELEASE_API_URL"
download_base_url="$DEFAULT_DOWNLOAD_BASE_URL"
if [ "${YOEL_TEST_MODE:-}" = "1" ]; then
	# These overrides are deliberately test-only, so normal installs always use
	# the audited GitHub repository above.
	release_api_url="${YOEL_RELEASE_API_URL:-$release_api_url}"
	download_base_url="${YOEL_RELEASE_DOWNLOAD_BASE_URL:-$download_base_url}"
fi

fetch() {
	if [ "${YOEL_TEST_MODE:-}" = "1" ]; then
		curl --fail --silent --show-error --location "$1" -o "$2"
	else
		curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 "$1" -o "$2"
	fi
}

fetch_stdout() {
	if [ "${YOEL_TEST_MODE:-}" = "1" ]; then
		curl --fail --silent --show-error --location "$1"
	else
		curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 "$1"
	fi
}

if [ -n "${YOEL_VERSION:-}" ]; then
	version="$YOEL_VERSION"
else
	metadata=$(fetch_stdout "$release_api_url") || \
		fail "could not find the latest release; see https://github.com/${REPOSITORY}/releases"
	version=$(printf '%s\n' "$metadata" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | sed -n '1p')
fi

valid_version "${version:-}" || fail "invalid release version; see https://github.com/${REPOSITORY}/releases"

install_dir="${YOEL_INSTALL_DIR:-$HOME/.local/bin}"
[ -n "$install_dir" ] || fail "install directory is empty"
archive="yoel_${version}_${os}_${arch}.tar.gz"

tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/yoel-install.XXXXXX") || fail "could not create a temporary directory"
cleanup() { rm -rf "$tmp_dir"; }
trap cleanup EXIT HUP INT TERM

archive_path="$tmp_dir/$archive"
checksums_path="$tmp_dir/checksums.txt"
download_url="$download_base_url/$version"

fetch "$download_url/$archive" "$archive_path" || fail "could not download $archive"
fetch "$download_url/checksums.txt" "$checksums_path" || fail "could not download checksums.txt"

expected_sum=$(awk -v name="$archive" '$2 == name { print $1; exit }' "$checksums_path")
[ -n "$expected_sum" ] || fail "checksum for $archive is missing"
case "$expected_sum" in *[!0-9A-Fa-f]*|?????????????????????????????????????????????????????????????????) fail "invalid checksum file" ;; esac

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
stage_path=$(mktemp "$install_dir/.yoel.new.XXXXXX") || fail "could not stage the executable"
cp "$tmp_dir/yoel" "$stage_path" || fail "could not stage the executable"
chmod 755 "$stage_path" || fail "could not mark yoel executable"
mv -f "$stage_path" "$install_dir/yoel" || fail "could not replace $install_dir/yoel"

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

printf 'Installed Yoel to %s\n' "$install_dir/yoel"
printf 'Verify the installation with: yoel --help\n'
