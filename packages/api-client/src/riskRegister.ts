import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type RiskRegisterEntry = components["schemas"]["RiskRegisterEntry"];
export type RiskRegisterInput = components["schemas"]["RiskRegisterInput"];
export type RiskAcceptance = components["schemas"]["RiskAcceptance"];
export type RiskAcceptanceInput = components["schemas"]["RiskAcceptanceInput"];

const riskRegisterKey = ["risk", "register"] as const;
const riskKey = (id: string) => [...riskRegisterKey, id] as const;
const acceptancesKey = (riskId: string) => [...riskRegisterKey, riskId, "acceptances"] as const;

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

/** GET /admin/v1/risk/risks/{id}/acceptances (RRA-08) — every acceptance ever recorded for this risk,
 *  newest first. */
export function useAcceptances(client: ApiClient, riskId: string | undefined) {
  return useQuery({
    queryKey: acceptancesKey(riskId ?? ""),
    enabled: !!riskId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/risk/risks/{id}/acceptances", { params: { path: { id: riskId! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** POST /admin/v1/risk/risks/{id}/acceptances — record acceptance of the risk's own residual (reason +
 *  review date). Flips the risk to status=accepted and schedules the one reminder at expires_at. */
export function useRecordAcceptance(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { riskId: string; input: RiskAcceptanceInput }) => {
      const { data, error } = await client.POST("/admin/v1/risk/risks/{id}/acceptances", {
        params: { path: { id: v.riskId } },
        body: v.input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (_data, v) => {
      qc.invalidateQueries({ queryKey: acceptancesKey(v.riskId) });
      qc.invalidateQueries({ queryKey: riskKey(v.riskId) });
      qc.invalidateQueries({ queryKey: riskRegisterKey });
    },
  });
}
