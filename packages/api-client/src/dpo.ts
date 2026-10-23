import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type DpoAppointment = components["schemas"]["DpoAppointment"];
export type DpoAppointmentInput = components["schemas"]["DpoAppointmentInput"];
export type DpoType = components["schemas"]["DpoType"];
export type DpoSecurityAssessment = components["schemas"]["DpoSecurityAssessment"];
export type DpoRemediationTask = components["schemas"]["DpoRemediationTask"];
export type DpoDeadlineItem = components["schemas"]["DpoDeadlineItem"];
export type DpoRequirementCheck = components["schemas"]["DpoRequirementCheck"];
export type DpoRequirementCheckInput = components["schemas"]["DpoRequirementCheckInput"];

const ifMatch = (v: number) => ({ "If-Match": `"${v}"` });
const appointmentsKey = ["dpo", "appointments"] as const;
const assessmentsKey = ["dpo", "security-assessments"] as const;
const deadlinesKey = ["dpo", "deadlines"] as const;
const requirementChecksKey = ["dpo", "requirement-checks"] as const;

/** GET /admin/v1/dpo/requirement-checks (DPO-02), newest first, for one legal entity. */
export function useRequirementChecks(client: ApiClient, legalEntityId: string | undefined) {
  return useQuery({
    queryKey: [...requirementChecksKey, legalEntityId ?? ""],
    enabled: !!legalEntityId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dpo/requirement-checks", {
        params: { query: { legal_entity_id: legalEntityId! } },
      });
      if (error) throw error;
      return data.data;
    },
  });
}

/** POST /admin/v1/dpo/requirement-checks — ม.41(2) + ประกาศ สคส. 2566 OR-rule screening for a legal entity. */
export function useAssessRequirementCheck(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: DpoRequirementCheckInput) => {
      const { data, error } = await client.POST("/admin/v1/dpo/requirement-checks", { body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: requirementChecksKey }),
  });
}

/** GET /admin/v1/dpo/deadlines (DPO-05): near-deadline and overdue work from every module that already tracks a legal deadline, on one page. */
export function useDeadlines(client: ApiClient) {
  return useQuery({
    queryKey: deadlinesKey,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dpo/deadlines", {});
      if (error) throw error;
      return data;
    },
  });
}

/** GET /admin/v1/dpo/appointments (DPO-01), newest first. */
export function useAppointments(client: ApiClient, filter: { legal_entity_id?: string } = {}) {
  return useInfiniteQuery({
    queryKey: [...appointmentsKey, filter],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/dpo/appointments", {
        params: { query: { legal_entity_id: filter.legal_entity_id, cursor: pageParam, limit: 50 } },
      });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

export function useAppointment(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...appointmentsKey, id ?? ""],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dpo/appointments/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

export function useAppointmentMutations(client: ApiClient) {
  const qc = useQueryClient();
  return {
    save: useMutation({
      mutationFn: async (v: { appointment?: DpoAppointment; input: DpoAppointmentInput }) => {
        if (!v.appointment) {
          const { data, error } = await client.POST("/admin/v1/dpo/appointments", { body: v.input });
          if (error) throw error;
          return data;
        }
        const { data, error } = await client.PATCH("/admin/v1/dpo/appointments/{id}", {
          params: { path: { id: v.appointment.id }, header: ifMatch(v.appointment.row_version) },
          body: v.input,
        });
        if (error) throw error;
        return data;
      },
      onSuccess: () => qc.invalidateQueries({ queryKey: appointmentsKey }),
    }),
  };
}

/** GET /admin/v1/dpo/security-assessments (DPO-09), newest first. */
export function useAssessments(client: ApiClient, filter: { legal_entity_id?: string } = {}) {
  return useInfiniteQuery({
    queryKey: [...assessmentsKey, filter],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/dpo/security-assessments", {
        params: { query: { legal_entity_id: filter.legal_entity_id, cursor: pageParam, limit: 50 } },
      });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

export function useAssessment(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...assessmentsKey, id ?? ""],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dpo/security-assessments/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** POST /admin/v1/dpo/security-assessments — every yes_no item answered "no" opens a remediation task. */
export function useRecordAssessment(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: { legal_entity_id: string; form_id: string; answers: Record<string, unknown> }) => {
      const { data, error } = await client.POST("/admin/v1/dpo/security-assessments", { body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: assessmentsKey }),
  });
}
