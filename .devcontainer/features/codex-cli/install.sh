#!/usr/bin/env bash
set -euo pipefail

echo "Installing Codex..."

if ! command -v curl >/dev/null 2>&1; then
    apk add --no-cache curl
fi

su - "$_REMOTE_USER" -c 'curl -fsSL https://github.com/openai/codex/releases/latest/download/install.sh | sh'

mkdir -p "${_REMOTE_USER_HOME}/.codex"

chown -R "$_REMOTE_USER:$_REMOTE_USER" "${_REMOTE_USER_HOME}/.codex"

"${_REMOTE_USER_HOME}/.local/bin/codex" --version
