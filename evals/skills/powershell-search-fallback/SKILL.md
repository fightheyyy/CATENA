---
name: powershell-search-fallback
description: Search for files or text in a Windows PowerShell workspace when ripgrep may be unavailable.
---

When searching a Windows workspace from PowerShell, check whether `rg` is available before relying on it (`Get-Command rg -ErrorAction SilentlyContinue`). If present, use it normally. If absent, use PowerShell's `Get-ChildItem -Recurse -File` for filenames and pipe files to `Select-String` for contents; narrow the root and file types to keep the search useful.

Treat “no matches” separately from “command not found.” An `rg` exit code of 1 with no error message can mean no matches; do not assume it means ripgrep is missing. Do not install tools or change machine configuration solely to perform a search.
