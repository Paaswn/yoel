#!/bin/sh
# Local, offline coverage for scripts/install.sh. It never contacts GitHub.

set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
installer="$root/scripts/install.sh"
work=$(mktemp -d "${TMPDIR:-/tmp}/yoel-installer-test.XXXXXX")
cleanup() { rm -rf "$work"; }
trap cleanup EXIT HUP INT TERM

fake_bin="$work/fake-bin"
release_dir="$work/releases/v0.1.0"
home_dir="$work/home"
install_dir="$work/install directory"
mkdir -p "$fake_bin" "$release_dir" "$home_dir" "$work/tmp"

printf '%s\n' '#!/bin/sh' 'case "$1" in -s) printf Linux;; -m) printf x86_64;; esac' > "$fake_bin/uname"
chmod 755 "$fake_bin/uname"
printf '%s\n' '{"tag_name":"v0.1.0"}' > "$work/latest.json"

make_release() {
	payload=$1
	printf '%s\n' "$payload" > "$work/yoel"
	chmod 755 "$work/yoel"
	tar -C "$work" -czf "$release_dir/yoel_v0.1.0_linux_amd64.tar.gz" yoel
	( cd "$release_dir" && sha256sum yoel_v0.1.0_linux_amd64.tar.gz > checksums.txt )
}

make_arm_release() {
	printf '%s\n' 'arm binary' > "$work/yoel"
	chmod 755 "$work/yoel"
	tar -C "$work" -czf "$release_dir/yoel_v0.1.0_linux_arm64.tar.gz" yoel
	( cd "$release_dir" && sha256sum yoel_v0.1.0_linux_amd64.tar.gz yoel_v0.1.0_linux_arm64.tar.gz > checksums.txt )
}

run_installer() {
	PATH="$fake_bin:$PATH" HOME="$home_dir" SHELL=/bin/zsh TMPDIR="$work/tmp" \
	YOEL_TEST_MODE=1 YOEL_RELEASE_API_URL="file://$work/latest.json" \
	YOEL_RELEASE_DOWNLOAD_BASE_URL="file://$work/releases" YOEL_INSTALL_DIR="$install_dir" \
	sh "$installer"
}

make_release 'first binary'
run_installer >/dev/null
[ "$(cat "$install_dir/yoel")" = 'first binary' ]
[ "$(grep -Fc '# yoel installer' "$home_dir/.zprofile")" -eq 1 ]
[ -z "$(find "$work/tmp" -mindepth 1 -maxdepth 1 -name 'yoel-install.*' -print -quit)" ]

make_release 'upgraded binary'
run_installer >/dev/null
[ "$(cat "$install_dir/yoel")" = 'upgraded binary' ]
[ "$(grep -Fc '# yoel installer' "$home_dir/.zprofile")" -eq 1 ]

make_arm_release
printf '%s\n' '#!/bin/sh' 'case "$1" in -s) printf Linux;; -m) printf aarch64;; esac' > "$fake_bin/uname"
chmod 755 "$fake_bin/uname"
arm_install_dir="$work/arm install"
PATH="$fake_bin:$PATH" HOME="$home_dir" SHELL=/bin/zsh TMPDIR="$work/tmp" \
YOEL_TEST_MODE=1 YOEL_RELEASE_API_URL="file://$work/latest.json" \
YOEL_RELEASE_DOWNLOAD_BASE_URL="file://$work/releases" YOEL_INSTALL_DIR="$arm_install_dir" \
sh "$installer" >/dev/null
[ "$(cat "$arm_install_dir/yoel")" = 'arm binary' ]

printf '%064d  %s\n' 0 yoel_v0.1.0_linux_amd64.tar.gz > "$release_dir/checksums.txt"
if run_installer >/dev/null 2>&1; then
	printf '%s\n' 'checksum mismatch unexpectedly succeeded' >&2
	exit 1
fi
[ "$(cat "$install_dir/yoel")" = 'upgraded binary' ]

printf '%s\n' '#!/bin/sh' 'case "$1" in -s) printf Plan9;; -m) printf x86_64;; esac' > "$fake_bin/uname"
chmod 755 "$fake_bin/uname"
if run_installer >/dev/null 2>&1; then
	printf '%s\n' 'unsupported OS unexpectedly succeeded' >&2
	exit 1
fi
[ "$(cat "$install_dir/yoel")" = 'upgraded binary' ]

printf '%s\n' 'install.sh offline tests passed'
