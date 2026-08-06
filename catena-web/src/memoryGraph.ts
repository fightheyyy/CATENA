import type { MemoryFactGraph, MemoryGraphEntity, MemoryGraphRelation, MemoryRecord } from "./types";

export type MemoryVisualNodeKind = "fact" | "related" | "entity";

export type MemoryVisualNode = {
  id: string;
  kind: MemoryVisualNodeKind;
  eyebrow: string;
  title: string;
  content: string;
  factId?: string;
  entity?: MemoryGraphEntity;
  relation?: MemoryGraphRelation;
  position: { x: number; y: number };
};

export type MemoryVisualEdge = {
  id: string;
  source: string;
  target: string;
  label: string;
  confidence?: number;
};

export type MemoryGraphModel = {
  nodes: MemoryVisualNode[];
  edges: MemoryVisualEdge[];
  hiddenEntities: number;
  hiddenRelations: number;
};

const maxEntityNodes = 5;
const maxRelationNodes = 5;

export function buildMemoryGraph(graph: MemoryFactGraph, catalog: MemoryRecord[]): MemoryGraphModel {
  const rootID = `fact:${graph.fact_id}`;
  const entities = graph.entities.slice(0, maxEntityNodes);
  const relations = graph.relations.slice(0, maxRelationNodes);
  const nodes: MemoryVisualNode[] = [{
    id: rootID,
    kind: "fact",
    eyebrow: `FACT ${graph.fact_id}`,
    title: shortTitle(graph.content, 52),
    content: graph.content,
    factId: String(graph.fact_id),
    position: { x: 290, y: 210 },
  }];
  const edges: MemoryVisualEdge[] = [];

  entities.forEach((entity, index) => {
    const id = `entity:${index}:${entity.name}`;
    nodes.push({
      id,
      kind: "entity",
      eyebrow: entity.type && entity.type !== "unknown" ? entity.type.toUpperCase() : "ENTITY",
      title: entity.name,
      content: entity.description || entity.name,
      entity,
      position: { x: 24, y: distributedY(index, entities.length) },
    });
    edges.push({ id: `edge:${rootID}:${id}`, source: rootID, target: id, label: "MENTIONS" });
  });

  relations.forEach((relation, index) => {
    const relatedID = matchRelatedFactID(relation.target, catalog);
    const id = relatedID ? `fact:${relatedID}` : `related:${index}`;
    const uniqueID = nodes.some((node) => node.id === id) ? `${id}:${index}` : id;
    nodes.push({
      id: uniqueID,
      kind: "related",
      eyebrow: relatedID ? `FACT ${relatedID}` : "RELATED FACT",
      title: shortTitle(relation.target, 52),
      content: relation.target,
      factId: relatedID,
      relation,
      position: { x: 590, y: distributedY(index, relations.length) },
    });
    edges.push({
      id: `edge:${rootID}:${uniqueID}`,
      source: rootID,
      target: uniqueID,
      label: relation.type || "RELATED",
      confidence: relation.confidence,
    });
  });

  return {
    nodes,
    edges,
    hiddenEntities: Math.max(0, graph.entities.length - entities.length),
    hiddenRelations: Math.max(0, graph.relations.length - relations.length),
  };
}

function distributedY(index: number, total: number) {
  if (total <= 1) return 215;
  const top = 40;
  const bottom = 380;
  return Math.round(top + ((bottom - top) * index) / (total - 1));
}

function shortTitle(value: string, limit: number) {
  const clean = value.replace(/\s+/g, " ").trim();
  if (clean.length <= limit) return clean;
  return `${clean.slice(0, Math.max(1, limit - 3)).trimEnd()}...`;
}

function matchRelatedFactID(target: string, catalog: MemoryRecord[]) {
  const cleanTarget = target.replace(/\s+/g, " ").trim();
  const prefix = cleanTarget.endsWith("...") ? cleanTarget.slice(0, -3) : cleanTarget;
  const match = catalog.find((fact) => {
    const content = fact.content.replace(/\s+/g, " ").trim();
    return content === cleanTarget || (prefix.length > 20 && content.startsWith(prefix));
  });
  return match && /^\d+$/.test(match.id) ? match.id : undefined;
}
