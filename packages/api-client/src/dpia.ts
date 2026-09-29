import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";
import type { FormDraft } from "./forms";

export type DpiaScreeningRule = components["schemas"]["DpiaScreeningRule"];
export type DpiaAssessment = components["schemas"]["DpiaAssessment"];
export type DpiaTemplate = components["schemas"]["DpiaTemplate"];
export type DpiaActivityDescription = components["schemas"]["DpiaActivityDescription"];
export type DpiaNecessity = components["schemas"]["DpiaNecessity"];
export type DpiaAssessmentDiff = components["schemas"]["DpiaAssessmentDiff"];

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
