---
name: powershell-execution
description: Execute Python, Node, and text processing through PowerShell without Bash syntax or nested quoting failures.
---

# Powershell Execution

Identify which shell parses each layer. PowerShell does not accept Bash heredoc. For multiline Python or JavaScript, write a temporary script and pass its path to the interpreter. Use PowerShell single quoted here-strings for literal script text; put the closing delimiter on its own line. Avoid nesting shell quoting when a script file removes a layer. Use the call operator & for executable paths. For literal text matching, distinguish regex metacharacters from shell interpolation. Check the actual exit code and result; after a parser error, fix the parsing layer rather than repeating the same string.
