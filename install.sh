#!/bin/sh
# hax installer for macOS and Linux.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/BTCBap/hax/main/install.sh | sh
#   sh install.sh [--desktop] [--no-sudo]
#   sh install.sh --uninstall
#
# Environment:
#   HAX_VERSION      release tag to install (for example v1.4.0)
#   HAX_INSTALL_DIR  directory to install hax into
#   HAX_NO_SUDO=1    always use ~/.local/bin instead of /usr/local/bin
#   HAX_DESKTOP=1    install a Linux desktop entry (same as --desktop)
#   HAX_SYSTEM_DIR   system install directory (default /usr/local/bin; advanced)
#
# Without HAX_INSTALL_DIR the installer uses /usr/local/bin when it is
# writable. Otherwise, with sudo and a terminal, it asks for your password;
# without a terminal it tries passwordless sudo. If that does not work it
# installs to ~/.local/bin.
#
# The script only writes the hax binary, an optional desktop entry, and
# shell PATH advice. It never reads or changes wallet data in ~/.hax.

set -eu

REPO="BTCBap/hax"
RAW_URL="https://raw.githubusercontent.com/${REPO}/main"
RELEASES_URL="https://github.com/${REPO}/releases"
RELEASE_DOWNLOAD_URL="${RELEASES_URL}/download"
LATEST_API_URL="https://api.github.com/repos/${REPO}/releases/latest"
COSIGN_IDENTITY="https://github.com/${REPO}/.github/workflows/release.yml"

say() {
	printf '%s\n' "$*"
}

warn() {
	printf 'warning: %s\n' "$*" >&2
}

die() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

usage() {
	cat <<EOF
hax installer (macOS and Linux)

Usage:
  sh install.sh [options]

Options:
  --desktop    Install a Linux desktop entry (~/.local/share/applications)
  --no-sudo    Install into ~/.local/bin even if /usr/local/bin is writable
  --uninstall  Remove the hax binary and the desktop entry
  --help       Show this help

Environment:
  HAX_VERSION      Release to install, for example v1.4.0
  HAX_INSTALL_DIR  Directory to install hax into
  HAX_NO_SUDO=1    Same as --no-sudo
  HAX_DESKTOP=1    Same as --desktop
  HAX_SYSTEM_DIR   System install directory (default /usr/local/bin; advanced)

Without HAX_INSTALL_DIR the installer uses /usr/local/bin when it is
writable. Otherwise, when sudo and a terminal are available, it asks for
your password to install there; otherwise it tries passwordless sudo. If
neither works it installs to ~/.local/bin and prints PATH advice. Use
--no-sudo to install to ~/.local/bin without asking for administrator
rights.

Download first if you want to read the script:
  curl -fsSLO ${RAW_URL}/install.sh
  less install.sh
  sh install.sh
EOF
}

# ---------------------------------------------------------------------------
# Options and cleanup
# ---------------------------------------------------------------------------

install_dir_env=${HAX_INSTALL_DIR:-}
no_sudo=0
want_desktop=0
want_uninstall=0

if [ "${HAX_NO_SUDO:-0}" = "1" ]; then
	no_sudo=1
fi
if [ "${HAX_DESKTOP:-0}" = "1" ]; then
	want_desktop=1
fi

while [ $# -gt 0 ]; do
	case "$1" in
	--help | -h)
		usage
		exit 0
		;;
	--uninstall)
		want_uninstall=1
		;;
	--desktop)
		want_desktop=1
		;;
	--no-sudo)
		no_sudo=1
		;;
	*)
		die "unknown option '$1' (try --help)"
		;;
	esac
	shift
done

