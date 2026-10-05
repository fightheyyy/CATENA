import assert from "node:assert/strict";
import test from "node:test";
import { primaryNavigationRoutes, readNavigation, navigationURL } from "../src/navigation.ts";

test("primary navigation follows the focused cloud workspace", () => {
  assert.deepEqual(primaryNavigationRoutes, ["home", "traces", "memory", "cases", "evolution"]);
  assert.equal(primaryNavigationRoutes.includes("home"), true);
  assert.equal(primaryNavigationRoutes.includes("settings"), false);
});

test("evidence links retain exact IDs through URL encoding and reload", () => {
  const url = new URL(navigationURL("traces", { agentID: "Agent / 中文", traceID: "trace:second&old=1" }), "http://localhost");
  assert.deepEqual(readNavigation(url), { route: "traces", agentID: "Agent / 中文", traceID: "trace:second&old=1", jobID: "" });
  assert.equal(navigationURL("settings", { agentID: "private-agent", traceID: "trace" }), "/settings");
  assert.equal(navigationURL("evolution", { agentID: "a", jobID: "j" }), "/evolution?agent=a&job=j");
  assert.equal(navigationURL("assistant", { agentID: "a" }), "/?agent=a");
  assert.deepEqual(readNavigation(new URL("http://localhost/assistant?agent=a")), {route: "home", agentID: "a", traceID: "", jobID: ""});
  assert.equal(readNavigation(new URL(navigationURL("home", {jobID: "j"}), "http://localhost")).jobID, "j");
  assert.equal(readNavigation(new URL("http://localhost/evolution?agent=a")).route, "home");
  assert.equal(readNavigation(new URL("http://localhost/evolution?agent=a&job=j")).route, "evolution");
});
