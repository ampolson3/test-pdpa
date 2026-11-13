import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";
import type { FormDraft } from "./forms";

export type DpiaScreeningRule = components["schemas"]["DpiaScreeningRule"];
export type DpiaAssessment = components["schemas"]["DpiaAssessment"];
export type DpiaTemplate = components["schemas"]["DpiaTemplate"];
export type DpiaActivityDescription = components["schemas"]["DpiaActivityDescription"];
export type DpiaNecessity = components["schemas"]["DpiaNecessity"];
export type DpiaAssessmentDiff = components["schemas"]["DpiaAssessmentDiff"];
export type DpiaOpinion = components["schemas"]["DpiaOpinion"];
export type DpiaOpinionRecommendation = components["schemas"]["DpiaOpinionRecommendation"];
export type DpiaConsultation = components["schemas"]["DpiaConsultation"];
export type DpiaConsultationInput = components["schemas"]["DpiaConsultationInput"];
export type DpiaConsultationStakeholderType = components["schemas"]["DpiaConsultationStakeholderType"];
export type DpiaRegistryEntry = components["schemas"]["DpiaRegistryEntry"];
export type DpiaRisk = components["schemas"]["DpiaRisk"];
export type DpiaRiskInput = components["schemas"]["DpiaRiskInput"];
export type DpiaRiskCatalogItem = components["schemas"]["DpiaRiskCatalogItem"];
export type DpiaRiskControl = components["schemas"]["DpiaRiskControl"];
export type DpiaRiskControlInput = components["schemas"]["DpiaRiskControlInput"];
export type DpiaRiskControlStatus = components["schemas"]["DpiaRiskControlStatus"];
export type DpiaLiaAssessment = components["schemas"]["DpiaLiaAssessment"];

/** GET /admin/v1/dpia/assessments/{id}/report (DPIA-15) — a plain href, not a query/mutation: the browser
 *  downloads the PDF/Word file through this link directly, the same pattern documentExportHref uses. */
export function dpiaReportHref(bffBaseUrl: string, assessmentId: string, language: "th" | "en", format: "pdf" | "docx"): string {
  const q = new URLSearchParams({ language, format });
  return `${bffBaseUrl}/admin/v1/dpia/assessments/${assessmentId}/report?${q}`;
}

const rulesKey = ["dpia", "screening-rules"] as const;
const assessmentsKey = ["dpia", "assessments"] as const;
const templatesKey = ["dpia", "templates"] as const;

/** GET /admin/v1/dpia/screening-rules (DPIA-02) — the tenant's own thresholds, or the default. */
export function useScreeningRules(client: ApiClient) {
  return useQuery({
    queryKey: rulesKey,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dpia/screening-rules", {});
      if (error) throw error;
      return data;
    },
  });
}

export function useSaveScreeningRules(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: { min_factors: number; min_score?: number | null }) => {
      const { data, error } = await client.PUT("/admin/v1/dpia/screening-rules", { body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: rulesKey }),
  });
}

/** POST /admin/v1/dpia/activities/{id}/screen (DPIA-01) — answer the screening form for a RoPA activity. */
export function useScreenActivity(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { activityId: string; answers: Record<string, unknown> }) => {
      const { data, error } = await client.POST("/admin/v1/dpia/activities/{id}/screen", {
        params: { path: { id: v.activityId } },
        body: { answers: v.answers },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: assessmentsKey }),
  });
}

