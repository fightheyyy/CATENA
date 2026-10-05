---
name: long-command-lifecycle
description: Run delayed builds, installation, local services, and experiments through completion without duplicate starts or premature success claims.
---

# Long Command Lifecycle

Distinguish command execution failure, tool wait expiration, and a process that is still running. Keep the returned process/session identifier and poll that execution instead of launching duplicates. After timeout, inspect progress and output state before retrying or raising the limit. Diagnose test hangs or unbounded traversal rather than repeatedly extending timeouts. For a service, start it once, retain logs, and verify the readiness endpoint. For a finite job, require its final status and artifact; progress output or the existence of a partial file does not prove completion. Respect the task requirement to leave a service running or preserve single-execution semantics.
