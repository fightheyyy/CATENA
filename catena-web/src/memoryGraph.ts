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
  origin?: string;
};

export type MemoryGraphModel = {
  nodes: MemoryVisualNode[];
  edges: MemoryVisualEdge[];
  hiddenEntities: number;
  hiddenRelations: number;
};

const maxEntityNodes = 5;
const maxRelationNodes = 5;

export function buildMemoryGraph(graph: MemoryFactGraph, catalog: MemoryRecord[], locale: "zh" | "en" = "en"): MemoryGraphModel {
  const rootID = `fact:${graph.fact_id}`;
  const entities = [...graph.entities]
    .sort((left, right) => sourceEntityRank(left) - sourceEntityRank(right))
    .slice(0, maxEntityNodes);
  const allEntityNames = new Set(graph.entities.map((entity) => entity.name.trim().toLowerCase()));
  const entityRelations = graph.relations.filter((relation) => allEntityNames.has(relation.target.trim().toLowerCase()));
  const relations = graph.relations.filter((relation) => !allEntityNames.has(relation.target.trim().toLowerCase())).slice(0, maxRelationNodes);
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
    const relation = entityRelations.find((candidate) => candidate.target.trim().toLowerCase() === entity.name.trim().toLowerCase());
    edges.push({
      id: `edge:${rootID}:${id}`,
      source: rootID,
      target: id,
      label: relation ? memoryRelationLabel(relation.type, locale) : locale === "zh" ? "提及" : "MENTIONS",
      confidence: relation?.confidence,
      origin: relation?.origin || "semantic",
    });
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
      label: memoryRelationLabel(relation.type, locale),
      confidence: relation.confidence,
      origin: relation.origin,
    });
  });

  return {
    nodes,
    edges,
    hiddenEntities: Math.max(0, graph.entities.length - entities.length),
    hiddenRelations: Math.max(0, graph.relations.length - entityRelations.length - relations.length),
  };
}

function sourceEntityRank(entity: MemoryGraphEntity) {
  const type = entity.type.trim().toLowerCase();
  if (type === "conversation") return 0;
  if (type === "agent") return 1;
  return 2;
}

function memoryRelationLabel(value: string, locale: "zh" | "en") {
  const normalized = value.trim().toUpperCase();
  if (normalized === "SAME_CONVERSATION") return locale === "zh" ? "同源对话" : "SAME CONVERSATION";
  if (normalized === "SOURCE_CONVERSATION") return locale === "zh" ? "来自对话" : "FROM CONVERSATION";
  if (normalized === "SOURCE_AGENT") return locale === "zh" ? "来自 AGENT" : "FROM AGENT";
  return value || (locale === "zh" ? "相关" : "RELATED");
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
