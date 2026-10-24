#!/bin/sh
# Offline preview coverage: never contacts GitHub or changes the real user profile.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
installer="$root/scripts/install-preview.sh"
work=$(mktemp -d "${TMPDIR:-/tmp}/yoel-preview-test.XXXXXX")
cleanup() { rm -rf "$work"; }
trap cleanup EXIT HUP INT TERM

fake_bin="$work/fake-bin"
home_dir="$work/home"
install_dir="$work/install dir's"
mkdir -p "$fake_bin" "$home_dir" "$install_dir" "$work/tmp" "$work/releases"
cat > "$fake_bin/uname" <<'SH'
#!/bin/sh
case "$1" in
	-s) printf '%s' "${TEST_OS:-Linux}" ;;
	-m) printf '%s' "${TEST_ARCH:-x86_64}" ;;
esac
SH
cat > "$fake_bin/code" <<'SH'
#!/bin/sh
touch "$YOEL_TEST_LOG"
exit 99
SH
chmod 755 "$fake_bin/uname" "$fake_bin/code"

test_os=Linux
test_arch=x86_64
selected_version=""
stable_payload="stable untouched"
printf '%s\n' "$stable_payload" > "$install_dir/yoel"

make_release() {
	release_dir="$work/releases/$1"
	mkdir -p "$release_dir"
	printf '%s\n' "$4" > "$work/$5"
	chmod 755 "$work/$5"
	archive="yoel_$1_$2_$3.tar.gz"
	tar -C "$work" -czf "$release_dir/$archive" "$5"
	( cd "$release_dir" && sha256sum "$archive" > checksums.txt )
}

run_preview() {
	PATH="$fake_bin:$PATH" HOME="$home_dir" SHELL=/bin/zsh TMPDIR="$work/tmp" \
	TEST_OS="$test_os" TEST_ARCH="$test_arch" YOEL_VERSION="$selected_version" \
	YOEL_TEST_MODE=1 YOEL_RELEASE_DOWNLOAD_BASE_URL="file://$work/releases" \
	YOEL_INSTALL_DIR="$install_dir" YOEL_TEST_LOG="$work/extension-called" \
	sh "$installer"
}

assert_preserved() {
	[ "$(cat "$install_dir/yoel")" = "$stable_payload" ]
	[ ! -e "$work/extension-called" ]
	[ -z "$(find "$work/tmp" -mindepth 1 -maxdepth 1 -print -quit)" ]
	[ -z "$(find "$install_dir" -name '.yoel-preview.new.*' -print -quit)" ]
}

expect_failure() {
	if run_preview > "$work/failure.log" 2>&1; then
		printf '%s\n' "preview installer unexpectedly succeeded: $1" >&2
		exit 1
	fi
	grep -F "$1" "$work/failure.log" >/dev/null
	[ "$(cat "$install_dir/yoel-preview")" = 'latest preview' ]
	assert_preserved
}

make_release v0.6.0-alpha.1 linux amd64 'first preview' yoel
run_preview > "$work/success.log"
grep -F 'v0.6.0-alpha.1' "$work/success.log" >/dev/null
grep -F 'yoel-preview --version' "$work/success.log" >/dev/null
[ "$(cat "$install_dir/yoel-preview")" = 'first preview' ]
[ -x "$install_dir/yoel-preview" ]
[ "$(grep -Fc '# yoel installer' "$home_dir/.zprofile")" -eq 1 ]
assert_preserved

# A stable install into the shared directory must not replace the preview,
# and the two installers must not duplicate the shared PATH entry.
make_release v0.1.0 linux amd64 'stable installed' yoel
printf '%s\n' '{"tag_name":"v0.1.0"}' > "$work/latest.json"
PATH="$fake_bin:$PATH" HOME="$home_dir" SHELL=/bin/zsh TMPDIR="$work/tmp" \
YOEL_VERSION="" YOEL_TEST_MODE=1 YOEL_RELEASE_API_URL="file://$work/latest.json" \
YOEL_RELEASE_DOWNLOAD_BASE_URL="file://$work/releases" YOEL_INSTALL_DIR="$install_dir" \
sh "$root/scripts/install.sh" >/dev/null
stable_payload="stable installed"
[ "$(cat "$install_dir/yoel-preview")" = 'first preview' ]
[ "$(grep -Fc '# yoel installer' "$home_dir/.zprofile")" -eq 1 ]
assert_preserved

# Exercise each supported OS/CPU asset name without executing fixture binaries.
for platform in linux:Linux:x86_64:amd64 linux:Linux:aarch64:arm64 darwin:Darwin:x86_64:amd64 darwin:Darwin:arm64:arm64; do
	old_ifs=$IFS
	IFS=:
	set -- $platform
	IFS=$old_ifs
	test_os=$2
	test_arch=$3
	make_release v0.6.0-alpha.1 "$1" "$4" "preview $1 $4" yoel
	run_preview >/dev/null
	[ "$(cat "$install_dir/yoel-preview")" = "preview $1 $4" ]
	assert_preserved
done
test_os=Linux
test_arch=x86_64

# Explicit overrides stay on the v0.6.0 prerelease line.
make_release v0.6.0-beta.2 linux amd64 'latest preview' yoel
selected_version=v0.6.0-beta.2
run_preview >/dev/null
[ "$(cat "$install_dir/yoel-preview")" = 'latest preview' ]
[ "$(grep -Fc '# yoel installer' "$home_dir/.zprofile")" -eq 1 ]
# Verify the PATH entry correctly quotes the apostrophe in the directory.
profile_path=$(HOME="$home_dir" PATH=/usr/bin:/bin sh -c '. "$1"; printf "%s" "$PATH"' sh "$home_dir/.zprofile")
[ "$profile_path" = "$install_dir:/usr/bin:/bin" ]
assert_preserved

for invalid in v0.6.0 v0.5.3 v0.7.0-alpha.1 v0.6.0- 'v0.6.0-alpha/../../bad'; do
	selected_version=$invalid
	expect_failure 'expected a v0.6.0 prerelease'
done
selected_version=v0.6.0-beta.2

# Downloads and verification must fail without replacing either installed binary.
archive="yoel_v0.6.0-beta.2_linux_amd64.tar.gz"
release_dir="$work/releases/v0.6.0-beta.2"
printf '%064d  %s\n' 0 "$archive" > "$release_dir/checksums.txt"
expect_failure 'checksum verification failed'
printf 'not-a-checksum  %s\n' "$archive" > "$release_dir/checksums.txt"
expect_failure 'invalid checksum file'
printf 'abc  %s\n' "$archive" > "$release_dir/checksums.txt"
expect_failure 'invalid checksum file'
printf '' > "$release_dir/checksums.txt"
expect_failure 'checksum for'

printf 'not a tar file\n' > "$release_dir/$archive"
( cd "$release_dir" && sha256sum "$archive" > checksums.txt )
expect_failure 'downloaded archive is invalid'
make_release v0.6.0-beta.2 linux amd64 'wrong member' unexpected
expect_failure 'expected yoel executable'
rm "$release_dir/$archive"
expect_failure 'could not download'

test_os=Plan9
expect_failure 'unsupported operating system'
test_os=Linux
test_arch=mips
expect_failure 'unsupported CPU architecture'

printf '%s\n' 'install-preview.sh offline tests passed'