tmp_dir=""
cleanup() {
	if [ -n "$tmp_dir" ] && [ -d "$tmp_dir" ]; then
		rm -rf "$tmp_dir"
	fi
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

# ---------------------------------------------------------------------------
# Download helpers
# ---------------------------------------------------------------------------

if command -v curl >/dev/null 2>&1; then
	downloader=curl
elif command -v wget >/dev/null 2>&1; then
	downloader=wget
else
	die "curl or wget is required"
fi

# download URL OUTPUT
download() {
	dl_url=$1
	dl_out=$2
	if [ "$downloader" = "curl" ]; then
		curl -fsSL --retry 3 --retry-delay 1 -o "$dl_out" "$dl_url"
	else
		wget -q -O "$dl_out" "$dl_url"
	fi
}

# download_stdout URL
download_stdout() {
	dl_url=$1
	if [ "$downloader" = "curl" ]; then
		curl -fsSL --retry 3 --retry-delay 1 "$dl_url"
	else
		wget -qO- "$dl_url"
	fi
}

# ---------------------------------------------------------------------------
# Platform detection
# ---------------------------------------------------------------------------

detect_platform() {
	os_name=$(uname -s 2>/dev/null || true)
	case "$os_name" in
	Darwin)
		os=darwin
		;;
	Linux)
		os=linux
		;;
	*)
		die "unsupported operating system '${os_name:-unknown}'; hax releases support macOS and Linux (Windows: use install.ps1)"
		;;
	esac

	arch_name=$(uname -m 2>/dev/null || true)
	case "$arch_name" in
	x86_64 | amd64)
		arch=amd64
		;;
	arm64 | aarch64)
		arch=arm64
		;;
	*)
		die "unsupported architecture '${arch_name:-unknown}'; hax releases support amd64 and arm64"
		;;
	esac
}

# ---------------------------------------------------------------------------
# Version resolution
# ---------------------------------------------------------------------------

resolve_version() {
	if [ -n "${HAX_VERSION:-}" ]; then
		version=$HAX_VERSION
	else
		say "Looking up the latest hax release..."
		latest_json=$(download_stdout "$LATEST_API_URL") ||
			die "could not reach the GitHub API; set HAX_VERSION to install a specific release"
		version=$(printf '%s\n' "$latest_json" |
			sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' |
			head -n 1)
		[ -n "$version" ] || die "could not find the latest release tag; set HAX_VERSION instead"
	fi

	# Accept both 1.4.0 and v1.4.0.
	case "$version" in
	v[0-9]*) ;;
	[0-9]*) version="v$version" ;;
	*) die "invalid HAX_VERSION '$version'; expected a tag such as v1.4.0" ;;
	esac

	version_number=${version#v}
	case "$version_number" in
	"" | *[!0-9A-Za-z._-]*)
		die "invalid HAX_VERSION '$version'; expected a tag such as v1.4.0"
		;;
	esac
}

# ---------------------------------------------------------------------------
# Checksum and signature verification
# ---------------------------------------------------------------------------

verify_checksum() {
	checksum_file=$1
	archive_file=$2
	archive_name=$3

	expected=$(awk -v name="$archive_name" '$2 == name { print $1; exit }' "$checksum_file")
	[ -n "$expected" ] || die "checksums.txt has no entry for $archive_name"

	if command -v sha256sum >/dev/null 2>&1; then
		actual=$(sha256sum "$archive_file" | awk '{ print $1 }')
	elif command -v shasum >/dev/null 2>&1; then
		actual=$(shasum -a 256 "$archive_file" | awk '{ print $1 }')
	else
		die "sha256sum or shasum is required to verify the download"
	fi

	if [ "$expected" != "$actual" ]; then
		die "checksum mismatch for $archive_name (expected $expected, got $actual)"
	fi
	say "Checksum verified: $archive_name"
}

verify_signature() {
	sigstore_file="$tmp_dir/checksums.txt.sigstore.json"

	if ! command -v cosign >/dev/null 2>&1; then
		say "cosign is not installed; skipping the optional checksums.txt signature check."
		return 0
	fi

	say "Verifying checksums.txt with cosign..."
	download "${RELEASE_DOWNLOAD_URL}/${version}/checksums.txt.sigstore.json" "$sigstore_file" ||
		die "could not download checksums.txt.sigstore.json"
	if ! cosign verify-blob \
		--bundle "$sigstore_file" \
		--certificate-identity "${COSIGN_IDENTITY}@refs/tags/${version}" \
		--certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
		"$tmp_dir/checksums.txt"; then
		die "cosign could not verify checksums.txt"
	fi
	say "Signature verified: checksums.txt"
}

# ---------------------------------------------------------------------------
# Install directory selection
# ---------------------------------------------------------------------------

system_dir=${HAX_SYSTEM_DIR:-/usr/local/bin}
have_tty=0

# A controlling terminal is needed both to ask sudo for a password and to
# read one. /dev/tty works under `curl | sh`, where stdin is the pipe.
detect_tty() {
	if ( : </dev/tty >/dev/tty ) 2>/dev/null; then
		have_tty=1
	fi
}