/** GET /admin/v1/dpia/assessments, newest first — optionally one RoPA activity's own rounds. */
export function useDpiaAssessments(client: ApiClient, filter: { activity_id?: string } = {}) {
  return useInfiniteQuery({
    queryKey: [...assessmentsKey, filter],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/dpia/assessments", {
        params: { query: { activity_id: filter.activity_id, cursor: pageParam, limit: 50 } },
      });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

export function useDpiaAssessment(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...assessmentsKey, id ?? ""],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dpia/assessments/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** GET /admin/v1/dpia/assessments/{id}/description (DPIA-04) — composed live from the assessment's RoPA activity. */
export function useDpiaAssessmentDescription(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...assessmentsKey, id ?? "", "description"],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dpia/assessments/{id}/description", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** GET /admin/v1/dpia/assessments/{id}/diff (DPIA-14) — which screening answers changed since the round this one supersedes. */
export function useDpiaAssessmentDiff(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...assessmentsKey, id ?? "", "diff"],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dpia/assessments/{id}/diff", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** GET /admin/v1/dpia/assessments/{id}/necessity (DPIA-05) — the necessity/proportionality checklist. */
export function useDpiaNecessity(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...assessmentsKey, id ?? "", "necessity"],
    enabled: !!id,
    retry: false,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dpia/assessments/{id}/necessity", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** POST /admin/v1/dpia/assessments/{id}/necessity (DPIA-05) — answer (or re-answer) the checklist. */
export function useAssessNecessity(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { assessmentId: string; answers: Record<string, unknown> }) => {
      const { data, error } = await client.POST("/admin/v1/dpia/assessments/{id}/necessity", {
        params: { path: { id: v.assessmentId } },
        body: { answers: v.answers },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (_data, v) => qc.invalidateQueries({ queryKey: [...assessmentsKey, v.assessmentId, "necessity"] }),
  });
}

/** GET /admin/v1/ropa/activities/{id}/lia (DPIA-16) — every LIA round recorded against this activity. */
export function useLiaForActivity(client: ApiClient, activityId: string | undefined) {
  return useQuery({
    queryKey: ["dpia", "lia", activityId ?? ""],
    enabled: !!activityId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/ropa/activities/{id}/lia", { params: { path: { id: activityId! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** POST /admin/v1/ropa/activities/{id}/lia (DPIA-16) — answer the published LIA test, opening a new round. */
export function useAssessLia(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { activityId: string; answers: Record<string, unknown> }) => {
      const { data, error } = await client.POST("/admin/v1/ropa/activities/{id}/lia", {
        params: { path: { id: v.activityId } },
        body: { answers: v.answers },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (_data, v) => qc.invalidateQueries({ queryKey: ["dpia", "lia", v.activityId] }),
  });
}

/** POST /admin/v1/dpia/lia/{id}/decide (DPIA-16) — the DPO's own approve/reject decision on a round. */
export function useDecideLia(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { activityId: string; liaId: string; ifMatch: string; to: "approved" | "rejected"; reason?: string }) => {
      const { data, error } = await client.POST("/admin/v1/dpia/lia/{id}/decide", {
        params: { path: { id: v.liaId }, header: { "If-Match": v.ifMatch } },
        body: { to: v.to, reason: v.reason },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (_data, v) => qc.invalidateQueries({ queryKey: ["dpia", "lia", v.activityId] }),
  });
}

/** POST /admin/v1/dpia/assessments/{id}/transition (DPIA-10, ST-05#2): submit for review, request more
 *  info, decide (approved/rejected/needs_review), resume or close. */
export function useTransitionDpiaAssessment(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: {
      assessmentId: string;
      ifMatch: string;
      to: "in_review" | "in_progress" | "approved" | "rejected" | "needs_review" | "closed";
      reason?: string;
    }) => {
      const { data, error } = await client.POST("/admin/v1/dpia/assessments/{id}/transition", {
        params: { path: { id: v.assessmentId }, header: { "If-Match": v.ifMatch } },
        body: { to: v.to, reason: v.reason },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (_data, v) => qc.invalidateQueries({ queryKey: [...assessmentsKey, v.assessmentId] }),
  });
}

/** GET /admin/v1/dpia/assessments/{id}/opinions (DPIA-10) — every DPO opinion recorded, oldest first. */
export function useDpiaOpinions(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...assessmentsKey, id ?? "", "opinions"],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dpia/assessments/{id}/opinions", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** POST /admin/v1/dpia/assessments/{id}/opinions (DPIA-10) — the DPO's written opinion + recommendation. */
export function useRecordDpiaOpinion(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { assessmentId: string; opinion: string; recommendation: DpiaOpinionRecommendation }) => {
      const { data, error } = await client.POST("/admin/v1/dpia/assessments/{id}/opinions", {
        params: { path: { id: v.assessmentId } },
        body: { opinion: v.opinion, recommendation: v.recommendation },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (_data, v) => qc.invalidateQueries({ queryKey: [...assessmentsKey, v.assessmentId, "opinions"] }),
  });
}

/** GET /admin/v1/dpia/assessments/{id}/consultations (DPIA-11) — the stakeholder consultation log, oldest first. */
export function useDpiaConsultations(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...assessmentsKey, id ?? "", "consultations"],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dpia/assessments/{id}/consultations", { params: { path: { id: id! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** POST / DELETE on DPIA-11's own consultation log. */
export function useDpiaConsultationMutations(client: ApiClient, assessmentId: string | undefined) {
  const qc = useQueryClient();
  const refresh = () => qc.invalidateQueries({ queryKey: [...assessmentsKey, assessmentId ?? "", "consultations"] });
  return {
    add: useMutation({
      mutationFn: async (input: DpiaConsultationInput) => {
        const { data, error } = await client.POST("/admin/v1/dpia/assessments/{id}/consultations", {
          params: { path: { id: assessmentId! } },
          body: input,
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    remove: useMutation({
      mutationFn: async (consultationId: string) => {
        const { error } = await client.DELETE("/admin/v1/dpia/assessments/{id}/consultations/{consultationId}", {
          params: { path: { id: assessmentId!, consultationId } },
        });
        if (error) throw error;
      },
      onSuccess: refresh,
    }),
  };
}

const riskCatalogKey = ["dpia", "risk-catalog"] as const;

/** GET /admin/v1/dpia/risk-catalog (DPIA-06) — the ready-made starter list for the "add risk" form. */
export function useRiskCatalog(client: ApiClient) {
  return useQuery({
    queryKey: riskCatalogKey,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dpia/risk-catalog", {});
      if (error) throw error;
      return data;
    },
  });
}

/** GET /admin/v1/dpia/assessments/{id}/risks (DPIA-06) — every risk identified against this round. */
export function useAssessmentRisks(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...assessmentsKey, id ?? "", "risks"],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dpia/assessments/{id}/risks", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** POST /admin/v1/dpia/assessments/{id}/risks (DPIA-06) — identify + score a risk against this round. */
export function useIdentifyRisk(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { assessmentId: string; input: DpiaRiskInput }) => {
      const { data, error } = await client.POST("/admin/v1/dpia/assessments/{id}/risks", {
        params: { path: { id: v.assessmentId } },
        body: v.input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (_data, v) => qc.invalidateQueries({ queryKey: [...assessmentsKey, v.assessmentId, "risks"] }),
  });
}

/** PUT /admin/v1/dpia/assessments/{id}/risks/{riskId} (DPIA-06) — edit a risk already linked to this round. */
export function useUpdateRisk(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { assessmentId: string; riskId: string; rowVersion: number; input: DpiaRiskInput }) => {
      const { data, error } = await client.PUT("/admin/v1/dpia/assessments/{id}/risks/{riskId}", {
        params: { path: { id: v.assessmentId, riskId: v.riskId }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: v.input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (_data, v) => qc.invalidateQueries({ queryKey: [...assessmentsKey, v.assessmentId, "risks"] }),
  });
}

/** DELETE /admin/v1/dpia/assessments/{id}/risks/{riskId} (DPIA-06) — unlink a risk from this round. */
export function useRemoveAssessmentRisk(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { assessmentId: string; riskId: string }) => {
      const { error } = await client.DELETE("/admin/v1/dpia/assessments/{id}/risks/{riskId}", {
        params: { path: { id: v.assessmentId, riskId: v.riskId } },
      });
      if (error) throw error;
    },
    onSuccess: (_data, v) => qc.invalidateQueries({ queryKey: [...assessmentsKey, v.assessmentId, "risks"] }),
  });
}

/** POST /admin/v1/dpia/assessments/{id}/risks/{riskId}/submit-to-register (DPIA-08) — confirm a
 *  still-significant risk is on record in the shared risk register (RRA-09) and open its remediation task
 *  if one isn't already open. Returns whether this call actually opened a new task. */
export function useSubmitRiskToRegister(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { assessmentId: string; riskId: string }) => {
      const { data, error } = await client.POST("/admin/v1/dpia/assessments/{id}/risks/{riskId}/submit-to-register", {
        params: { path: { id: v.assessmentId, riskId: v.riskId } },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (_data, v) => qc.invalidateQueries({ queryKey: [...assessmentsKey, v.assessmentId, "risks"] }),
  });
}

/** GET /admin/v1/dpia/assessments/{id}/risks/{riskId}/controls (DPIA-07) — mitigation measures linked to a risk. */
export function useRiskControls(client: ApiClient, assessmentId: string | undefined, riskId: string | undefined) {
  return useQuery({
    queryKey: [...assessmentsKey, assessmentId ?? "", "risks", riskId ?? "", "controls"],
    enabled: !!assessmentId && !!riskId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dpia/assessments/{id}/risks/{riskId}/controls", {
        params: { path: { id: assessmentId!, riskId: riskId! } },
      });
      if (error) throw error;
      return data;
    },
  });
}

/** POST .../risks/{riskId}/controls (DPIA-07) — link a control; an owner + due date opens a tracked task. */
export function useAddRiskControl(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { assessmentId: string; riskId: string; input: DpiaRiskControlInput }) => {
      const { data, error } = await client.POST("/admin/v1/dpia/assessments/{id}/risks/{riskId}/controls", {
        params: { path: { id: v.assessmentId, riskId: v.riskId } },
        body: v.input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (_data, v) => {
      qc.invalidateQueries({ queryKey: [...assessmentsKey, v.assessmentId, "risks", v.riskId, "controls"] });
      qc.invalidateQueries({ queryKey: [...assessmentsKey, v.assessmentId, "risks"] });
    },
  });
}

/** PUT .../risks/{riskId}/controls/{controlId} (DPIA-07) — change a linked control's own status. */
export function useUpdateRiskControlStatus(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { assessmentId: string; riskId: string; controlId: string; rowVersion: number; status: DpiaRiskControlStatus }) => {
      const { data, error } = await client.PUT("/admin/v1/dpia/assessments/{id}/risks/{riskId}/controls/{controlId}", {
        params: { path: { id: v.assessmentId, riskId: v.riskId, controlId: v.controlId }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: { status: v.status },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (_data, v) => {
      qc.invalidateQueries({ queryKey: [...assessmentsKey, v.assessmentId, "risks", v.riskId, "controls"] });
      qc.invalidateQueries({ queryKey: [...assessmentsKey, v.assessmentId, "risks"] });
    },
  });
}

/** DELETE .../risks/{riskId}/controls/{controlId} (DPIA-07) — unlink a control; residual score recomputes. */
export function useRemoveRiskControl(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { assessmentId: string; riskId: string; controlId: string }) => {
      const { error } = await client.DELETE("/admin/v1/dpia/assessments/{id}/risks/{riskId}/controls/{controlId}", {
        params: { path: { id: v.assessmentId, riskId: v.riskId, controlId: v.controlId } },
      });
      if (error) throw error;
    },
    onSuccess: (_data, v) => {
      qc.invalidateQueries({ queryKey: [...assessmentsKey, v.assessmentId, "risks", v.riskId, "controls"] });
      qc.invalidateQueries({ queryKey: [...assessmentsKey, v.assessmentId, "risks"] });
    },
  });
}

/** GET /admin/v1/dpia/templates (DPIA-03) — the template library, optionally filtered by assessment_type. */
export function useDpiaTemplates(client: ApiClient, assessmentType?: string) {
  return useQuery({
    queryKey: [...templatesKey, assessmentType ?? ""],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dpia/templates", {
        params: { query: { assessment_type: assessmentType } },
      });
      if (error) throw error;
      return data;
    },
  });
}

