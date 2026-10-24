#!/bin/sh
set -eu

REPO="yun-lun-li/devcontainer-helper"
VERSION=${DCH_VERSION:-latest}
PREFIX=${DCH_PREFIX:-"$HOME/.local"}
INSTALL_DIR=${DCH_INSTALL_DIR:-"$PREFIX/bin"}
DEVCONTAINER_INSTALLER_URL=https://raw.githubusercontent.com/devcontainers/cli/main/scripts/install.sh

log() {
	printf '[dch] %s\n' "$1"
}

fail() {
	printf '[dch] ERROR: %s\n' "$1" >&2
	exit 1
}

usage() {
	cat <<'EOF'
Install dch and the Dev Container CLI.

Usage: sh install.sh

Environment variables:
  DCH_VERSION       dch release tag to install (default: latest)
  DCH_PREFIX        installation prefix (default: ~/.local)
  DCH_INSTALL_DIR   directory for the dch binary (default: ~/.local/bin)
EOF
}

download() {
	url=$1
	output=$2
	if [ "$DOWNLOADER" = curl ]; then
		curl -fsSL --retry 3 "$url" -o "$output"
	else
		wget --tries=3 -q -O "$output" "$url"
	fi
}

detect_os() {
	case "$(uname -s)" in
		Linux*) echo linux ;;
		Darwin*) echo darwin ;;
		*) fail "Unsupported operating system: $(uname -s)" ;;
	esac
}

detect_arch() {
	case "$(uname -m)" in
		x86_64|amd64) echo amd64 ;;
		arm64|aarch64) echo arm64 ;;
		*) fail "Unsupported architecture: $(uname -m)" ;;
	esac
}

install_devcontainer_cli() {
	installer="$TMP_DIR/devcontainer-install.sh"
	log "Installing Dev Container CLI first"
	download "$DEVCONTAINER_INSTALLER_URL" "$installer"
	if sh "$installer" --prefix "$PREFIX" 2>"$TMP_DIR/devcontainer-install.stderr"; then
		:
	else
		cat "$TMP_DIR/devcontainer-install.stderr" >&2
		fail "Dev Container CLI installation failed"
	fi
}

install_dch() {
	os=$(detect_os)
	arch=$(detect_arch)
	if [ "$VERSION" = latest ]; then
		log "Resolving latest dch release"
		if [ "$DOWNLOADER" = curl ]; then
			release_json=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest")
		else
			release_json=$(wget -qO- "https://api.github.com/repos/${REPO}/releases/latest")
		fi
		VERSION=$(printf '%s\n' "$release_json" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | sed -n '1p')
		[ -n "$VERSION" ] || fail "Could not determine the latest dch release"
	fi
	archive="dch-${VERSION}-${os}-${arch}.tar.gz"
	url="https://github.com/${REPO}/releases/download/${VERSION}/${archive}"

	log "Downloading dch ${VERSION} for ${os}-${arch}"
	download "$url" "$TMP_DIR/$archive"
	tar -xzf "$TMP_DIR/$archive" -C "$TMP_DIR"

	# Release archives contain a versioned top-level directory.
	set -- "$TMP_DIR"/dch-*-"$os"-"$arch"/dch
	[ -f "$1" ] || fail "Downloaded archive does not contain the dch binary"
	mkdir -p "$INSTALL_DIR"
	cp "$1" "$INSTALL_DIR/dch"
	chmod 755 "$INSTALL_DIR/dch"
	log "Installed dch to $INSTALL_DIR/dch"
}

setup_config() {
	config_dir="$HOME/.dch"
	mkdir -p "$config_dir"
	if [ ! -f "$config_dir/config" ]; then
		printf '{\n  "version": 1,\n  "projects": []\n}\n' > "$config_dir/config"
		log "Created default configuration at $config_dir/config"
	fi
}

setup_path() {
	case "${SHELL:-}" in
		*/bash) profile="$HOME/.bashrc" ;;
		*/zsh) profile="$HOME/.zshrc" ;;
		*) profile="$HOME/.profile" ;;
	esac
	marker='# >>> dch PATH >>>'
	mkdir -p "$HOME"
	if [ -f "$profile" ] && grep -F "$marker" "$profile" >/dev/null 2>&1; then
		return
	fi
	{
		printf '\n%s\n' "$marker"
		printf 'case ":$PATH:" in\n'
		printf '  *":%s:"*) ;;\n' "$INSTALL_DIR"
		printf '  *) PATH="%s:$PATH" ;;\n' "$INSTALL_DIR"
		printf 'esac\nexport PATH\n'
		printf '# <<< dch PATH <<<\n'
	} >> "$profile"
	log "Added $INSTALL_DIR to PATH in $profile"
}

main() {
	case "${1:-}" in
		-h|--help) usage; exit 0 ;;
		"") ;;
		*) fail "Unknown argument: $1 (see --help)" ;;
	esac

	if command -v curl >/dev/null 2>&1; then
		DOWNLOADER=curl
	elif command -v wget >/dev/null 2>&1; then
		DOWNLOADER=wget
	else
		fail "curl or wget is required"
	fi
	command -v tar >/dev/null 2>&1 || fail "tar is required"
	command -v uname >/dev/null 2>&1 || fail "uname is required"

	TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/dch-install.XXXXXX") || fail "Could not create temporary directory"
	trap 'rm -rf "$TMP_DIR"' 0 HUP INT TERM

	install_devcontainer_cli
	install_dch
	setup_config
	setup_path
	log "Installation completed"
	"$INSTALL_DIR/dch" help >/dev/null
	printf '\nRestart your shell, then run:\n  dch --help\n'
}

main "$@"
