import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type RiskRegisterEntry = components["schemas"]["RiskRegisterEntry"];
export type RiskRegisterInput = components["schemas"]["RiskRegisterInput"];

const riskRegisterKey = ["risk", "register"] as const;
const riskKey = (id: string) => [...riskRegisterKey, id] as const;

/** GET /admin/v1/risk/risks (RRA-09) — every risk regardless of source (dpia/activity/vendor/manual/...),
 *  in one shared register. */
export function useRiskRegister(client: ApiClient, filter: { status?: string; source_type?: string; owner_user_id?: string } = {}) {
  return useInfiniteQuery({
    queryKey: [...riskRegisterKey, filter],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/risk/risks", {
        params: {
          query: {
            status: filter.status as never,
            source_type: filter.source_type as never,
            owner_user_id: filter.owner_user_id,
            cursor: pageParam,
            limit: 50,
          },
        },
      });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

/** GET /admin/v1/risk/risks/{id}. */
export function useRiskRegisterEntry(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: riskKey(id ?? ""),
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/risk/risks/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** Create (no id) or update (id + row_version as If-Match) a risk register entry. Updating never moves
 *  the entry's own activity/asset/vendor link — those are set only once, at creation. */
export function useSaveRiskRegisterEntry(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id?: string; rowVersion?: number; input: RiskRegisterInput }) => {
      if (!v.id) {
        const { data, error } = await client.POST("/admin/v1/risk/risks", { body: v.input });
        if (error) throw error;
        return data;
      }
      const { data, error } = await client.PUT("/admin/v1/risk/risks/{id}", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: v.input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: riskRegisterKey });
      if (data?.id) qc.invalidateQueries({ queryKey: riskKey(data.id) });
    },
  });
}
