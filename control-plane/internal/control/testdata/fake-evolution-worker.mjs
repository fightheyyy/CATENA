#!/usr/bin/env node
import fs from "node:fs";

const request = JSON.parse(fs.readFileSync(0, "utf8"));
const roles = [
  {
    id: "user-cat",
    display_name: "UserCat",
    responsibility: "Simulate one natural, incomplete user turn without judging the Agent.",
    output: "user turn",
  },
  {
    id: "inspector-cat",
    display_name: "InspectorCat",
    responsibility: "Locate a failure mode in retained evidence and pair it with a replayable Case.",
    output: "finding + case",
  },
  {
    id: "reviewer-cat",
    display_name: "ReviewerCat",
    responsibility: "Review verifier-backed evidence and emit a semantic pass, fail, or blocked verdict.",
    output: "semantic review",
  },
  {
    id: "evolution-cat",
    display_name: "EvolutionCat",
    responsibility: "Create a minimal Role, Skill, or Memory candidate from an accepted finding.",
    output: "role / skill / memory candidate",
  },
];

if (request.operation === "probe") {
  console.log(JSON.stringify({
    schema: "barena.xiaoba_evolution_response.v1",
    request_id: request.request_id,
    operation: "probe",
    status: "ok",
    runtime: {
      schema: "barena.xiaoba_evolution_runtime.v1",
      runtime_id: "xiaobaos-evolution",
      display_name: "XiaoBa Evolution Runtime",
      kind: "embedded_evolution",
      source: "configured",
      status: "ready",
      version: "fake-xiaoba 0.2.1",
      detail: "ready at /host/secret/xiaoba",
      roles,
      capabilities: {
        probe: true,
        role_turn: true,
        cancellation: true,
        telemetry: "native",
        target_runtime_hosted: false,
      },
    },
  }));
  process.exit(0);
}

if (request.operation === "turn") {
  console.log(JSON.stringify({
    schema: "barena.xiaoba_evolution_response.v1",
    request_id: request.request_id,
    operation: "turn",
    status: "ok",
    result: {
      status: "completed",
      detail: "fake role turn completed",
      assistant: { role: "assistant", content: `${request.role}: complete` },
      process: {
        exit_code: 0,
        signal: null,
        duration_ms: 1,
        stdout: "complete",
        stderr: "",
      },
      telemetry: {
        mode: "native",
        configured: Boolean(request.telemetry),
        trace_context_propagated: false,
      },
      native_trace_refs: [],
    },
  }));
  process.exit(0);
}

process.exit(2);
