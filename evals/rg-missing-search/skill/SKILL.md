---
name: powershell-search-fallback
description: Find source implementations and configuration in Windows PowerShell repositories when ripgrep may be unavailable.
---

Before a repository search relies on `rg`, check `Get-Command rg -ErrorAction SilentlyContinue`. If it exists, use it normally. If absent, search the relevant directories with `Get-ChildItem -Recurse -File` and `Select-String -SimpleMatch`; inspect the matching files with `Get-Content` to distinguish implementation, documentation, and archived configuration.

An absent command and a search with no matches are different outcomes. Do not repeat an unavailable `rg` call or install tools merely to search. `Select-String` prints nothing when there is no match; report absence only after searching the requested scope. Preserve the user's exclusions and return paths relative to the repository.
