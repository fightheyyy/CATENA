# Third-party Runtime parser sources

Catena's Coding Agent capture module derives its Runtime-specific parsing and
turn-assembly code from the following MIT-licensed Langfuse repositories. The
Langfuse backend clients, SDK emitters, credential handling and hosted API are
not included or used.

| Runtime | Upstream | Pinned commit | Local derived code |
| --- | --- | --- | --- |
| Codex | <https://github.com/langfuse/codex-observability-plugin> | `7500867afecf963d1cf83bf2b860a659591ace18` | `codex/src/langfuse-derived/` |
| Claude Code | <https://github.com/langfuse/claude-observability-plugin> | `5b3d4323c49f3839545fad36883ed02420ebc0ba` | `catena_tap/claude_parser.py` |

The corresponding license grants are preserved in
`third_party/langfuse-codex/LICENSE` and
`third_party/langfuse-claude/LICENSE`. Each adjacent `SOURCE.md` records the
files reused and Catena-specific modifications.
