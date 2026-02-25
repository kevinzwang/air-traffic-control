---
name: release
description: Create a new GitHub release by tagging and pushing a version tag. Triggers the CI release workflow that builds binaries for all platforms.
disable-model-invocation: true
user-invocable: true
argument-hint: [version]
---

Create a new release for this project.

## Steps

1. **Get the latest version tag** by running `git tag -l 'v*' --sort=-v:refname | head -1` to find the current latest version.

2. **Determine the target version:**
   - If the user provided a version argument (`$ARGUMENTS`), use that as the target version. Ensure it starts with `v` (prepend if missing).
   - If NO version argument was provided (i.e. `$ARGUMENTS` is empty), parse the latest tag into major, minor, and patch components, then ask the user which bump they want using AskUserQuestion with these options:
     - **Patch** — with description showing the resulting version (e.g., "v0.2.5 -> v0.2.6")
     - **Minor** — with description showing the resulting version (e.g., "v0.2.5 -> v0.3.0")
     - **Major** — with description showing the resulting version (e.g., "v0.2.5 -> v1.0.0")

3. **Validate** that the chosen version tag does not already exist (`git tag -l <version>`). If it does, tell the user and stop.

4. **Confirm with the user** before proceeding: show the version that will be tagged, the commit it points to (`git log --oneline -1`), and ask for confirmation.

5. **Create and push the tag:**
   ```bash
   git tag <version>
   git push origin <version>
   ```

6. **Report success** with a link to the GitHub Actions workflow run. Use `gh run list --workflow=release.yml --limit=1` to find it.
