#!/bin/sh
set -eu

CODEX_VERSION="${CODEXVERSION:-latest}"
INSTALL_DIR="/usr/local/bin"

# ------------------------------------
# 1. Install dependencies
# ------------------------------------

install_dependencies() {
    has_compatible_ps() {
        ps_output=$(ps -p "$$" -o stat= -o lstart= 2>/dev/null) || return 1
        [ -n "$ps_output" ]
    }

    if command -v curl >/dev/null 2>&1 &&
       command -v tar >/dev/null 2>&1 &&
       [ -d /etc/ssl/certs ] &&
       has_compatible_ps; then
        return
    fi

    if command -v apt-get >/dev/null 2>&1; then
        export DEBIAN_FRONTEND=noninteractive
        apt-get update
        apt-get install -y --no-install-recommends \
            curl ca-certificates tar gzip procps

    elif command -v apk >/dev/null 2>&1; then
        apk add --no-cache \
            curl ca-certificates tar gzip procps

    elif command -v dnf >/dev/null 2>&1; then
        dnf install -y \
            curl ca-certificates tar gzip procps-ng

    elif command -v yum >/dev/null 2>&1; then
        yum install -y \
            curl ca-certificates tar gzip procps-ng

    else
        echo "ERROR: Unsupported package manager" >&2
        exit 1
    fi
}

# ------------------------------------
# 2. Detect architecture
# ------------------------------------

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)
            echo "x86_64"
            ;;
        aarch64|arm64)
            echo "aarch64"
            ;;
        *)
            echo "ERROR: Unsupported architecture: $(uname -m)" >&2
            exit 1
            ;;
    esac
}

# ------------------------------------
# 3. Download Codex
# ------------------------------------

install_codex() {
    ARCH="$(detect_arch)"

    TARGET="${ARCH}-unknown-linux-musl"
    ASSET="codex-${TARGET}.tar.gz"

    BASE_URL="https://github.com/openai/codex/releases"

    if [ "$CODEX_VERSION" = "latest" ]; then
        URL="${BASE_URL}/latest/download/${ASSET}"
    else
        URL="${BASE_URL}/download/${CODEX_VERSION}/${ASSET}"
    fi

    echo "Installing Codex..."
    echo "Architecture: ${ARCH}"
    echo "Version: ${CODEX_VERSION}"

    TMP_DIR="$(mktemp -d)"

    trap 'rm -rf "$TMP_DIR"' EXIT

    curl -fL --retry 3 \
        "$URL" \
        -o "${TMP_DIR}/${ASSET}"

    tar -xzf "${TMP_DIR}/${ASSET}" \
        -C "$TMP_DIR"

    install -m 755 \
        "${TMP_DIR}/codex-${TARGET}" \
        "${INSTALL_DIR}/codex"

    echo "Codex installed successfully:"
    "${INSTALL_DIR}/codex" --version
}

# ------------------------------------
# Main
# ------------------------------------

install_dependencies
install_codex
