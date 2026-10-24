---
name: create-pr
description: Draft or create a pull request for this repository. Use when the user asks to prepare or open a PR.
---

# Create a pull request

Prepare a clear PR title and body from the final changes on the current branch relative to the PR target. When the user asks to open the PR, push the branch if needed and create the PR with GitHub CLI.

## Review the changes

1. Use the target branch named by the user. Use `main` when the user does not name a target branch.
2. Confirm that the current branch is not the target branch. Compare the current branch with the target branch using the merge base and inspect the final diff (`target...HEAD`). Use the commit list and PR history to understand the changes, but write only about behavior present in that final diff.
3. Check the working tree. If uncommitted changes are part of the requested PR, stop and ask the user to commit them. Do not commit or stage changes unless the user asks. If changes are unrelated, state that they are excluded from the PR. Pass `--allow-dirty` to the helper only after confirming all uncommitted changes are unrelated.
4. Read the changed code and existing tests. Report test status only when the user or current task has run those tests.
5. Describe changes that are present in the diff. Do not infer features or claim behavior that the changes do not support.

The PR body describes the net change proposed for the target branch. Treat development history as context: when a problem found during feature work was fixed before the PR is ready, describe the final feature once and omit the resolved intermediate problem as a separate bugfix. Use `Bugfix` for a defect that the final diff fixes for users.

## Write the title and body

Write a short, direct title that states the main change. Write the body in simple English and apply ASD-STE100 writing principles. Use common words, short sentences, and one idea in each point. Keep technical names when the reader needs them. Do not claim full compliance with the controlled vocabulary unless you check the official dictionary.

Use numbered points under category headings. Include only categories that have content. Each point can have zero to three sub-items. Do not add sub-items to fill space. Do not add a PR number to a point.

Use these categories when they fit the changes:

- `Features`
- `Bugfix`
- `Chore`
- `Test`
- `Documentations`
- Other short headings when the changes need them, such as `Build`, `CI`, `Security`, or `Performance`

Use this Markdown structure:

```markdown
### Features

1. Add a command to register a project
    - Accept relative paths
    - Check the project configuration

### Test

1. Check project registration with relative paths

### Documentations

1. Explain how to register a project
```

## Create the PR

If the user asks only for a draft, show the title and body, then stop. If the user asks to open the PR:

1. Show the proposed title and body. The user's request to create the PR authorizes the push and PR creation.
2. Write the body to a temporary Markdown file.
3. Run the helper with Bash. Keep `create-pr.sh` in LF format; it may not have executable permission, and CRLF line endings can break Bash.

   ```bash
   bash .agents/skills/create-pr/create-pr.sh \
     --base "$base" \
     --title "$title" \
     --body-file "$body_file"
   ```

   Omit `--base` when the target is `main`. If unrelated local changes remain, add `--allow-dirty` after stating that they are excluded. The helper checks for an existing PR before it pushes, then pushes the branch if needed and creates the PR with `gh pr create --body-file`.
4. Report the PR URL and the test status that is supported by the available evidence.

Use GitHub CLI (`gh`) for PR operations. Do not commit, amend, rebase, or force-push as part of PR creation.
