"use client";

import { useEffect, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import {
  createApiClient,
  useDataflowGraph,
  useLegalEntities,
  dataflowExportHref,
  dataflowReportHref,
  useDataflowSnapshots,
  useDataflowPublishSnapshot,
  useDataflowLayout,
  useSaveDataflowLayout,
  useResetDataflowLayout,
  useHighRiskActivityScores,
  type DataflowEdge,
  type DataflowNode,
  type DataflowView,
} from "@pdpa/api-client";
import { Button } from "@pdpa/ui";
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
import { useRouter } from "@/i18n/routing";

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

// activityIdOf extracts the uuid from an activity node's own id scheme ("activity:<uuid>", see
// dataflowservice.activityNodeID) — the only way to match a Graph node back to a risk.activity_scores row,
// since the backend never joins the two modules together (rule 9).
function activityIdOf(nodeId: string): string | undefined {
  return nodeId.startsWith("activity:") ? nodeId.slice("activity:".length) : undefined;
}

// DFG-06: whichever swimlane type the requested view produced (org_unit/legal_entity/data_category) is
// rendered as a React Flow "group" background, same as the department view's own org_unit always was.
function isSwimlane(type: string): boolean {
  return type === "org_unit" || type === "legal_entity" || type === "data_category";
}

// DFG-05: the caller's own saved drag-and-drop overlay for one (legal_entity, view) diagram — node
// positions keyed by the node's own stable id (still applies after a RoPA data update, since the id never
// changes for an activity/recipient/country/disposal that still exists) and free-text annotations per node.
interface Overlay {
  positions: Record<string, { x: number; y: number }>;
  notes: Record<string, string>;
}

const EMPTY_OVERLAY: Overlay = { positions: {}, notes: {} };

function toFlowNode(
  n: DataflowNode,
  pos: { x: number; y: number; width: number; height: number },
  isHighRisk: boolean,
  draggable: boolean,
  note: string | undefined,
): Node {
  const isGroup = isSwimlane(n.type);
  // RRA-11: a high-risk activity is highlighted the same warning-red DFG-04/DSA-12's own missing-agreement
  // flag already uses — "ไฮไลต์กิจกรรม...พร้อมเหตุผลและลิงก์ไปแก้ไข" (reason via the tooltip, link via
  // onNodeClick below).
  const warn = n.warning || isHighRisk;
  return {
    id: n.id,
    type: isGroup ? "group" : undefined,
    data: { label: note ? `${n.label} 📝` : n.label, node: n, isHighRisk, note },
    position: { x: pos.x, y: pos.y },
    parentId: n.parent_id ?? undefined,
    extent: n.parent_id ? "parent" : undefined,
    style: isGroup
      ? { width: pos.width, height: pos.height, border: "1px solid #cbd5e1", borderRadius: 8, background: "rgba(241,245,249,0.6)" }
      : {
          width: pos.width,
          height: pos.height,
          borderRadius: 8,
          // DFG-04/DSA-12: a processor recipient with no DPA, or a controller/joint_controller recipient
          // with no DSA, is outlined and tinted the same warning red the transfer edges already use, and
          // clickable through to its own agreement(s) when any exist.
          border: warn ? "2px solid #dc2626" : `2px solid ${NODE_COLOR[n.type] ?? "#334155"}`,
          background: warn ? "#fef2f2" : "#fff",
          fontSize: 12,
          padding: 6,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          textAlign: "center",
          cursor: n.agreement_ids?.length || isHighRisk || draggable ? "pointer" : undefined,
        },
    draggable: !isGroup && draggable,
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
 *  destination countries, auto-laid-out with ELK on every load, overlaid with DFG-05's own saved
 *  drag-and-drop positions + per-node annotations (ตำแหน่งที่จัดเองยังอยู่หลังข้อมูลอัปเดต — the overlay is
 *  keyed by each node's own stable id, so it still applies to the same nodes after the graph is
 *  recomputed). A "transfer" edge in warning colour is a foreign recipient with no ROPA-08 transfer basis
 *  logged (DFG-03's own acceptance criterion). */
export function DataFlowContent() {
  const t = useTranslations("dataflow");
  const canRead = usePermission("ropa.dataflow.read");
  const canEdit = usePermission("ropa.dataflow.update");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const router = useRouter();
  const [legalEntityId, setLegalEntityId] = useState<string>();
  const [view, setView] = useState<DataflowView>("department");

  const entities = useLegalEntities(client);
  const graph = useDataflowGraph(client, { legal_entity_id: legalEntityId, view });
  // RRA-11: the same tenant-wide high-risk set the RoPA activity list highlights, matched here against
  // each activity node's own id ("activity:<uuid>") — "ไฮไลต์...ทั้งในรายการและแผนผัง".
  const highRisk = useHighRiskActivityScores(client);
  const highRiskIds = useMemo(() => new Set((highRisk.data ?? []).map((s) => s.activity_id)), [highRisk.data]);

  // DFG-05: drag & drop editor — editMode gates dragging/annotating at all; overlay is the working copy
  // (dragged positions + notes, not yet necessarily saved), history/historyIdx its own undo/redo stack,
  // savedOverlay the last-known-saved baseline (what "dirty" is measured against).
  const layoutQuery = useDataflowLayout(client, { legal_entity_id: legalEntityId, view });
  const saveLayout = useSaveDataflowLayout(client);
  const resetLayout = useResetDataflowLayout(client);
  const [editMode, setEditMode] = useState(false);
  const [overlay, setOverlay] = useState<Overlay>(EMPTY_OVERLAY);
  const [history, setHistory] = useState<Overlay[]>([EMPTY_OVERLAY]);
  const [historyIdx, setHistoryIdx] = useState(0);
  const [savedOverlay, setSavedOverlay] = useState<Overlay>(EMPTY_OVERLAY);
  const [selectedNodeId, setSelectedNodeId] = useState<string>();

  useEffect(() => {
    if (!layoutQuery.data) return;
    const positions: Overlay["positions"] = {};
    for (const p of layoutQuery.data.positions) positions[p.node_id] = { x: p.x, y: p.y };
    const notes: Overlay["notes"] = {};
    for (const n of layoutQuery.data.notes) notes[n.node_id] = n.text;
    const loaded: Overlay = { positions, notes };
    setOverlay(loaded);
    setSavedOverlay(loaded);
    setHistory([loaded]);
    setHistoryIdx(0);
    setSelectedNodeId(undefined);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- re-seed only when the server's own overlay for this (legal_entity, view) arrives, not on every local edit
  }, [layoutQuery.data]);

  const dirty = JSON.stringify(overlay) !== JSON.stringify(savedOverlay);

  function pushOverlay(next: Overlay) {
    setOverlay(next);
    setHistory((h) => [...h.slice(0, historyIdx + 1), next]);
    setHistoryIdx((i) => i + 1);
  }

  function undo() {
    if (historyIdx === 0) return;
    const idx = historyIdx - 1;
    setHistoryIdx(idx);
    setOverlay(history[idx]);
  }

  function redo() {
    if (historyIdx >= history.length - 1) return;
    const idx = historyIdx + 1;
    setHistoryIdx(idx);
    setOverlay(history[idx]);
  }

  const [noteDraft, setNoteDraft] = useState("");
  useEffect(() => {
    setNoteDraft(selectedNodeId ? overlay.notes[selectedNodeId] ?? "" : "");
  }, [selectedNodeId, overlay.notes]);

  function commitNote() {
    if (!selectedNodeId) return;
    const notes = { ...overlay.notes };
    if (noteDraft.trim()) notes[selectedNodeId] = noteDraft.trim();
    else delete notes[selectedNodeId];
    pushOverlay({ positions: overlay.positions, notes });
  }

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
        const activityId = activityIdOf(n.id);
        // DFG-05: a saved position overrides the auto-layout's own x/y for that exact node id — the
        // acceptance criterion itself, surviving the ELK recompute a data/view change triggers.
        const override = overlay.positions[id];
        const effectivePos = override ? { ...pos, x: override.x, y: override.y } : pos;
        nodes.push(toFlowNode(n, effectivePos, !!activityId && highRiskIds.has(activityId), editMode, overlay.notes[id]));
        if (isSwimlane(n.type)) {
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
  }, [gNodes, gEdges, highRiskIds, overlay, editMode]);

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
        <label>
          <span className="block text-slate-600">{t("view.label")}</span>
          <select className={INPUT} value={view} onChange={(e) => setView(e.target.value as DataflowView)} data-testid="dataflow-view-picker">
            <option value="department">{t("view.department")}</option>
            <option value="company">{t("view.company")}</option>
            <option value="data_category">{t("view.dataCategory")}</option>
          </select>
        </label>
        <div className="flex items-center gap-3 text-slate-500">
          {graph.data ? <p>{t("activityCount", { count: graph.data.activity_count })}</p> : null}
          {graph.data ? (
            <p data-testid="dataflow-last-updated">
              {graph.isFetching ? t("updating") : t("lastUpdated", { time: new Date(graph.data.generated_at).toLocaleTimeString() })}
            </p>
          ) : null}
          <button
            type="button"
            className="rounded-md border border-slate-300 bg-white px-2 py-1 text-xs hover:bg-slate-50"
            onClick={() => graph.refetch()}
          >
            {t("refreshNow")}
          </button>
        </div>
      </div>

      <Legend />

      <ExportPanel legalEntityId={legalEntityId} view={view} />

      <SnapshotsPanel legalEntityId={legalEntityId} view={view} />

      {canEdit && (
        <LayoutEditorPanel
          editMode={editMode}
          onToggleEditMode={() => setEditMode((v) => !v)}
          dirty={dirty}
          canUndo={historyIdx > 0}
          canRedo={historyIdx < history.length - 1}
          onUndo={undo}
          onRedo={redo}
          saving={saveLayout.isPending}
          onSave={() => {
            saveLayout.mutate(
              {
                filter: { legal_entity_id: legalEntityId, view },
                rowVersion: layoutQuery.data?.row_version ?? 0,
                positions: Object.entries(overlay.positions).map(([node_id, p]) => ({ node_id, x: p.x, y: p.y })),
                notes: Object.entries(overlay.notes).map(([node_id, text]) => ({ node_id, text })),
              },
              { onSuccess: () => setSavedOverlay(overlay) },
            );
          }}
          resetting={resetLayout.isPending}
          onReset={() => {
            resetLayout.mutate(
              { legal_entity_id: legalEntityId, view },
              {
                onSuccess: () => {
                  setOverlay(EMPTY_OVERLAY);
                  setSavedOverlay(EMPTY_OVERLAY);
                  setHistory([EMPTY_OVERLAY]);
                  setHistoryIdx(0);
                  setSelectedNodeId(undefined);
                },
              },
            );
          }}
        />
      )}

      {editMode && selectedNodeId && (
        <div className="flex items-end gap-2 rounded-md border border-sky-200 bg-sky-50 p-3 text-xs" data-testid="dataflow-note-panel">
          <label className="flex-1">
            <span className="block text-slate-600">{t("layoutEditor.noteFor", { node: selectedNodeId })}</span>
            <textarea
              className="mt-0.5 w-full rounded-md border border-slate-300 bg-white px-2 py-1"
              rows={2}
              value={noteDraft}
              onChange={(e) => setNoteDraft(e.target.value)}
              data-testid="dataflow-note-textarea"
            />
          </label>
          <Button onClick={commitNote} data-testid="dataflow-note-save">
            {t("layoutEditor.saveNote")}
          </Button>
          <button type="button" className="rounded-md border border-slate-300 bg-white px-3 py-1.5 hover:bg-slate-50" onClick={() => setSelectedNodeId(undefined)}>
            {t("layoutEditor.closeNote")}
          </button>
        </div>
      )}

      {graph.isPending || !laidOut ? (
        <p className="text-slate-500">{t("loading")}</p>
      ) : graph.isError ? (
        <p className="text-red-700">{t("loadError")}</p>
      ) : flowNodes.length === 0 ? (
        <p className="rounded-md bg-amber-50 p-3 text-amber-800">{t("empty")}</p>
      ) : (
        <div style={{ height: "70vh" }} className="rounded-md border border-slate-200 bg-white" data-testid="dataflow-canvas">
          <ReactFlow
            nodes={flowNodes}
            edges={flowEdges}
            fitView
            nodesDraggable={editMode}
            nodesConnectable={false}
            edgesFocusable={false}
            onNodeDragStop={(_, node) => {
              if (!editMode) return;
              const data = node.data as { node?: DataflowNode };
              if (!data.node || isSwimlane(data.node.type)) return;
              pushOverlay({ positions: { ...overlay.positions, [node.id]: { x: node.position.x, y: node.position.y } }, notes: overlay.notes });
            }}
            onNodeClick={(_, node) => {
              const data = node.data as { node?: DataflowNode; isHighRisk?: boolean };
              if (editMode) {
                if (!data.node || isSwimlane(data.node.type)) return;
                setSelectedNodeId(node.id);
                return;
              }
              const agreementId = data.node?.agreement_ids?.[0];
              if (agreementId) {
                router.push(`/agreements/${agreementId}`);
                return;
              }
              // RRA-11: "ลิงก์ไปแก้ไข" — a high-risk activity node clicks through to its own detail page.
              const activityId = data.isHighRisk ? activityIdOf(node.id) : undefined;
              if (activityId) router.push(`/ropa/activities/${activityId}`);
            }}
          >
            <Background />
            <Controls showInteractive={false} />
          </ReactFlow>
        </div>
      )}
    </main>
  );
}

/** DFG-05: the drag & drop editor's own toolbar — enter/exit edit mode, undo/redo the working overlay,
 *  save it (ETag-guarded against the loaded layout's own row_version), or "คืนค่าเดิม" (discard it). */
function LayoutEditorPanel(props: {
  editMode: boolean;
  onToggleEditMode: () => void;
  dirty: boolean;
  canUndo: boolean;
  canRedo: boolean;
  onUndo: () => void;
  onRedo: () => void;
  saving: boolean;
  onSave: () => void;
  resetting: boolean;
  onReset: () => void;
}) {
  const t = useTranslations("dataflow.layoutEditor");
  return (
    <div className="flex flex-wrap items-center gap-2 rounded-md border border-slate-200 bg-white p-3 text-xs" data-testid="dataflow-layout-editor">
      <button
        type="button"
        className={`rounded-md border px-3 py-1.5 ${props.editMode ? "border-sky-400 bg-sky-100 text-sky-800" : "border-slate-300 bg-white hover:bg-slate-50"}`}
        onClick={props.onToggleEditMode}
        data-testid="dataflow-edit-toggle"
      >
        {props.editMode ? t("exitEdit") : t("enterEdit")}
      </button>
      {props.editMode && (
        <>
          <button
            type="button"
            className="rounded-md border border-slate-300 bg-white px-3 py-1.5 hover:bg-slate-50 disabled:opacity-40"
            disabled={!props.canUndo}
            onClick={props.onUndo}
            data-testid="dataflow-undo"
          >
            {t("undo")}
          </button>
          <button
            type="button"
            className="rounded-md border border-slate-300 bg-white px-3 py-1.5 hover:bg-slate-50 disabled:opacity-40"
            disabled={!props.canRedo}
            onClick={props.onRedo}
            data-testid="dataflow-redo"
          >
            {t("redo")}
          </button>
          <Button disabled={!props.dirty || props.saving} onClick={props.onSave} data-testid="dataflow-save-layout">
            {props.saving ? t("saving") : t("save")}
          </Button>
          <button
            type="button"
            className="rounded-md border border-amber-300 bg-amber-50 px-3 py-1.5 text-amber-800 hover:bg-amber-100 disabled:opacity-40"
            disabled={props.resetting}
            onClick={props.onReset}
            data-testid="dataflow-reset-layout"
          >
            {t("reset")}
          </button>
          {props.dirty && <span className="text-amber-700">{t("unsaved")}</span>}
        </>
      )}
    </div>
  );
}

function Legend() {
  const t = useTranslations("dataflow");
  const items: { color: string; label: string }[] = [
    { color: NODE_COLOR.activity, label: t("legend.activity") },
    { color: NODE_COLOR.recipient, label: t("legend.recipient") },
    { color: NODE_COLOR.country, label: t("legend.country") },
    { color: NODE_COLOR.disposal, label: t("legend.disposal") },
    { color: "#dc2626", label: t("legend.missingAgreement") },
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

/** DFG-07: publish the current live diagram as a new, immutable version, and browse past published
 *  versions — "ปุ่ม publish + ดูเวอร์ชันเก่า". The publish button is shown only to a caller holding
 *  ropa.dataflow.publish (DPO, the module's own actor); the version list itself is visible to any reader. */
function SnapshotsPanel({ legalEntityId, view }: { legalEntityId?: string; view: DataflowView }) {
  const t = useTranslations("dataflow.snapshots");
  const canPublish = usePermission("ropa.dataflow.publish");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const [open, setOpen] = useState(false);
  const snapshots = useDataflowSnapshots(client, { legal_entity_id: legalEntityId, view });
  const publish = useDataflowPublishSnapshot(client);

  return (
    <div className="rounded-md border border-slate-200 bg-white p-3 text-xs" data-testid="dataflow-snapshots-panel">
      <div className="flex items-center justify-between gap-2">
        <button type="button" className="text-left font-medium text-sky-700" onClick={() => setOpen((v) => !v)} data-testid="dataflow-snapshots-toggle">
          {open ? "▾ " : "▸ "}{t("toggle")}
        </button>
        {canPublish && (
          <Button
            disabled={publish.isPending}
            onClick={() => publish.mutate({ legal_entity_id: legalEntityId, view })}
            data-testid="dataflow-publish-button"
          >
            {t("publish")}
          </Button>
        )}
      </div>
      {publish.isError && <p className="mt-1 text-red-700">{t("publishError")}</p>}
      {open && (
        snapshots.isPending ? (
          <p className="mt-2 text-slate-500">{t("loading")}</p>
        ) : snapshots.isError ? (
          <p className="mt-2 text-red-700">{t("loadError")}</p>
        ) : (snapshots.data ?? []).length === 0 ? (
          <p className="mt-2 text-slate-500">{t("empty")}</p>
        ) : (
          <ul className="mt-2 divide-y divide-slate-100" data-testid="dataflow-snapshots-list">
            {snapshots.data!.map((s) => (
              <li key={s.id} className="flex items-center justify-between gap-2 py-1">
                <span>{t("versionLabel", { n: s.version_no })}</span>
                <span className="text-slate-500">{new Date(s.published_at).toLocaleString()}</span>
              </li>
            ))}
          </ul>
        )
      )}
    </div>
  );
}

/** DFG-08: export the diagram as a standalone image (svg/png/pdf, sized for print — always A3/A4/Letter
 *  landscape) and the written companion report (pdf/docx/html, th/en) — both plain `<a href>`s through the
 *  BFF, the same pattern dpiaReportHref/documentExportHref already use; the browser downloads the file
 *  directly, no client-side fetch needed. */
function ExportPanel({ legalEntityId, view }: { legalEntityId?: string; view: DataflowView }) {
  const t = useTranslations("dataflow");
  const [format, setFormat] = useState<"svg" | "png" | "pdf">("svg");
  const [paper, setPaper] = useState<"a3" | "a4" | "letter">("a3");
  const [reportLang, setReportLang] = useState<"th" | "en">("th");
  const [reportFormat, setReportFormat] = useState<"pdf" | "docx" | "html">("pdf");

  return (
    <div className="flex flex-wrap items-end gap-4 rounded-md border border-slate-200 bg-white p-3 text-xs">
      <div className="flex items-end gap-2">
        <label>
          <span className="block text-slate-600">{t("export.format")}</span>
          <select className={INPUT} value={format} onChange={(e) => setFormat(e.target.value as typeof format)}>
            <option value="svg">SVG</option>
            <option value="png">PNG</option>
            <option value="pdf">PDF</option>
          </select>
        </label>
        {format !== "svg" ? (
          <label>
            <span className="block text-slate-600">{t("export.paper")}</span>
            <select className={INPUT} value={paper} onChange={(e) => setPaper(e.target.value as typeof paper)}>
              <option value="a3">A3</option>
              <option value="a4">A4</option>
              <option value="letter">Letter</option>
            </select>
          </label>
        ) : null}
        <a
          className="rounded-md border border-sky-300 bg-sky-50 px-3 py-1.5 text-sky-800 hover:bg-sky-100"
          href={dataflowExportHref("/api/bff", format, paper, legalEntityId, view)}
          target="_blank"
          rel="noreferrer"
          data-testid="dataflow-export-image"
        >
          {t("export.downloadImage")}
        </a>
      </div>

      <span className="border-l border-slate-300 self-stretch" />

      <div className="flex items-end gap-2">
        <label>
          <span className="block text-slate-600">{t("export.language")}</span>
          <select className={INPUT} value={reportLang} onChange={(e) => setReportLang(e.target.value as typeof reportLang)}>
            <option value="th">ไทย</option>
            <option value="en">English</option>
          </select>
        </label>
        <label>
          <span className="block text-slate-600">{t("export.reportFormat")}</span>
          <select className={INPUT} value={reportFormat} onChange={(e) => setReportFormat(e.target.value as typeof reportFormat)}>
            <option value="pdf">PDF</option>
            <option value="docx">Word</option>
            <option value="html">HTML</option>
          </select>
        </label>
        <a
          className="rounded-md border border-slate-300 bg-white px-3 py-1.5 hover:bg-slate-50"
          href={dataflowReportHref("/api/bff", reportLang, reportFormat, legalEntityId, view)}
          target="_blank"
          rel="noreferrer"
          data-testid="dataflow-export-report"
        >
          {t("export.downloadReport")}
        </a>
      </div>
    </div>
  );
}
