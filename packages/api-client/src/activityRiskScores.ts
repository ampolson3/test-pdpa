import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type ActivityRiskScore = components["schemas"]["ActivityRiskScore"];
export type RiskFactorContribution = components["schemas"]["RiskFactorContribution"];

const scoreKey = (activityId: string) => ["risk", "activity-score", activityId] as const;

/** GET /admin/v1/ropa/activities/{id}/risk-score — the last computed score, without recomputing. */
export function useActivityRiskScore(client: ApiClient, activityId: string | undefined) {
  return useQuery({
    queryKey: scoreKey(activityId ?? ""),
    enabled: !!activityId,
    retry: false,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/ropa/activities/{id}/risk-score", { params: { path: { id: activityId! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** POST /admin/v1/ropa/activities/{id}/risk-score (RRA-01) — computes a fresh score from the activity's
 *  current RoPA data and records it. */
export function useScoreActivity(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (activityId: string) => {
      const { data, error } = await client.POST("/admin/v1/ropa/activities/{id}/risk-score", { params: { path: { id: activityId } } });
      if (error) throw error;
      return data;
    },
    onSuccess: (data) => qc.invalidateQueries({ queryKey: scoreKey(data.activity_id) }),
  });
}
