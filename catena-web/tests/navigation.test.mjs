import assert from "node:assert/strict";
import test from "node:test";
import { primaryNavigationRoutes, readNavigation, navigationURL } from "../src/navigation.ts";

test("primary navigation follows the focused cloud workspace", () => {
  assert.deepEqual(primaryNavigationRoutes, ["home", "traces", "memory", "evolution"]);
  assert.equal(primaryNavigationRoutes.includes("home"), true);
  assert.equal(primaryNavigationRoutes.includes("settings"), false);
});

test("evidence links retain exact IDs through URL encoding and reload", () => {
  const url = new URL(navigationURL("traces", { agentID: "Agent / 中文", traceID: "trace:second&old=1" }), "http://localhost");
  assert.deepEqual(readNavigation(url), { route: "traces", agentID: "Agent / 中文", traceID: "trace:second&old=1", jobID: "" });
  assert.equal(navigationURL("settings", { agentID: "private-agent", traceID: "trace" }), "/settings");
  assert.equal(navigationURL("evolution", { agentID: "a", jobID: "j" }), "/evolution?agent=a&job=j");
});
