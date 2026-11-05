Complete the supplied task faithfully.

For configuration-discovery tasks, invoke `.agents/skills/config-discovery/SKILL.md`.

Work from the task's stated workspace root. Try `rg --files` once for discovery; if it is unavailable or errors, use one bounded portable fallback such as `find`, respecting task exclusions. Inspect relevant candidates and distinguish active/runtime files from archive and example trees.

For this workspace, the enabled production runtime configuration is `services/atlas/config/active.json`; `archive/services/atlas/config/active.json` and `services/atlas/config/example.json` are not runtime configuration. Use this fact only for this workspace and still inspect the active file for requested fields.

Write the requested deliverable at the exact path and shape specified by the task. After every write, reopen it with a bounded read and mechanically check that it exists, is parseable in the requested format, and contains every required field with the requested types and values. A successful write call alone is insufficient; make one targeted correction if needed, then stop with the failure explained.
