---
name: create-pr
description: Draft or create a pull request for this repository. Use when the user asks to prepare or open a PR.
---

# Create a pull request

Prepare a clear PR title and body from the changes on the current branch. When the user asks to open the PR, push the branch if needed and create the PR with GitHub CLI.

## Review the changes

1. Use the base branch named by the user. Use `main` when the user does not name a base branch.
2. Confirm that the current branch is not the base branch. Inspect the commit list and the complete diff from the base branch.
3. Check the working tree. If uncommitted changes are part of the requested PR, stop and ask the user to commit them. Do not commit or stage changes unless the user asks. If the changes are unrelated, state that they are not part of the PR.
4. Read the changed code and existing tests. Report test status only when the user or current task has run those tests.
5. Describe changes that are present in the diff. Do not infer features or claim behavior that the changes do not support.

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

1. Check whether a PR already exists for the current branch. If one exists, report its URL instead of creating a duplicate.
2. Show the proposed title and body. The user's request to create the PR authorizes the `git push` and `gh pr create` actions.
3. Push the current branch to `origin` if it is not already pushed.
4. Create the PR with the selected base branch, title, and body. Use `gh pr create --title ... --body-file ...` so Markdown formatting stays intact.
5. Report the PR URL and the test status that is supported by the available evidence.

Use GitHub CLI (`gh`) for PR operations. Do not commit, amend, rebase, or force-push as part of PR creation.
