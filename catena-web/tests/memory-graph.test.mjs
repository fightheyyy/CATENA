import assert from "node:assert/strict";
import test from "node:test";
import { buildMemoryGraph } from "../src/memoryGraph.ts";

test("buildMemoryGraph creates only evidence-backed nodes and links a known related fact", () => {
  const graph = {
    fact_id: 27,
    content: "The user requested a release brief.",
    entities: [{ name: "release brief", type: "DOCUMENT" }],
    relations: [{
      source: "The user requested a release brief.",
      target: "The assistant delivered release-brief.md.",
      type: "DELIVEREDAS",
      confidence: 0.95,
    }],
    total_entities: 1,
    total_relations: 1,
  };
  const model = buildMemoryGraph(graph, [
    { id: "27", content: graph.content },
    { id: "31", content: "The assistant delivered release-brief.md." },
  ]);

  assert.deepEqual(model.nodes.map((node) => node.kind), ["fact", "entity", "related"]);
  assert.equal(model.nodes[2].factId, "31");
  assert.deepEqual(model.edges.map((edge) => edge.label), ["MENTIONS", "DELIVEREDAS"]);
  assert.equal(model.hiddenEntities, 0);
  assert.equal(model.hiddenRelations, 0);
});

test("buildMemoryGraph bounds dense graph output", () => {
  const graph = {
    fact_id: 1,
    content: "root",
    entities: Array.from({ length: 10 }, (_, index) => ({ name: `entity-${index}`, type: "unknown" })),
    relations: Array.from({ length: 11 }, (_, index) => ({ source: "root", target: `fact-${index}`, type: "RELATED", confidence: 0.5 })),
    total_entities: 10,
    total_relations: 11,
  };
  const model = buildMemoryGraph(graph, []);

  assert.equal(model.nodes.length, 11);
  assert.equal(model.hiddenEntities, 5);
  assert.equal(model.hiddenRelations, 6);
});
