#!/bin/sh
set -eu

socket=/run/dch-github-auth/pass.sock
if ! token=$(curl --fail --silent --show-error --max-time 125 --unix-socket "$socket" http://localhost/token); then
    echo 'Cannot read GitHub PAT from host pass. Start the project with dch; unlock host GPG and retry.' >&2
    exit 1
fi
if [ -z "$token" ]; then
    echo 'Host pass returned an empty GitHub PAT.' >&2
    exit 1
fi
# Set both recognized variables so inherited credentials cannot override host pass.
GH_HOST=github.com GH_TOKEN="$token" GITHUB_TOKEN="$token" exec /usr/local/lib/dch-github-auth/gh-real "$@"
