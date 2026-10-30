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
export type DpoIndependenceDeclaration = components["schemas"]["DpoIndependenceDeclaration"];
export type DpoIndependenceDeclarationInput = components["schemas"]["DpoIndependenceDeclarationInput"];
export type DpoAdvisory = components["schemas"]["DpoAdvisory"];
export type DpoAdvisoryInput = components["schemas"]["DpoAdvisoryInput"];
export type DpoKbArticle = components["schemas"]["DpoKbArticle"];
export type DpoCalendarItem = components["schemas"]["DpoCalendarItem"];
export type DpoScorecard = components["schemas"]["DpoScorecard"];
export type DpoScorecardComponent = components["schemas"]["DpoScorecardComponent"];

const ifMatch = (v: number) => ({ "If-Match": `"${v}"` });
const appointmentsKey = ["dpo", "appointments"] as const;
const assessmentsKey = ["dpo", "security-assessments"] as const;
const deadlinesKey = ["dpo", "deadlines"] as const;
const calendarKey = ["dpo", "calendar"] as const;
const requirementChecksKey = ["dpo", "requirement-checks"] as const;
const independenceDeclarationsKey = ["dpo", "independence-declarations"] as const;
const advisoriesKey = ["dpo", "advisories"] as const;
const kbArticlesKey = ["dpo", "kb-articles"] as const;

/** GET /admin/v1/dpo/advisories (DPO-07) — advisory questions to the DPO, optionally filtered. */
export function useAdvisories(client: ApiClient, filter: { org_unit_id?: string; status?: string } = {}) {
  return useQuery({
    queryKey: [...advisoriesKey, filter],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dpo/advisories", {
        params: { query: { org_unit_id: filter.org_unit_id, status: filter.status as never } },
      });
      if (error) throw error;
      return data.data;
    },
  });
}

/** POST /admin/v1/dpo/advisories — a department sends a question to the DPO. */
export function useSubmitAdvisory(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: DpoAdvisoryInput) => {
      const { data, error } = await client.POST("/admin/v1/dpo/advisories", { body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: advisoriesKey }),
  });
}

/** POST /admin/v1/dpo/advisories/{id}/answer (DPO-07's own acceptance criterion) — answers the question
 *  and promotes it into the searchable knowledge base in the same step. */
export function useAnswerAdvisory(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { advisory: DpoAdvisory; answer: string }) => {
      const { data, error } = await client.POST("/admin/v1/dpo/advisories/{id}/answer", {
        params: { path: { id: v.advisory.id }, header: ifMatch(v.advisory.row_version) },
        body: { answer: v.answer },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: advisoriesKey });
      qc.invalidateQueries({ queryKey: kbArticlesKey });
    },
  });
}

/** GET /admin/v1/dpo/kb-articles (DPO-07) — the searchable knowledge base. */
export function useKbArticles(client: ApiClient, filter: { category?: string; q?: string } = {}) {
  return useQuery({
    queryKey: [...kbArticlesKey, filter],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dpo/kb-articles", { params: { query: filter } });
      if (error) throw error;
      return data.data;
    },
  });
}

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

/** GET /admin/v1/dpo/calendar (DPO-08, ม.42(2)): every module's own scheduled review/due-date cycle,
 *  read live within [from, to] (default: today through 90 days out). */
export function useComplianceCalendar(client: ApiClient, range: { from?: string; to?: string } = {}) {
  return useQuery({
    queryKey: [...calendarKey, range],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dpo/calendar", { params: { query: range } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** GET /admin/v1/dpo/calendar.ics (DPO-08's own "export iCal") — a plain href, not a query/mutation: the
 *  browser downloads the feed through this link directly, the same pattern riskReportHref uses. */
export function complianceCalendarIcsHref(bffBaseUrl: string, range: { from?: string; to?: string } = {}): string {
  const q = new URLSearchParams(range as Record<string, string>);
  const qs = q.toString();
  return `${bffBaseUrl}/admin/v1/dpo/calendar.ics${qs ? `?${qs}` : ""}`;
}

/** GET /admin/v1/dpo/scorecard/{orgUnitId} (DPO-10): one org unit's own PDPA-readiness scorecard —
 *  RoPA completeness, legal-gap readiness, DPIA status and training completion, each named, so the
 *  score is both reproducible and explainable. */
export function useOrgUnitScorecard(client: ApiClient, orgUnitId: string | undefined) {
  return useQuery({
    queryKey: ["dpo", "scorecard", orgUnitId ?? ""],
    enabled: !!orgUnitId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dpo/scorecard/{orgUnitId}", { params: { path: { orgUnitId: orgUnitId! } } });
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

/** GET /admin/v1/dpo/appointments/{id}/independence-declarations (DPO-03), newest year first. */
export function useIndependenceDeclarations(client: ApiClient, appointmentId: string | undefined) {
  return useQuery({
    queryKey: [...independenceDeclarationsKey, appointmentId ?? ""],
    enabled: !!appointmentId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dpo/appointments/{id}/independence-declarations", {
        params: { path: { id: appointmentId! } },
      });
      if (error) throw error;
      return data.data;
    },
  });
}

export function useIndependenceDeclarationMutations(client: ApiClient, appointmentId: string) {
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: [...independenceDeclarationsKey, appointmentId] });
  return {
    record: useMutation({
      mutationFn: async (input: DpoIndependenceDeclarationInput) => {
        const { data, error } = await client.POST("/admin/v1/dpo/appointments/{id}/independence-declarations", {
          params: { path: { id: appointmentId } },
          body: input,
        });
        if (error) throw error;
        return data;
      },
      onSuccess: invalidate,
    }),
    sign: useMutation({
      mutationFn: async (v: { declaration: DpoIndependenceDeclaration; file_id?: string }) => {
        const { data, error } = await client.POST("/admin/v1/dpo/independence-declarations/{id}/sign", {
          params: { path: { id: v.declaration.id }, header: ifMatch(v.declaration.row_version) },
          body: { file_id: v.file_id },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: invalidate,
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
