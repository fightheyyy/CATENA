# ADR 0002: Three business services and six demo containers

- Status: accepted for MVP1
- Date: 2026-08-02

## Context

The current Go server directly starts local Node workers. That is convenient
for development but blurs the control/execution boundary. Splitting Scenario,
each XiaoBa role, OTLP, and every queue into separate microservices would make
the first demo operationally complex without a scaling requirement.

## Decision

MVP1 deploys three business services:

1. `catena-app`: LangWatch-derived Web, Scenario, auth/project, OTLP and Trace;
2. `catena-core`: Go workflow/control plane;
3. `catena-runner`: Barena engine plus restricted XiaoBaOS role runtime.

Together with PostgreSQL, ClickHouse, and Redis, the Compose stack has exactly
six long-running containers. Internal runner transport replaces direct worker
ownership in the Compose path. A local subprocess compatibility path may remain
temporarily, with removal after the remote runner passes equivalent
cancellation, event-ordering, and failure tests.

## Consequences

- The Go backend is a real control plane and does not execute model/runtime
  work itself.
- The runner is functional rather than a placeholder sidecar.
- LangWatch worker processes may be supervised inside `catena-app` for the
  local demo; independent scaling is deferred.
- Kubernetes is not adopted until Compose is insufficient for a measured
  deployment need.
