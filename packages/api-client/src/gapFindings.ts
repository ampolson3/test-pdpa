import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type RiskGapRule = components["schemas"]["RiskGapRule"];
export type RiskGapFinding = components["schemas"]["RiskGapFinding"];
export type RiskRemediateFindingInput = components["schemas"]["RiskRemediateFindingInput"];

const activityFindingsKey = (activityId: string) => ["risk", "gap-findings", activityId] as const;

/** GET /admin/v1/risk/gap-rules (RRA-04) — every active legal-gap rule, with the article it cites. */
export function useGapRules(client: ApiClient) {
  return useQuery({
    queryKey: ["risk", "gap-rules"],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/risk/gap-rules", {});
      if (error) throw error;
      return data.data;
    },
  });
}

/** GET /admin/v1/risk/gap-findings (RRA-04) — the tenant-wide open-findings register. */
export function useOpenGapFindings(client: ApiClient) {
  return useQuery({
    queryKey: ["risk", "gap-findings", "open"],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/risk/gap-findings", {});
      if (error) throw error;
      return data.data;
    },
  });
}

/** GET /admin/v1/ropa/activities/{id}/gap-findings — this activity's own findings, as last analyzed. */
export function useActivityGapFindings(client: ApiClient, activityId: string | undefined) {
  return useQuery({
    queryKey: activityFindingsKey(activityId ?? ""),
    enabled: !!activityId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/ropa/activities/{id}/gap-findings", { params: { path: { id: activityId! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** POST /admin/v1/ropa/activities/{id}/gap-analysis (RRA-04) — re-run every active rule against this
 *  activity's current RoPA data now; the acceptance criterion itself. */
export function useAnalyzeActivity(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (activityId: string) => {
      const { data, error } = await client.POST("/admin/v1/ropa/activities/{id}/gap-analysis", { params: { path: { id: activityId } } });
      if (error) throw error;
      return { activityId, findings: data.data };
    },
    onSuccess: ({ activityId }) => {
      qc.invalidateQueries({ queryKey: activityFindingsKey(activityId) });
      qc.invalidateQueries({ queryKey: ["risk", "gap-findings", "open"] });
    },
  });
}

/** POST /admin/v1/risk/gap-findings/{id}/remediate (RRA-07) — open a tracked dpo.tasks job against an
 *  open finding, with an assignee, due date and priority. */
export function useRemediateGapFinding(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, activityId, input }: { id: string; activityId: string; input: RiskRemediateFindingInput }) => {
      const { data, error } = await client.POST("/admin/v1/risk/gap-findings/{id}/remediate", {
        params: { path: { id } },
        body: input,
      });
      if (error) throw error;
      return { activityId, finding: data };
    },
    onSuccess: ({ activityId }) => {
      qc.invalidateQueries({ queryKey: activityFindingsKey(activityId) });
      qc.invalidateQueries({ queryKey: ["risk", "gap-findings", "open"] });
    },
  });
}
