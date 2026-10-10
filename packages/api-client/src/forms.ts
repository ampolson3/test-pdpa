import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type FormTypeInfo = components["schemas"]["FormTypeInfo"];
export type FormSummary = components["schemas"]["FormSummary"];
export type FormDefinition = components["schemas"]["Form"];
export type FormVersion = components["schemas"]["FormVersion"];
export type FormDraft = components["schemas"]["FormDraft"];
export type FormResponse = components["schemas"]["FormResponse"];
export type FormResponseSummary = components["schemas"]["FormResponseSummary"];
export type FormSectionAssignmentItem = components["schemas"]["FormSectionAssignmentItem"];

const ifMatch = (v: number) => ({ "If-Match": `"${v}"` });

/** GET /admin/v1/platform/form-types (PLT-06): the types the caller may read and what they may do. */
export function useFormTypes(client: ApiClient) {
  return useQuery({
    queryKey: ["platform", "form-types"],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/form-types");
      if (error) throw error;
      return data.data;
    },
  });
}

/** GET /admin/v1/platform/forms[?form_type=]. */
export function useForms(client: ApiClient, formType?: string) {
  return useQuery({
    queryKey: ["platform", "forms", "list", formType ?? ""],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/forms", { params: { query: formType ? { form_type: formType } : {} } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** GET /admin/v1/platform/forms/{id} — with every version, newest first. */
export function useForm(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: ["platform", "forms", id],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/forms/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** Create, save the draft (or start a new one) and publish. */
export function useFormMutations(client: ApiClient) {
  const qc = useQueryClient();
  const refresh = () => void qc.invalidateQueries({ queryKey: ["platform", "forms"] });
  return {
    create: useMutation({
      mutationFn: async (v: { code: string; name: string; form_type: string; draft: FormDraft }) => {
        const { data, error } = await client.POST("/admin/v1/platform/forms", { body: v });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    /** Saves the draft version; with no draft (latest published) it starts a new one. */
    saveDraft: useMutation({
      mutationFn: async (v: { formId: string; draft: FormDraft; draftRowVersion?: number }) => {
        if (v.draftRowVersion === undefined) {
          const { data, error } = await client.POST("/admin/v1/platform/forms/{id}/versions", { params: { path: { id: v.formId } }, body: v.draft });
          if (error) throw error;
          return data;
        }
        const { data, error } = await client.PUT("/admin/v1/platform/forms/{id}/draft", {
          params: { path: { id: v.formId }, header: ifMatch(v.draftRowVersion) },
          body: v.draft,
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    publish: useMutation({
      mutationFn: async (v: { formId: string; draftRowVersion: number }) => {
        const { data, error } = await client.POST("/admin/v1/platform/forms/{id}/publish", { params: { path: { id: v.formId }, header: ifMatch(v.draftRowVersion) } });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
  };
}

/** GET /admin/v1/platform/forms/{id}/responses. */
export function useFormResponses(client: ApiClient, formId: string | undefined) {
  return useQuery({
    queryKey: ["platform", "form-responses", "list", formId],
    enabled: !!formId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/forms/{id}/responses", { params: { path: { id: formId! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** GET /admin/v1/platform/form-responses/{id}. */
export function useFormResponse(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: ["platform", "form-responses", id],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/form-responses/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** GET /admin/v1/platform/my-form-sections — sections waiting for the caller. */
export function useMyFormSections(client: ApiClient) {
  return useQuery({
    queryKey: ["platform", "my-form-sections"],
    refetchInterval: 30_000,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/my-form-sections");
      if (error) throw error;
      return data.data;
    },
  });
}

/** Start, answer, assign, complete and submit responses. Each result updates the response's cache entry. */
export function useFormResponseMutations(client: ApiClient) {
  const qc = useQueryClient();
  const store = (r: FormResponse) => {
    qc.setQueryData(["platform", "form-responses", r.id], r);
    void qc.invalidateQueries({ queryKey: ["platform", "form-responses", "list"] });
    void qc.invalidateQueries({ queryKey: ["platform", "my-form-sections"] });
  };
  return {
    start: useMutation({
      mutationFn: async (v: { formId: string; entity_type?: string; entity_id?: string }) => {
        const { formId, ...body } = v;
        const { data, error } = await client.POST("/admin/v1/platform/forms/{id}/responses", { params: { path: { id: formId } }, body });
        if (error) throw error;
        return data;
      },
      onSuccess: store,
    }),
    saveAnswers: useMutation({
      mutationFn: async (v: { response: FormResponse; answers: Record<string, unknown> }) => {
        const { data, error } = await client.PATCH("/admin/v1/platform/form-responses/{id}/answers", {
          params: { path: { id: v.response.id }, header: ifMatch(v.response.row_version) },
          body: { answers: v.answers },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: store,
    }),
    assign: useMutation({
      mutationFn: async (v: { response: FormResponse; section: string; userId: string | null }) => {
        const { data, error } = await client.PUT("/admin/v1/platform/form-responses/{id}/assignments/{section}", {
          params: { path: { id: v.response.id, section: v.section }, header: ifMatch(v.response.row_version) },
          body: { assignee_user_id: v.userId },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: store,
    }),
    completeSection: useMutation({
      mutationFn: async (v: { response: FormResponse; section: string }) => {
        const { data, error } = await client.POST("/admin/v1/platform/form-responses/{id}/sections/{section}/complete", {
          params: { path: { id: v.response.id, section: v.section } },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: store,
    }),
    submit: useMutation({
      mutationFn: async (v: { response: FormResponse }) => {
        const { data, error } = await client.POST("/admin/v1/platform/form-responses/{id}/submit", {
          params: { path: { id: v.response.id }, header: ifMatch(v.response.row_version) },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: store,
    }),
  };
}
