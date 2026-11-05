import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type DataflowGraph = components["schemas"]["DataflowGraph"];
export type DataflowNode = components["schemas"]["DataflowNode"];
export type DataflowEdge = components["schemas"]["DataflowEdge"];
export type DataflowSnapshot = components["schemas"]["DataflowSnapshot"];
export type DataflowSnapshotSummary = components["schemas"]["DataflowSnapshotSummary"];
export type DataflowView = components["schemas"]["DataflowView"];
export type DataflowLayout = components["schemas"]["DataflowLayout"];
export type DataflowNodePosition = components["schemas"]["DataflowNodePosition"];
export type DataflowNodeNote = components["schemas"]["DataflowNodeNote"];

/** GET /admin/v1/dataflow/graph (DFG-01/DFG-03) — the data flow diagram, composed live from RoPA on every
 *  read; no pagination (the whole tenant diagram is meant to be viewed as one graph). DFG-02's "update
 *  without regenerating" needs no server-side push: the backend never caches the graph, so the 30s poll
 *  below is enough to show a RoPA edit made elsewhere without the viewer doing anything. DFG-06's own
 *  `view` switches the swimlane grouping (department/company/data_category) — the same nodes/edges
 *  otherwise, so switching it needs no extra invalidation beyond the query key already carrying it. */
export function useDataflowGraph(client: ApiClient, filter: { legal_entity_id?: string; view?: DataflowView } = {}) {
  return useQuery({
    queryKey: ["dataflow", "graph", filter],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dataflow/graph", {
        params: { query: filter },
      });
      if (error) throw error;
      return data;
    },
    refetchInterval: 30_000,
  });
}

/** GET /admin/v1/dataflow/graph/export (DFG-08) — a plain href, not a query/mutation: the browser downloads
 *  the SVG/PNG/PDF file directly, the same pattern dpiaReportHref/documentExportHref already use. `paper` is
 *  ignored server-side for svg. */
export function dataflowExportHref(
  bffBaseUrl: string,
  format: "svg" | "png" | "pdf",
  paper: "a3" | "a4" | "letter" = "a3",
  legalEntityId?: string,
  view?: DataflowView,
): string {
  const q = new URLSearchParams({ format, paper });
  if (legalEntityId) q.set("legal_entity_id", legalEntityId);
  if (view) q.set("view", view);
  return `${bffBaseUrl}/admin/v1/dataflow/graph/export?${q}`;
}

/** GET /admin/v1/dataflow/report (DFG-08) — the written companion report ("รายงานประกอบ") to the image
 *  export, built from the same live graph. */
export function dataflowReportHref(
  bffBaseUrl: string,
  language: "th" | "en",
  format: "pdf" | "docx" | "html",
  legalEntityId?: string,
  view?: DataflowView,
): string {
  const q = new URLSearchParams({ language, format });
  if (legalEntityId) q.set("legal_entity_id", legalEntityId);
  if (view) q.set("view", view);
  return `${bffBaseUrl}/admin/v1/dataflow/report?${q}`;
}

/** GET /admin/v1/dataflow/snapshots (DFG-07): every published version of the diagram, newest first —
 *  "ดูแผนผังที่ publish ย้อนหลังได้". */
export function useDataflowSnapshots(client: ApiClient, filter: { legal_entity_id?: string; view?: DataflowView } = {}) {
  return useQuery({
    queryKey: ["dataflow", "snapshots", filter],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dataflow/snapshots", { params: { query: filter } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** GET /admin/v1/dataflow/snapshots/{id} (DFG-07): one past published version's full diagram. */
export function useDataflowSnapshot(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: ["dataflow", "snapshots", "detail", id ?? ""],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dataflow/snapshots/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** POST /admin/v1/dataflow/snapshots (DFG-07): freeze the current live diagram as a new published version. */
export function useDataflowPublishSnapshot(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (filter: { legal_entity_id?: string; view?: DataflowView } = {}) => {
      const { data, error } = await client.POST("/admin/v1/dataflow/snapshots", { params: { query: filter } });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["dataflow", "snapshots"] }),
  });
}

/** GET /admin/v1/dataflow/layout (DFG-05): the caller's own saved drag-and-drop overlay (node positions +
 *  per-node annotations) for one (legal_entity, view) diagram — the empty defaults (row_version 0) if
 *  never saved, so the first save can carry If-Match: "0". */
export function useDataflowLayout(client: ApiClient, filter: { legal_entity_id?: string; view?: DataflowView } = {}) {
  return useQuery({
    queryKey: ["dataflow", "layout", filter],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dataflow/layout", { params: { query: filter } });
      if (error) throw error;
      return data;
    },
  });
}

/** PUT /admin/v1/dataflow/layout (DFG-05): a full replace of the overlay — "ตำแหน่งที่จัดเองยังอยู่หลัง
 *  ข้อมูลอัปเดต". */
export function useSaveDataflowLayout(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      filter?: { legal_entity_id?: string; view?: DataflowView };
      rowVersion: number;
      positions: DataflowNodePosition[];
      notes: DataflowNodeNote[];
    }) => {
      const { data, error } = await client.PUT("/admin/v1/dataflow/layout", {
        params: { query: input.filter ?? {}, header: { "If-Match": `"${input.rowVersion}"` } },
        body: { positions: input.positions, notes: input.notes },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (_data, input) => qc.invalidateQueries({ queryKey: ["dataflow", "layout", input.filter ?? {}] }),
  });
}

/** POST /admin/v1/dataflow/layout/reset (DFG-05): "คืนค่าเดิม" — discard the saved overlay. */
export function useResetDataflowLayout(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (filter: { legal_entity_id?: string; view?: DataflowView } = {}) => {
      const { error } = await client.POST("/admin/v1/dataflow/layout/reset", { params: { query: filter } });
      if (error) throw error;
    },
    onSuccess: (_data, filter) => qc.invalidateQueries({ queryKey: ["dataflow", "layout", filter] }),
  });
}
