import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type RopaQuestionnaire = components["schemas"]["RopaQuestionnaire"];
export type RopaQuestionnaireInput = components["schemas"]["RopaQuestionnaireInput"];
export type RopaQuestionnaireAnswerInput = components["schemas"]["RopaQuestionnaireAnswerInput"];
export type RopaQuestionnaireApproveInput = components["schemas"]["RopaQuestionnaireApproveInput"];

const questionnairesKey = ["ropa", "questionnaires"] as const;
const ifMatch = (v: number) => ({ "If-Match": `"${v}"` });

/** GET /admin/v1/ropa/questionnaires (ROPA-11) — the data mapping questionnaire's own rounds, optionally
 *  filtered by department or status. */
export function useQuestionnaires(client: ApiClient, filter: { org_unit_id?: string; status?: string } = {}) {
  return useQuery({
    queryKey: [...questionnairesKey, filter],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/ropa/questionnaires", {
        params: { query: { org_unit_id: filter.org_unit_id, status: filter.status as never } },
      });
      if (error) throw error;
      return data.data;
    },
  });
}

/** POST /admin/v1/ropa/questionnaires — send the questionnaire to a department's own respondent. */
export function useSendQuestionnaire(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: RopaQuestionnaireInput) => {
      const { data, error } = await client.POST("/admin/v1/ropa/questionnaires", { body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: questionnairesKey }),
  });
}

/** POST /admin/v1/ropa/questionnaires/{id}/answer — the respondent's own one-shot submission. */
export function useAnswerQuestionnaire(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; input: RopaQuestionnaireAnswerInput }) => {
      const { data, error } = await client.POST("/admin/v1/ropa/questionnaires/{id}/answer", {
        params: { path: { id: v.id } }, body: v.input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: questionnairesKey }),
  });
}

/** POST /admin/v1/ropa/questionnaires/{id}/approve — the acceptance criterion itself: approved answers
 *  enter RoPA automatically as a new processing activity (activity_id on the response). */
export function useApproveQuestionnaire(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { questionnaire: RopaQuestionnaire; input: RopaQuestionnaireApproveInput }) => {
      const { data, error } = await client.POST("/admin/v1/ropa/questionnaires/{id}/approve", {
        params: { path: { id: v.questionnaire.id }, header: ifMatch(v.questionnaire.row_version) }, body: v.input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: questionnairesKey });
      qc.invalidateQueries({ queryKey: ["ropa", "activities"] });
    },
  });
}

/** POST /admin/v1/ropa/questionnaires/{id}/reject — send an answered round back to the respondent. */
export function useRejectQuestionnaire(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (questionnaire: RopaQuestionnaire) => {
      const { data, error } = await client.POST("/admin/v1/ropa/questionnaires/{id}/reject", {
        params: { path: { id: questionnaire.id }, header: ifMatch(questionnaire.row_version) },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: questionnairesKey }),
  });
}
