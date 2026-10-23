---
name: release
description: Prepare a release for this repository. Use when the user asks to draft release notes or publish a versioned release.
---

# Release this project

Create release notes from merged pull requests. When the user asks to publish a release, pass those notes to `release.sh`. The script creates and pushes an annotated tag. The tag push starts the GitHub Actions workflow, which creates the GitHub Release and uploads the binaries.

## Prepare the notes

1. Get the target version or tag from the user. If the user asks only for a draft, do not require a version.
2. Use the requested target tag as the comparison endpoint. If the user asks only for a draft and gives no target tag, use `HEAD` and state that endpoint. Find the previous release tag in its history and compare the endpoint with that tag. For the first release, use all merged pull requests included in the endpoint.
3. List merged pull requests whose merge commits are in that tag-to-tag range. Review each pull request title and description. Use its final user-visible change as the release note. Put each pull request in one section: `Features`, `Bugfix`, or `Chore`.
4. Write one numbered point for each pull request. Add its pull request number to that point. Add zero to three sub-items only when they help explain the change.
5. Describe the net changes between the two endpoints, not the development history inside the range. A feature developed and then fixed before release is one feature note; do not add a separate bugfix note for the resolved intermediate problem. Use `Bugfix` for a defect fixed by a PR that remains a user-visible change in the release diff.
6. Omit empty sections. Omit changes that do not map to a PR. If a user-visible change in the tag range has no matching PR, tell the user before publishing.
7. Show the complete notes to the user before publishing. When the user asked to publish, continue to the release script unless a release precondition fails. When the user asked only for notes, stop after showing the draft.

## Release note format

Use these headings and Markdown structure:

```markdown
### Features

1. Add a project registration command (#123)
    - Accept relative paths
    - Check the project configuration

### Bugfix

1. Keep the project name when local settings are missing (#124)

### Chore

1. Update the Go toolchain version (#125)
```

Write in simple English. Use short, direct sentences and common words. State what changed for the user. Keep one idea in each point or sub-item. Use active voice when it is clear. Keep technical names when the user needs them. Apply ASD-STE100 writing principles; do not claim full compliance with its controlled vocabulary unless you check the official dictionary.

Every numbered point must include its pull request number in `(#123)` form. A point can have no sub-items. If a point has sub-items, use no more than three. Do not add sub-items to fill space.

## Publish

When the user asks to publish the release:

1. Confirm that the working tree is clean, the current branch is `main`, and local `main` matches `origin/main`. The script checks these conditions too. Confirm that the target commit is the intended release point and that the notes cover only changes since the previous release tag.
2. Present the release notes and version. If the user has not authorized publishing, ask for approval before pushing the tag.
3. Run the co-located helper with the version and the full notes as its second argument:

   ```bash
   .agents/skills/release/release.sh "v1.2.3-rc1" "$release_notes"
   ```

   Pass the multi-line notes as one quoted shell argument. Do not create the tag or GitHub Release by another method.
4. Report whether the tag push succeeded. Explain that GitHub Actions will create the Release and upload the binaries.

The helper requires Git and permission to push tags to `origin`. It does not create a GitHub Release. The workflow reads the annotated tag message for the Release notes.
