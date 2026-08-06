import {
  Controls,
  Handle,
  MarkerType,
  Position,
  ReactFlow,
  type Edge,
  type Node,
  type NodeProps,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { useMemo } from "react";
import { buildMemoryGraph, type MemoryVisualNode } from "./memoryGraph";
import type { MemoryFactGraph, MemoryRecord } from "./types";

type MemoryGraphProps = {
  graph: MemoryFactGraph;
  facts: MemoryRecord[];
  selectedNodeID: string;
  onSelect: (node: MemoryVisualNode) => void;
  onOpenFact: (factID: string) => void;
};

type FlowNode = Node<MemoryVisualNode, "memory">;

export function MemoryGraphCanvas({ graph, facts, selectedNodeID, onSelect, onOpenFact }: MemoryGraphProps) {
  const model = useMemo(() => buildMemoryGraph(graph, facts), [graph, facts]);
  const nodes = useMemo<FlowNode[]>(() => model.nodes.map((node) => ({
    id: node.id,
    type: "memory",
    position: node.position,
    data: node,
    selected: node.id === selectedNodeID,
    draggable: false,
  })), [model.nodes, selectedNodeID]);
  const edges = useMemo<Edge[]>(() => model.edges.map((edge) => ({
    id: edge.id,
    source: edge.source,
    target: edge.target,
    label: edge.label,
    type: "smoothstep",
    markerEnd: { type: MarkerType.ArrowClosed, width: 12, height: 12, color: "#777a75" },
    style: { stroke: "#8d8f8a", strokeWidth: 1.25 },
    labelStyle: { fill: "#4d4f4b", fontSize: 10, fontWeight: 700 },
    labelBgStyle: { fill: "#fafaf8", fillOpacity: 0.94 },
    labelBgPadding: [5, 3],
    ariaLabel: edge.confidence ? `${edge.label}, ${Math.round(edge.confidence * 100)} percent confidence` : edge.label,
  })), [model.edges]);

  return (
    <div className="memory-graph-canvas" aria-label="Memory relationship graph">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={{ memory: MemoryNode }}
        fitView
        fitViewOptions={{ padding: 0.18, minZoom: 0.58, maxZoom: 1.05 }}
        minZoom={0.42}
        maxZoom={1.5}
        nodesConnectable={false}
        nodesDraggable={false}
        elementsSelectable
        proOptions={{ hideAttribution: true }}
        onNodeClick={(_, flowNode) => {
          const node = flowNode.data;
          onSelect(node);
          if (node.kind === "related" && node.factId && node.factId !== String(graph.fact_id)) {
            onOpenFact(node.factId);
          }
        }}
      >
        <Controls showInteractive={false} position="bottom-left" />
      </ReactFlow>
      {model.hiddenEntities || model.hiddenRelations ? (
        <p className="memory-graph-limit">+{model.hiddenEntities + model.hiddenRelations} more connected nodes</p>
      ) : null}
    </div>
  );
}

function MemoryNode({ data, selected }: NodeProps<FlowNode>) {
  return (
    <article className={`memory-node ${data.kind} ${selected ? "selected" : ""}`}>
      <Handle type="target" position={Position.Left} className="memory-node-handle" />
      <p>{data.eyebrow}</p>
      <strong>{data.title}</strong>
      {data.relation?.confidence ? <span>{Math.round(data.relation.confidence * 100)}%</span> : null}
      <Handle type="source" position={Position.Right} className="memory-node-handle" />
    </article>
  );
}
