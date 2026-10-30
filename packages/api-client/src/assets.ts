import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type Asset = components["schemas"]["Asset"];
export type AssetInput = components["schemas"]["AssetInput"];
export type AssetType = components["schemas"]["AssetType"];

const assetsKey = ["ropa", "assets"] as const;

/** GET /admin/v1/ropa/assets (ROPA-02), newest first. */
export function useAssets(client: ApiClient, filter: { asset_type?: AssetType; org_unit_id?: string; status?: "active" | "retired"; q?: string } = {}) {
  return useInfiniteQuery({
    queryKey: [...assetsKey, filter],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/ropa/assets", {
        params: { query: { asset_type: filter.asset_type, org_unit_id: filter.org_unit_id, status: filter.status, q: filter.q || undefined, cursor: pageParam, limit: 50 } },
      });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

/** Create (no id) or update (id + row_version as If-Match) an asset. */
export function useSaveAsset(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id?: string; rowVersion?: number; input: AssetInput }) => {
      if (!v.id) {
        const { data, error } = await client.POST("/admin/v1/ropa/assets", { body: v.input });
        if (error) throw error;
        return data;
      }
      const { data, error } = await client.PATCH("/admin/v1/ropa/assets/{id}", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: v.input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: assetsKey }),
  });
}
