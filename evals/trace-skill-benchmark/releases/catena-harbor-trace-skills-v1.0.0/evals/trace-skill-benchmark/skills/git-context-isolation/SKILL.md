---
name: git-context-isolation
description: Find the correct Git working tree and make scoped changes while preserving dirty user work or isolating an edit.
---

# Git Context Isolation

Check whether the working directory belongs to the intended Git repository using git rev-parse and inspect status before changing files. A parent folder may contain multiple independent repositories; a vendor repository is not automatically the application root. If a directory is not a repository, inspect project structure before retrying git. Preserve existing dirty edits. When isolation is requested, create a separate worktree with its own branch and perform the change there; do not stash, reset, clean, or overwrite the original workspace. Verify both the target change and the original workspace state.
