---
name: python-environment-selection
description: Run project Python tools with the correct existing environment or offline project dependencies.
---

# Python Environment Selection

Check the project environment instructions and available venv or conda interpreters before importing optional libraries. Invoke the chosen interpreter by its path, and use that same interpreter for pip. An import failure in the default Python does not mean every local environment lacks the dependency. Check versions when multiple environments exist. Prefer the declared project environment or an available equivalent tool. Install only required dependencies into a project environment; use local wheels when the task is offline. A successful diagnostic that reports missing packages is not itself a failed task.
