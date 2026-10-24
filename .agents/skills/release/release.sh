#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: $0 vX.Y.Z[-xxx] [release-notes]" >&2
  exit 2
}

[[ $# -ge 1 && $# -le 2 ]] || usage
version=$1
notes=${2:-"Release $version"}

if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9]+([.-][A-Za-z0-9]+)*)?$ ]]; then
  echo "Invalid version '$version'. Expected vX.Y.Z or vX.Y.Z-xxx, for example v1.2.3 or v1.2.3-rc1." >&2
  exit 2
fi

branch=$(git branch --show-current)
if [[ "$branch" != "main" ]]; then
  echo "Releases must be created from main (current branch: ${branch:-detached HEAD})." >&2
  exit 1
fi

if [[ -n "$(git status --porcelain)" ]]; then
  echo "Working tree must be clean before creating a release." >&2
  exit 1
fi

git fetch origin main
if [[ "$(git rev-parse HEAD)" != "$(git rev-parse origin/main)" ]]; then
  echo "Local main must match origin/main before creating a release." >&2
  exit 1
fi

if git show-ref --verify --quiet "refs/tags/$version"; then
  echo "Local tag '$version' already exists." >&2
  exit 1
fi
if git ls-remote --exit-code --tags origin "refs/tags/$version" >/dev/null 2>&1; then
  echo "Remote tag '$version' already exists." >&2
  exit 1
fi

git tag --cleanup=verbatim -a "$version" -m "$notes"
git push origin "refs/tags/$version"

echo "Pushed tag $version. The tag push triggered the release workflow."
