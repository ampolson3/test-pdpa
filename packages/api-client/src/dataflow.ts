import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type DataflowGraph = components["schemas"]["DataflowGraph"];
export type DataflowNode = components["schemas"]["DataflowNode"];
export type DataflowEdge = components["schemas"]["DataflowEdge"];
export type DataflowSnapshot = components["schemas"]["DataflowSnapshot"];
export type DataflowSnapshotSummary = components["schemas"]["DataflowSnapshotSummary"];

/** GET /admin/v1/dataflow/graph (DFG-01/DFG-03) — the data flow diagram, composed live from RoPA on every
 *  read; no pagination (the whole tenant diagram is meant to be viewed as one graph). DFG-02's "update
 *  without regenerating" needs no server-side push: the backend never caches the graph, so the 30s poll
 *  below is enough to show a RoPA edit made elsewhere without the viewer doing anything. */
export function useDataflowGraph(client: ApiClient, filter: { legal_entity_id?: string } = {}) {
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
): string {
  const q = new URLSearchParams({ format, paper });
  if (legalEntityId) q.set("legal_entity_id", legalEntityId);
  return `${bffBaseUrl}/admin/v1/dataflow/graph/export?${q}`;
}

/** GET /admin/v1/dataflow/report (DFG-08) — the written companion report ("รายงานประกอบ") to the image
 *  export, built from the same live graph. */
export function dataflowReportHref(
  bffBaseUrl: string,
  language: "th" | "en",
  format: "pdf" | "docx" | "html",
  legalEntityId?: string,
): string {
  const q = new URLSearchParams({ language, format });
  if (legalEntityId) q.set("legal_entity_id", legalEntityId);
  return `${bffBaseUrl}/admin/v1/dataflow/report?${q}`;
}

/** GET /admin/v1/dataflow/snapshots (DFG-07): every published version of the diagram, newest first —
 *  "ดูแผนผังที่ publish ย้อนหลังได้". */
export function useDataflowSnapshots(client: ApiClient, filter: { legal_entity_id?: string } = {}) {
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
    mutationFn: async (filter: { legal_entity_id?: string } = {}) => {
      const { data, error } = await client.POST("/admin/v1/dataflow/snapshots", { params: { query: filter } });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["dataflow", "snapshots"] }),
  });
}
