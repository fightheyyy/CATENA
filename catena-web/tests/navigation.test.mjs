import assert from "node:assert/strict";
import test from "node:test";
import { primaryNavigationRoutes } from "../src/navigation.ts";

test("primary navigation follows the Catena evidence loop", () => {
  assert.deepEqual(primaryNavigationRoutes, ["agents", "conversations", "memory", "traces", "evolution"]);
  assert.equal(primaryNavigationRoutes.includes("home"), false);
  assert.equal(primaryNavigationRoutes.includes("settings"), false);
});
