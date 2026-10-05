---
name: secret-output-control
description: Audit configuration and Git history while keeping credential values out of tool output and final answers.
---

# Secret Output Control

Before reading .env or historical configuration, decide which facts the audit needs: names, presence, tracked paths, or commit locations. Use a script that projects only these facts before printing. Do not print whole secret files or unfiltered git show/diff/grep output; redacting the final answer cannot remove secrets already in tool output. Treat examples separately from runtime values. Complete the requested audit and report findings without copying raw credential values. Use no network requests to test credentials unless the user authorized them.
