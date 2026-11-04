#!/bin/sh
set -eu

if ! command -v curl >/dev/null 2>&1; then
    echo 'The GitHub host-pass Feature requires curl in the container.' >&2
    exit 1
fi
if ! command -v git >/dev/null 2>&1; then
    echo 'The GitHub host-pass Feature requires git in the container.' >&2
    exit 1
fi

install -d /usr/local/lib/dch-github-auth /usr/local/bin
install -m 0755 gh-wrapper.sh /usr/local/lib/dch-github-auth/gh
install -m 0755 credential-helper.sh /usr/local/bin/git-credential-dch-pass

real_gh=$(command -v gh)
case "$real_gh" in
    /usr/local/bin/gh)
        if [ ! -L "$real_gh" ] || [ "$(readlink "$real_gh")" != /usr/local/lib/dch-github-auth/gh ]; then
            install -m 0755 "$real_gh" /usr/local/lib/dch-github-auth/gh-real
        fi
        ;;
    '')
        echo 'The GitHub CLI dependency did not install gh.' >&2
        exit 1
        ;;
    *)
        ln -sf "$real_gh" /usr/local/lib/dch-github-auth/gh-real
        ;;
esac
ln -sf /usr/local/lib/dch-github-auth/gh /usr/local/bin/gh

git config --system --unset-all 'credential.https://github.com.helper' || :
git config --system --add 'credential.https://github.com.helper' ''
git config --system --add 'credential.https://github.com.helper' dch-pass