export function useDpiaTemplate(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...templatesKey, "id", id ?? ""],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dpia/templates/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

export function useCreateDpiaTemplate(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      assessment_type: string;
      code: string;
      name: string;
      legal_refs?: string[];
      draft: FormDraft;
    }) => {
      const { data, error } = await client.POST("/admin/v1/dpia/templates", { body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: templatesKey }),
  });
}

export function useCloneDpiaTemplate(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; code: string; name: string }) => {
      const { data, error } = await client.POST("/admin/v1/dpia/templates/{id}/clone", {
        params: { path: { id: v.id } },
        body: { code: v.code, name: v.name },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: templatesKey }),
  });
}

export function usePublishDpiaTemplate(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; ifMatch: string }) => {
      const { data, error } = await client.POST("/admin/v1/dpia/templates/{id}/publish", {
        params: { path: { id: v.id }, header: { "If-Match": v.ifMatch } },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: templatesKey }),
  });
}

/** GET /admin/v1/dpia/registry (DPIA-12) — every RoPA activity's current DPIA round in one report, read
 *  live so its status always matches reality; no pagination. */
export function useDpiaRegistry(
  client: ApiClient,
  filter: { legal_entity_id?: string; org_unit_id?: string; status?: DpiaRegistryEntry["status"] } = {},
) {
  return useQuery({
    queryKey: ["dpia", "registry", filter],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dpia/registry", {
        params: { query: filter },
      });
      if (error) throw error;
      return data;
    },
  });
}

export function useRetireDpiaTemplate(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; ifMatch: string }) => {
      const { data, error } = await client.POST("/admin/v1/dpia/templates/{id}/retire", {
        params: { path: { id: v.id }, header: { "If-Match": v.ifMatch } },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: templatesKey }),
  });
}
