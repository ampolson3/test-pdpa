import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type RiskGapRule = components["schemas"]["RiskGapRule"];
export type RiskGapFinding = components["schemas"]["RiskGapFinding"];
export type RiskRemediateFindingInput = components["schemas"]["RiskRemediateFindingInput"];
export type RiskComplianceScore = components["schemas"]["RiskComplianceScore"];
export type RiskOrgUnitComplianceScore = components["schemas"]["RiskOrgUnitComplianceScore"];

const activityFindingsKey = (activityId: string) => ["risk", "gap-findings", activityId] as const;
const activityScoreKey = (activityId: string) => ["risk", "compliance-score", activityId] as const;
const orgUnitScoreKey = (orgUnitId: string) => ["risk", "compliance-score", "org-unit", orgUnitId] as const;
const scoreReportKey = ["risk", "compliance-score", "report"] as const;

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
      qc.invalidateQueries({ queryKey: activityScoreKey(activityId) });
      qc.invalidateQueries({ queryKey: scoreReportKey });
    },
  });
}

/** GET /admin/v1/ropa/activities/{id}/compliance-score (RRA-13) — the acceptance criterion itself: a
 *  score that explains exactly which still-open gap it was deducted for. */
export function useActivityComplianceScore(client: ApiClient, activityId: string | undefined) {
  return useQuery({
    queryKey: activityScoreKey(activityId ?? ""),
    enabled: !!activityId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/ropa/activities/{id}/compliance-score", { params: { path: { id: activityId! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** GET /admin/v1/risk/compliance-score/{orgUnitId} (RRA-13) — one org unit's own rollup, every activity
 *  under it with its own explained score, averaged. */
export function useOrgUnitComplianceScore(client: ApiClient, orgUnitId: string | undefined) {
  return useQuery({
    queryKey: orgUnitScoreKey(orgUnitId ?? ""),
    enabled: !!orgUnitId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/risk/compliance-score/{orgUnitId}", { params: { path: { orgUnitId: orgUnitId! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** GET /admin/v1/risk/compliance-score (RRA-13) — the module description's own "ตารางคะแนนรายหน่วยงาน":
 *  every org unit's averaged score, for comparing and tracking progress. */
export function useComplianceScoreReport(client: ApiClient) {
  return useQuery({
    queryKey: scoreReportKey,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/risk/compliance-score", {});
      if (error) throw error;
      return data.data;
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
