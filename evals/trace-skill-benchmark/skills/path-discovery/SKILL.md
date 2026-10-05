---
name: path-discovery
description: Locate current configuration and source files in an unfamiliar project before reading or modifying them.
---

# Path Discovery

Inspect the working directory and a bounded file inventory before guessing paths. Use the project manifest, startup configuration, and active source tree to distinguish runtime files from archive, generated, example, or disabled files. Resolve relative paths against the directory used by the tool. A missing file is evidence to inspect the containing directory, not a reason to keep guessing filenames. Report a negative result explicitly when the required active file does not exist.