tty_say() {
	if [ "$have_tty" -eq 1 ]; then
		printf '%s\n' "$*" >/dev/tty
	else
		printf '%s\n' "$*" >&2
	fi
}

# can_install_without_sudo DIR
can_install_without_sudo() {
	check_dir=$1
	while [ ! -e "$check_dir" ] && [ "$check_dir" != "/" ] && [ "$check_dir" != "." ]; do
		check_dir=$(dirname "$check_dir")
	done
	[ -d "$check_dir" ] && [ -w "$check_dir" ]
}

run_sudo() {
	if [ "$have_tty" -eq 1 ]; then
		# The redirect is intentional: sudo gets the terminal as stdin, not
		# the script's stdin, which can be the pipe for `curl | sh`.
		# shellcheck disable=SC2024
		sudo "$@" </dev/tty
	else
		sudo -n "$@"
	fi
}

choose_install_dir() {
	use_sudo=0

	if [ -n "$install_dir_env" ]; then
		install_dir=$install_dir_env
		return
	fi

	if [ "$no_sudo" -eq 1 ]; then
		install_dir="$HOME/.local/bin"
		return
	fi

	if can_install_without_sudo "$system_dir"; then
		install_dir=$system_dir
		return
	fi

	if command -v sudo >/dev/null 2>&1; then
		if [ "$have_tty" -eq 1 ]; then
			tty_say "hax: installing to $system_dir needs administrator rights; sudo will ask for your password (use --no-sudo to install to ~/.local/bin instead)."
			# sudo reads the password from the terminal, which also works
			# under `curl | sh` where stdin is the pipe.
			# shellcheck disable=SC2024
			if sudo -v </dev/tty; then
				install_dir=$system_dir
				use_sudo=1
				return
			fi
			tty_say "hax: sudo was not granted; installing to $HOME/.local/bin instead."
			install_dir="$HOME/.local/bin"
			return
		fi
		if sudo -n true 2>/dev/null; then
			install_dir=$system_dir
			use_sudo=1
			return
		fi
	fi

	install_dir="$HOME/.local/bin"
}

prepare_install_dir() {
	if [ -d "$install_dir" ]; then
		if [ ! -w "$install_dir" ] && [ "$use_sudo" -eq 0 ]; then
			die "install directory '$install_dir' is not writable"
		fi
		return
	fi

	if [ "$use_sudo" -eq 1 ]; then
		run_sudo mkdir -p "$install_dir" || die "could not create $install_dir"
	else
		mkdir -p "$install_dir"
	fi

	if [ "$use_sudo" -eq 0 ] && [ ! -w "$install_dir" ]; then
		die "install directory '$install_dir' is not writable"
	fi
}

# ---------------------------------------------------------------------------
# Binary and desktop entry install
# ---------------------------------------------------------------------------

install_binary() {
	extract_dir="$tmp_dir/extract"
	mkdir -p "$extract_dir"
	tar -xzf "$tmp_dir/$archive_name" -C "$extract_dir" hax ||
		die "could not extract $archive_name"
	[ -f "$extract_dir/hax" ] || die "$archive_name does not contain hax"

	target="$install_dir/hax"
	old_version=""
	if [ -x "$target" ]; then
		old_version=$("$target" --version 2>/dev/null |
			sed -n 's/.*\(v[0-9][0-9A-Za-z._-]*\).*/\1/p' |
			head -n 1) || old_version=""
	fi

	# Write to a temporary name in the same directory, then rename, so an
	# existing running binary is never left half written.
	tmp_target="$install_dir/.hax.tmp.$$"
	if [ "$use_sudo" -eq 1 ]; then
		run_sudo install -m 0755 "$extract_dir/hax" "$tmp_target" ||
			die "could not write $tmp_target"
		run_sudo mv -f "$tmp_target" "$target" || die "could not replace $target"
	else
		install -m 0755 "$extract_dir/hax" "$tmp_target" ||
			die "could not write $tmp_target"
		mv -f "$tmp_target" "$target" || die "could not replace $target"
	fi

	if [ "$os" = "darwin" ]; then
		# The binary came from the internet, so clear the quarantine flag.
		xattr -d com.apple.quarantine "$target" 2>/dev/null || true
	fi

	if [ "$old_version" = "$version" ]; then
		say "hax $version is already up to date at $target"
	elif [ -n "$old_version" ]; then
		say "Updated hax from $old_version to $version at $target"
	else
		say "Installed hax $version to $target"
	fi
}

