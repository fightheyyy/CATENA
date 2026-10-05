---
name: reliable-file-editing
description: Edit existing files while preserving unrelated content and recover from patch path or deletion failures.
---

# Reliable File Editing

Read the current target and use a small update for a local change. A delete-and-add rewrite can fail even when writing the existing file is allowed. After patch failure, check the actual path, current content, and write versus directory permissions. Prefer updating the existing file; if patch remains unsuitable, use a bounded read-modify-write operation. Preserve encoding, line endings, required comments, unrelated files, and user changes. Verify the requested behavior and inspect the diff. Do not change permissions or rewrite an entire file unless the task requires it.
