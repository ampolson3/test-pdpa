"use client";

import { useEffect, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import {
  createApiClient,
  useDataflowGraph,
  useLegalEntities,
  type DataflowEdge,
  type DataflowNode,
} from "@pdpa/api-client";
import {
  ReactFlow,
  Background,
  Controls,
  MarkerType,
  type Edge,
  type Node,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import ELK from "elkjs/lib/elk.bundled.js";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";

const NODE_SIZE: Record<string, { width: number; height: number }> = {
  activity: { width: 200, height: 56 },
  recipient: { width: 180, height: 48 },
  country: { width: 140, height: 40 },
  disposal: { width: 150, height: 36 },
};

const NODE_COLOR: Record<string, string> = {
  activity: "#0ea5e9",
  recipient: "#8b5cf6",
  country: "#f59e0b",
  disposal: "#64748b",
};

const EDGE_COLOR: Record<string, string> = {
  send: "#8b5cf6",
  transfer: "#f59e0b",
  destroy: "#64748b",
};

interface ElkNode {
  id: string;
  width?: number;
  height?: number;
  layoutOptions?: Record<string, string>;
  children?: ElkNode[];
  x?: number;
  y?: number;
}

const elk = new ELK();

async function layout(gNodes: DataflowNode[], gEdges: DataflowEdge[]) {
  const byId = new Map(gNodes.map((n) => [n.id, n]));
  const childrenOf = new Map<string, DataflowNode[]>();
  for (const n of gNodes) {
    const key = n.parent_id ?? "";
    childrenOf.set(key, [...(childrenOf.get(key) ?? []), n]);
  }

  function buildElk(id: string): ElkNode {
    const n = byId.get(id);
    const kids = childrenOf.get(id) ?? [];
    const isContainer = kids.length > 0;
    const size = n ? NODE_SIZE[n.type] ?? { width: 170, height: 44 } : { width: 170, height: 44 };
    if (isContainer) {
      return {
        id,
        layoutOptions: {
          "elk.algorithm": "layered",
          "elk.direction": "RIGHT",
          "elk.padding": "[top=44,left=20,bottom=20,right=20]",
          "elk.spacing.nodeNode": "28",
        },
        children: kids.map((k) => buildElk(k.id)),
      };
    }
    return { id, width: size.width, height: size.height };
  }

  const topLevel = childrenOf.get("") ?? [];
  const elkGraph: ElkNode = {
    id: "root",
    layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": "RIGHT",
      "elk.spacing.nodeNode": "48",
      "elk.layered.spacing.nodeNodeBetweenLayers": "80",
    },
    children: topLevel.map((n) => buildElk(n.id)),
  };

  const elkEdges = gEdges.map((e) => ({ id: e.id, sources: [e.source], targets: [e.target] }));

  // elkjs's own types don't model nested `children`/`edges` the way this recursive builder does; the
  // library itself is untyped-friendly at runtime, so a narrow cast here is simpler than fighting its d.ts.
  const result = (await elk.layout({ ...elkGraph, edges: elkEdges } as never)) as unknown as ElkNode & {
    children?: (ElkNode & { children?: ElkNode[] })[];
  };

  const positions = new Map<string, { x: number; y: number; width: number; height: number; parentId?: string }>();
  function walk(node: ElkNode & { children?: ElkNode[] }, parentId: string | undefined, offsetX: number, offsetY: number) {
    if (node.id !== "root") {
      const size = node.width && node.height ? { width: node.width, height: node.height } : { width: 220, height: 120 };
      positions.set(node.id, { x: node.x ?? 0, y: node.y ?? 0, width: size.width, height: size.height, parentId });
    }
    for (const child of node.children ?? []) {
      walk(child as ElkNode & { children?: ElkNode[] }, node.id === "root" ? undefined : node.id, 0, 0);
    }
  }
  walk(result, undefined, 0, 0);
  return positions;
}

function toFlowNode(n: DataflowNode, pos: { x: number; y: number; width: number; height: number }): Node {
  // org_unit nodes are rendered as a React Flow "group" background; every other node type is a styled box.
  const isGroup = n.type === "org_unit";
  return {
    id: n.id,
    type: isGroup ? "group" : undefined,
    data: { label: n.label, node: n },
    position: { x: pos.x, y: pos.y },
    parentId: n.parent_id ?? undefined,
    extent: n.parent_id ? "parent" : undefined,
    style: isGroup
      ? { width: pos.width, height: pos.height, border: "1px solid #cbd5e1", borderRadius: 8, background: "rgba(241,245,249,0.6)" }
      : {
          width: pos.width,
          height: pos.height,
          borderRadius: 8,
          border: `2px solid ${NODE_COLOR[n.type] ?? "#334155"}`,
          background: "#fff",
          fontSize: 12,
          padding: 6,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          textAlign: "center",
        },
    draggable: false,
    selectable: !isGroup,
  };
}

function toFlowEdge(e: DataflowEdge): Edge {
  const color = e.warning ? "#dc2626" : EDGE_COLOR[e.type] ?? "#334155";
  return {
    id: e.id,
    source: e.source,
    target: e.target,
    label: e.label,
    animated: false,
    style: { stroke: color, strokeWidth: e.warning ? 2.5 : 1.5, strokeDasharray: e.warning ? "6 3" : undefined },
    markerEnd: { type: MarkerType.ArrowClosed, color },
    labelStyle: { fontSize: 10, fill: color },
  };
}

/** DFG-01/DFG-03: the RoPA-generated data flow diagram — org-unit swimlanes, activities, recipients and
 *  destination countries, auto-laid-out with ELK on every load (no persisted/arranged layout yet — that's
 *  DFG-05, a later feature). A "transfer" edge in warning colour is a foreign recipient with no ROPA-08
 *  transfer basis logged (DFG-03's own acceptance criterion). */
export function DataFlowContent() {
  const t = useTranslations("dataflow");
  const canRead = usePermission("ropa.dataflow.read");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const [legalEntityId, setLegalEntityId] = useState<string>();

  const entities = useLegalEntities(client);
  const graph = useDataflowGraph(client, { legal_entity_id: legalEntityId });

  const [flowNodes, setFlowNodes] = useState<Node[]>([]);
  const [flowEdges, setFlowEdges] = useState<Edge[]>([]);
  const [laidOut, setLaidOut] = useState(false);

  const gNodes = useMemo(() => graph.data?.nodes ?? [], [graph.data]);
  const gEdges = useMemo(() => graph.data?.edges ?? [], [graph.data]);

  useEffect(() => {
    let cancelled = false;
    setLaidOut(false);
    if (gNodes.length === 0) {
      setFlowNodes([]);
      setFlowEdges([]);
      setLaidOut(true);
      return;
    }
    layout(gNodes, gEdges).then((positions) => {
      if (cancelled) return;
      const byId = new Map(gNodes.map((n) => [n.id, n]));
      // React Flow requires a parent node to appear before its children in the array — sort by nesting
      // depth (org_unit=0, activity=1, disposal=2) rather than just "has a parent at all".
      function depthOf(id: string): number {
        let d = 0;
        let cur = positions.get(id)?.parentId;
        while (cur) {
          d++;
          cur = positions.get(cur)?.parentId;
        }
        return d;
      }
      const ordered = [...positions.entries()].sort((a, b) => depthOf(a[0]) - depthOf(b[0]));
      const nodes: Node[] = [];
      for (const [id, pos] of ordered) {
        const n = byId.get(id);
        if (!n) continue;
        nodes.push(toFlowNode(n, pos));
        if (n.type === "org_unit") {
          // React Flow's built-in "group" node renders no text of its own — a small non-interactive
          // label node, parented to the group, carries the swimlane's name instead.
          nodes.push({
            id: n.id + "__label",
            data: { label: n.label },
            position: { x: 10, y: 6 },
            parentId: n.id,
            extent: "parent",
            draggable: false,
            selectable: false,
            connectable: false,
            style: { background: "transparent", border: "none", fontSize: 12, fontWeight: 600, color: "#334155", padding: 0, width: "auto" },
          });
        }
      }
      setFlowNodes(nodes);
      setFlowEdges(gEdges.map(toFlowEdge));
      setLaidOut(true);
    });
    return () => {
      cancelled = true;
    };
  }, [gNodes, gEdges]);

  if (!canRead) return <main className="mx-auto max-w-6xl p-8 text-slate-600">{t("forbidden")}</main>;

  return (
    <main className="mx-auto max-w-7xl space-y-4 p-8 text-sm">
      <header>
        <h1 className="text-xl font-semibold">{t("title")}</h1>
        <p className="text-slate-600">{t("intro")}</p>
      </header>

      <div className="flex flex-wrap items-end justify-between gap-3">
        <label>
          <span className="block text-slate-600">{t("legalEntity")}</span>
          <select className={INPUT} value={legalEntityId ?? ""} onChange={(e) => setLegalEntityId(e.target.value || undefined)}>
            <option value="">{t("allEntities")}</option>
            {entities.data?.map((le) => (
              <option key={le.id} value={le.id}>
                {le.name_th}
              </option>
            ))}
          </select>
        </label>
        {graph.data ? <p className="text-slate-500">{t("activityCount", { count: graph.data.activity_count })}</p> : null}
      </div>

      <Legend />

      {graph.isPending || !laidOut ? (
        <p className="text-slate-500">{t("loading")}</p>
      ) : graph.isError ? (
        <p className="text-red-700">{t("loadError")}</p>
      ) : flowNodes.length === 0 ? (
        <p className="rounded-md bg-amber-50 p-3 text-amber-800">{t("empty")}</p>
      ) : (
        <div style={{ height: "70vh" }} className="rounded-md border border-slate-200 bg-white" data-testid="dataflow-canvas">
          <ReactFlow nodes={flowNodes} edges={flowEdges} fitView nodesDraggable={false} nodesConnectable={false} edgesFocusable={false}>
            <Background />
            <Controls showInteractive={false} />
          </ReactFlow>
        </div>
      )}
    </main>
  );
}

function Legend() {
  const t = useTranslations("dataflow");
  const items: { color: string; label: string }[] = [
    { color: NODE_COLOR.activity, label: t("legend.activity") },
    { color: NODE_COLOR.recipient, label: t("legend.recipient") },
    { color: NODE_COLOR.country, label: t("legend.country") },
    { color: NODE_COLOR.disposal, label: t("legend.disposal") },
  ];
  const edgeItems: { color: string; label: string; dashed?: boolean }[] = [
    { color: EDGE_COLOR.send, label: t("legend.send") },
    { color: EDGE_COLOR.transfer, label: t("legend.transfer") },
    { color: EDGE_COLOR.destroy, label: t("legend.destroy") },
    { color: "#dc2626", label: t("legend.warning"), dashed: true },
  ];
  return (
    <div className="flex flex-wrap gap-4 rounded-md border border-slate-200 bg-slate-50 p-3 text-xs">
      {items.map((it) => (
        <span key={it.label} className="flex items-center gap-1.5">
          <span className="inline-block h-3 w-3 rounded-sm border-2" style={{ borderColor: it.color }} />
          {it.label}
        </span>
      ))}
      <span className="mx-2 border-l border-slate-300" />
      {edgeItems.map((it) => (
        <span key={it.label} className="flex items-center gap-1.5">
          <span className="inline-block h-0.5 w-5" style={{ background: it.color, opacity: it.dashed ? 0.9 : 1 }} />
          {it.label}
        </span>
      ))}
    </div>
  );
}