install_desktop_entry() {
	if [ "$os" != "linux" ]; then
		warn "--desktop is only supported on Linux; skipping the desktop entry"
		return 0
	fi

	desktop_dir="$HOME/.local/share/applications"
	desktop_file="$desktop_dir/hax.desktop"
	mkdir -p "$desktop_dir"
	cat >"$desktop_file.tmp.$$" <<EOF
[Desktop Entry]
Type=Application
Name=hax
Comment=Terminal Bitcoin wallet
Exec="$install_dir/hax"
Terminal=true
Categories=Finance;Utility;
EOF
	chmod 0644 "$desktop_file.tmp.$$"
	mv -f "$desktop_file.tmp.$$" "$desktop_file"
	say "Installed desktop entry: $desktop_file"
}

print_path_advice() {
	case ":${PATH:-}:" in
	*":$install_dir:"*)
		return 0
		;;
	esac

	say ""
	say "$install_dir is not on your PATH."
	say "Add it with one of these commands, then open a new terminal:"
	printf "  bash: echo 'export PATH=\"%s:\$PATH\"' >> ~/.bashrc\n" "$install_dir"
	printf "  zsh:  echo 'export PATH=\"%s:\$PATH\"' >> ~/.zshrc\n" "$install_dir"
	printf "  fish: fish_add_path \"%s\"\n" "$install_dir"
	say "A new terminal is needed for the change to take effect."
}

# ---------------------------------------------------------------------------
# Uninstall
# ---------------------------------------------------------------------------

remove_installed_file() {
	remove_target=$1
	if [ ! -e "$remove_target" ] && [ ! -L "$remove_target" ]; then
		return
	fi

	remove_parent=$(dirname "$remove_target")
	if [ -w "$remove_parent" ]; then
		rm -f "$remove_target"
	elif command -v sudo >/dev/null 2>&1; then
		run_sudo rm -f "$remove_target" || {
			warn "cannot remove $remove_target (permission denied); remove it manually"
			return
		}
	else
		warn "cannot remove $remove_target (permission denied); remove it manually"
		return
	fi
	say "Removed $remove_target"
}

uninstall_hax() {
	say "Removing the hax binary and desktop entry."
	say "Wallet data in $HOME/.hax is not touched."

	seen=""
	for dir in "$install_dir_env" "$system_dir" "$HOME/.local/bin"; do
		[ -n "$dir" ] || continue
		case " $seen " in
		*" $dir "*) continue ;;
		esac
		seen="$seen $dir"
		remove_installed_file "$dir/hax"
	done

	desktop_file="$HOME/.local/share/applications/hax.desktop"
	if [ -e "$desktop_file" ]; then
		rm -f "$desktop_file"
		say "Removed $desktop_file"
	fi

	say "Uninstall finished. Open a new terminal if PATH still lists a removed directory."
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

detect_tty

if [ "$want_uninstall" -eq 1 ]; then
	uninstall_hax
	exit 0
fi

detect_platform
resolve_version
choose_install_dir

say "hax installer"
say "  version:    $version"
say "  platform:   $os/$arch"
say "  install to: $install_dir/hax"
if [ "$want_desktop" -eq 1 ]; then
	say "  desktop:    $HOME/.local/share/applications/hax.desktop"
fi
say ""

archive_name="hax_${version_number}_${os}_${arch}.tar.gz"
tmp_dir=$(mktemp -d 2>/dev/null || mktemp -d -t hax-install)
trap cleanup EXIT

say "Downloading $archive_name..."
download "${RELEASE_DOWNLOAD_URL}/${version}/${archive_name}" "$tmp_dir/$archive_name" ||
	die "could not download $archive_name (does release $version have a $os/$arch build?)"
download "${RELEASE_DOWNLOAD_URL}/${version}/checksums.txt" "$tmp_dir/checksums.txt" ||
	die "could not download checksums.txt"

verify_checksum "$tmp_dir/checksums.txt" "$tmp_dir/$archive_name" "$archive_name"
verify_signature

prepare_install_dir
install_binary

if [ "$want_desktop" -eq 1 ]; then
	install_desktop_entry
fi

print_path_advice
say ""
say "Installed version:"
"$install_dir/hax" --version || die "the installed binary did not run"
say ""
say "Run hax by typing: hax"
