import { useQuery } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type DataflowGraph = components["schemas"]["DataflowGraph"];
export type DataflowNode = components["schemas"]["DataflowNode"];
export type DataflowEdge = components["schemas"]["DataflowEdge"];

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
