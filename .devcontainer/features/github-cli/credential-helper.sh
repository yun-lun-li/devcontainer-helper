#!/bin/sh
set -eu

[ "${1:-}" = get ] || exit 0
protocol=
host=
while IFS= read -r line && [ -n "$line" ]; do
    case "$line" in
        protocol=*) protocol=${line#protocol=} ;;
        host=*) host=${line#host=} ;;
    esac
done
[ "$protocol" = https ] && [ "$host" = github.com ] || exit 0
if ! token=$(curl --fail --silent --show-error --max-time 125 --unix-socket /run/dch-github-auth/pass.sock http://localhost/token); then
    echo 'Cannot read GitHub PAT from host pass. Unlock host GPG and retry.' >&2
    exit 1
fi
[ -n "$token" ] || exit 1
printf 'username=x-access-token\npassword=%s\n\n' "$token"
