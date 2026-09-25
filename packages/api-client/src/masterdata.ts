import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type MasterDataKind = components["schemas"]["MasterDataKind"];
export type MasterDataItem = components["schemas"]["MasterDataItem"];
export type MasterDataInput = components["schemas"]["MasterDataInput"];

/** GET /admin/v1/org/master-data/{kind} (ORG-07): platform defaults, then the tenant's own. */
export function useMasterData(client: ApiClient, kind: MasterDataKind) {
  return useQuery({
    queryKey: ["org", "master-data", kind],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/org/master-data/{kind}", { params: { path: { kind } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** Create, update (If-Match) and delete the tenant's own entries of a kind. */
export function useMasterDataMutations(client: ApiClient, kind: MasterDataKind) {
  const qc = useQueryClient();
  const refresh = () => qc.invalidateQueries({ queryKey: ["org", "master-data", kind] });
  return {
    create: useMutation({
      mutationFn: async (input: MasterDataInput) => {
        const { data, error } = await client.POST("/admin/v1/org/master-data/{kind}", { params: { path: { kind } }, body: input });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    update: useMutation({
      mutationFn: async (v: { item: MasterDataItem; input: MasterDataInput }) => {
        const { data, error } = await client.PATCH("/admin/v1/org/master-data/{kind}/{id}", {
          params: { path: { kind, id: v.item.id! }, header: { "If-Match": `"${v.item.row_version}"` } },
          body: v.input,
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    remove: useMutation({
      mutationFn: async (item: MasterDataItem) => {
        const { error } = await client.DELETE("/admin/v1/org/master-data/{kind}/{id}", {
          params: { path: { kind, id: item.id! }, header: { "If-Match": `"${item.row_version}"` } },
        });
        if (error) throw error;
      },
      onSuccess: refresh,
    }),
  };
}
