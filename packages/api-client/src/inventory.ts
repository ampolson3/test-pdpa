import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type DataInventoryItem = components["schemas"]["DataInventoryItem"];
export type DataInventoryItemInput = components["schemas"]["DataInventoryItemInput"];
export type DataInventorySource = components["schemas"]["DataInventorySource"];

const inventoryKey = ["ropa", "data-inventory"] as const;

/** GET /admin/v1/ropa/data-inventory (ROPA-01), newest first. sensitive_only searches every department. */
export function useDataInventory(client: ApiClient, filter: { asset_id?: string; org_unit_id?: string; data_category_id?: string; sensitive_only?: boolean } = {}) {
  return useInfiniteQuery({
    queryKey: [...inventoryKey, filter],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/ropa/data-inventory", {
        params: { query: { asset_id: filter.asset_id, org_unit_id: filter.org_unit_id, data_category_id: filter.data_category_id, sensitive_only: filter.sensitive_only, cursor: pageParam, limit: 50 } },
      });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

/** Create (no id) or update (id + row_version as If-Match) a data inventory entry. */
export function useSaveDataInventoryItem(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id?: string; rowVersion?: number; input: DataInventoryItemInput }) => {
      if (!v.id) {
        const { data, error } = await client.POST("/admin/v1/ropa/data-inventory", { body: v.input });
        if (error) throw error;
        return data;
      }
      const { data, error } = await client.PATCH("/admin/v1/ropa/data-inventory/{id}", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: v.input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: inventoryKey }),
  });
}
