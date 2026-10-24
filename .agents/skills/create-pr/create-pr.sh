#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: bash create-pr.sh --title TITLE --body-file PATH [--base BRANCH] [--allow-dirty]

Options:
  --title TITLE       Pull request title
  --body-file PATH    Markdown file with the pull request body
  --base BRANCH       Target branch (default: main)
  --allow-dirty       Allow unrelated uncommitted changes; they are not pushed
  -h, --help          Show this help
EOF
}

base=main
title=
body_file=
allow_dirty=false

while (($#)); do
  case "$1" in
    --base)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      base=$2
      shift 2
      ;;
    --title)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      title=$2
      shift 2
      ;;
    --body-file)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      body_file=$2
      shift 2
      ;;
    --allow-dirty)
      allow_dirty=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf 'Unknown option: %s\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -z "$title" || -z "$body_file" || ! -r "$body_file" || ! -s "$body_file" ]]; then
  echo "A title and a readable, non-empty body file are required." >&2
  usage >&2
  exit 2
fi

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"

branch=$(git branch --show-current)
if [[ -z "$branch" ]]; then
  echo "Pull requests must be created from a branch, not detached HEAD." >&2
  exit 1
fi
if [[ "$branch" == "$base" ]]; then
  echo "Current branch is the pull request base ('$base'). Switch to a feature branch first." >&2
  exit 1
fi

git fetch origin "refs/heads/$base:refs/remotes/origin/$base"

existing_url=$(gh pr list --state all --head "$branch" --json url --jq '.[0].url // empty')
if [[ -n "$existing_url" ]]; then
  printf 'A pull request already exists for %s: %s\n' "$branch" "$existing_url"
  exit 0
fi

base_ref="origin/$base"
if ! git merge-base "$base_ref" HEAD >/dev/null; then
  printf "Branch '%s' has no common history with '%s'.\n" "$branch" "$base" >&2
  exit 1
fi
if git diff --quiet "$base_ref...HEAD" --; then
  printf "Branch '%s' has no changes relative to '%s'.\n" "$branch" "$base" >&2
  exit 1
fi

if [[ -n "$(git status --porcelain)" && "$allow_dirty" != true ]]; then
  echo "Working tree has uncommitted changes. Commit PR changes first, or use --allow-dirty only when all changes are unrelated." >&2
  exit 1
fi

local_sha=$(git rev-parse HEAD)
remote_sha=$(git ls-remote origin "refs/heads/$branch" | cut -f1)
if [[ "$local_sha" != "$remote_sha" ]]; then
  git push --set-upstream origin "$branch"
fi

gh pr create --base "$base" --head "$branch" --title "$title" --body-file "$body_file"
