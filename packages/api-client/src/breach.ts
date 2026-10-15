import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type BreachIncident = components["schemas"]["BreachIncident"];
export type BreachIncidentInput = components["schemas"]["BreachIncidentInput"];
export type BreachIncidentUpdate = components["schemas"]["BreachIncidentUpdate"];
export type BreachTimelineItem = components["schemas"]["BreachTimelineItem"];
export type BreachAssessment = components["schemas"]["BreachAssessment"];
export type BreachEvidence = components["schemas"]["BreachEvidence"];
export type BreachNotice = components["schemas"]["BreachNotice"];
export type BreachNoticeVars = components["schemas"]["BreachNoticeVars"];
export type BreachRecipient = components["schemas"]["BreachRecipient"];
export type BreachPDPCNotification = components["schemas"]["BreachPDPCNotification"];
export type BreachFilter = { status?: BreachIncident["status"]; risk?: "none" | "low" | "high"; q?: string; open_only?: boolean };

const ifMatch = (v: number) => ({ "If-Match": `"${v}"` });
const incidentKey = (id: string) => ["breach", "incident", id];

/** The breach register (BRE-02 / 13), newest awareness first. */
export function useIncidents(client: ApiClient, filter: BreachFilter) {
  return useQuery({
    queryKey: ["breach", "incidents", filter],
    refetchInterval: 60_000, // the 72-hour clocks move
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/breach/incidents", { params: { query: { ...filter, q: filter.q || undefined, limit: 100 } } });
      if (error) throw error;
      return data.data;
    },
  });
}

export function useIncident(client: ApiClient, id: string) {
  return useQuery({
    queryKey: incidentKey(id),
    refetchInterval: 60_000,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/breach/incidents/{id}", { params: { path: { id } } });
      if (error) throw error;
      return data;
    },
  });
}

export function useIncidentTimeline(client: ApiClient, id: string) {
  return useQuery({
    queryKey: [...incidentKey(id), "timeline"],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/breach/incidents/{id}/timeline", { params: { path: { id } } });
      if (error) throw error;
      return data.data;
    },
  });
}

export function useIncidentAssessments(client: ApiClient, id: string) {
  return useQuery({
    queryKey: [...incidentKey(id), "assessments"],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/breach/incidents/{id}/assessments", { params: { path: { id } } });
      if (error) throw error;
      return data.data;
    },
  });
}

export function useIncidentEvidence(client: ApiClient, id: string) {
  return useQuery({
    queryKey: [...incidentKey(id), "evidence"],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/breach/incidents/{id}/evidence", { params: { path: { id } } });
      if (error) throw error;
      return data.data;
    },
  });
}

export function useIncidentNotices(client: ApiClient, id: string, enabled = true) {
  return useQuery({
    queryKey: [...incidentKey(id), "notices"],
    enabled,
    refetchInterval: (q) => (q.state.data?.some((n) => n.status === "sending") ? 3000 : false),
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/breach/incidents/{id}/notices", { params: { path: { id } } });
      if (error) throw error;
      return data.data;
    },
  });
}

export function useNoticeRecipients(client: ApiClient, noticeId: string | undefined, status?: BreachRecipient["status"]) {
  return useQuery({
    queryKey: ["breach", "notice", noticeId, "recipients", status ?? ""],
    enabled: !!noticeId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/breach/notices/{id}/recipients", {
        params: { path: { id: noticeId! }, query: { status, limit: 200 } },
      });
      if (error) throw error;
      return data.data;
    },
  });
}

/** Every change to an incident refreshes it, its timeline and the register. */
export function useIncidentMutations(client: ApiClient, id?: string) {
  const qc = useQueryClient();
  const refresh = () => {
    qc.invalidateQueries({ queryKey: ["breach", "incidents"] });
    if (id) qc.invalidateQueries({ queryKey: incidentKey(id) });
  };
  return {
    create: useMutation({
      mutationFn: async (body: BreachIncidentInput) => {
        const { data, error } = await client.POST("/admin/v1/breach/incidents", { body });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    update: useMutation({
      mutationFn: async (v: { incident: BreachIncident; body: BreachIncidentUpdate }) => {
        const { data, error } = await client.PATCH("/admin/v1/breach/incidents/{id}", {
          params: { path: { id: v.incident.id }, header: ifMatch(v.incident.row_version) },
          body: v.body,
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    transition: useMutation({
      mutationFn: async (v: { incident: BreachIncident; to: "triage" | "assessing" | "notifying" | "remediating" | "closed"; reason?: string; owner_user_id?: string }) => {
        const { data, error } = await client.POST("/admin/v1/breach/incidents/{id}/transitions", {
          params: { path: { id: v.incident.id }, header: ifMatch(v.incident.row_version) },
          body: { to: v.to, reason: v.reason || undefined, owner_user_id: v.owner_user_id },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    assess: useMutation({
      mutationFn: async (v: { form_id: string; answers: Record<string, unknown> }) => {
        const { data, error } = await client.POST("/admin/v1/breach/incidents/{id}/assessments", { params: { path: { id: id! } }, body: v });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    decide: useMutation({
      mutationFn: async (v: { incident: BreachIncident; decision: "no_notification" | "notify_pdpc" | "notify_pdpc_and_subjects"; reason: string }) => {
        const { data, error } = await client.POST("/admin/v1/breach/incidents/{id}/decision", {
          params: { path: { id: v.incident.id }, header: ifMatch(v.incident.row_version) },
          body: { decision: v.decision, reason: v.reason },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    note: useMutation({
      mutationFn: async (v: { text: string; occurred_at?: string }) => {
        const { error } = await client.POST("/admin/v1/breach/incidents/{id}/timeline", { params: { path: { id: id! } }, body: v });
        if (error) throw error;
      },
      onSuccess: refresh,
    }),
    evidence: useMutation({
      mutationFn: async (v: { file_id: string; description?: string }) => {
        const { data, error } = await client.POST("/admin/v1/breach/incidents/{id}/evidence", { params: { path: { id: id! } }, body: v });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
  };
}

/** Notices to data subjects (BRE-10): draft → recipients → approve & send (another person). */
export function useNoticeMutations(client: ApiClient, incidentId: string) {
  const qc = useQueryClient();
  const refresh = () => qc.invalidateQueries({ queryKey: incidentKey(incidentId) });
  return {
    create: useMutation({
      mutationFn: async (v: { channel: "email" | "sms"; variables: BreachNoticeVars }) => {
        const { data, error } = await client.POST("/admin/v1/breach/incidents/{id}/notices", { params: { path: { id: incidentId } }, body: v });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    update: useMutation({
      mutationFn: async (v: { notice: BreachNotice; variables: BreachNoticeVars }) => {
        const { data, error } = await client.PUT("/admin/v1/breach/notices/{id}", {
          params: { path: { id: v.notice.id }, header: ifMatch(v.notice.row_version) },
          body: { variables: v.variables },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    recipients: useMutation({
      mutationFn: async (v: { notice: BreachNotice; file_id: string }) => {
        const { data, error } = await client.POST("/admin/v1/breach/notices/{id}/recipients", {
          params: { path: { id: v.notice.id }, header: ifMatch(v.notice.row_version) },
          body: { file_id: v.file_id },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    send: useMutation({
      mutationFn: async (notice: BreachNotice) => {
        const { data, error } = await client.POST("/admin/v1/breach/notices/{id}/send", {
          params: { path: { id: notice.id }, header: ifMatch(notice.row_version) },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
  };
}

/** PDPC filing rounds of an incident (BRE-08/09), oldest first. */
export function useIncidentPDPCNotifications(client: ApiClient, id: string, enabled = true) {
  return useQuery({
    queryKey: [...incidentKey(id), "pdpc"],
    enabled,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/breach/incidents/{id}/pdpc-notifications", { params: { path: { id } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** Record a filing round (maker) and confirm one (a second person, maker-checker). */
export function usePDPCNotificationMutations(client: ApiClient, incidentId: string) {
  const qc = useQueryClient();
  const refresh = () => qc.invalidateQueries({ queryKey: incidentKey(incidentId) });
  return {
    record: useMutation({
      mutationFn: async (v: {
        notification_type: "initial" | "supplementary" | "final";
        document_version_id: string;
        submitted_at: string;
        submission_ref?: string;
        evidence_file_id?: string;
        late_reason?: string;
      }) => {
        const { data, error } = await client.POST("/admin/v1/breach/incidents/{id}/pdpc-notifications", { params: { path: { id: incidentId } }, body: v });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    confirm: useMutation({
      mutationFn: async (n: BreachPDPCNotification) => {
        const { data, error } = await client.POST("/admin/v1/breach/pdpc-notifications/{id}/confirm", { params: { path: { id: n.id }, header: ifMatch(n.row_version) } });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
  };
}

// BRE-01: public incident-report forms (loginless, link / QR).
export type BreachReportForm = components["schemas"]["BreachReportForm"];
export type BreachReportFormInput = components["schemas"]["BreachReportFormInput"];
export type BreachReportFormStatus = components["schemas"]["BreachReportFormStatus"];

const reportFormsKey = ["breach", "report-forms"] as const;

/** GET /admin/v1/breach/report-forms (BRE-01). */
export function useBreachReportForms(client: ApiClient, status?: BreachReportFormStatus) {
  return useInfiniteQuery({
    queryKey: [...reportFormsKey, status ?? null],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/breach/report-forms", {
        params: { query: { status, cursor: pageParam, limit: 50 } },
      });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

/** Create / publish / retire a public incident-report form (BRE-01). */
export function useBreachReportFormMutations(client: ApiClient) {
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: reportFormsKey });
  const create = useMutation({
    mutationFn: async (input: BreachReportFormInput) => {
      const { data, error } = await client.POST("/admin/v1/breach/report-forms", { body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: invalidate,
  });
  const publish = useMutation({
    mutationFn: async (form: BreachReportForm) => {
      const { data, error } = await client.POST("/admin/v1/breach/report-forms/{id}/publish", {
        params: { path: { id: form.id }, header: ifMatch(form.row_version) },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: invalidate,
  });
  const retire = useMutation({
    mutationFn: async (form: BreachReportForm) => {
      const { data, error } = await client.POST("/admin/v1/breach/report-forms/{id}/retire", {
        params: { path: { id: form.id }, header: ifMatch(form.row_version) },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: invalidate,
  });
  return { create, publish, retire };
}

// BRE-15: root cause & lessons learned.
export type BreachRootCause = components["schemas"]["BreachRootCause"];
export type BreachRootCauseInput = components["schemas"]["BreachRootCauseInput"];
export type BreachCorrectiveAction = components["schemas"]["BreachCorrectiveAction"];
export type BreachCorrectiveActionInput = components["schemas"]["BreachCorrectiveActionInput"];

const rootCausesKey = (incidentId: string) => [...incidentKey(incidentId), "root-causes"];

/** GET /admin/v1/breach/incidents/{id}/root-causes (BRE-15). */
export function useRootCauses(client: ApiClient, incidentId: string) {
  return useQuery({
    queryKey: rootCausesKey(incidentId),
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/breach/incidents/{id}/root-causes", { params: { path: { id: incidentId } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** Add / edit / delete a root cause, and mark one corrective action done (BRE-15's own acceptance criterion). */
export function useRootCauseMutations(client: ApiClient, incidentId: string) {
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: rootCausesKey(incidentId) });
  const add = useMutation({
    mutationFn: async (input: BreachRootCauseInput) => {
      const { data, error } = await client.POST("/admin/v1/breach/incidents/{id}/root-causes", { params: { path: { id: incidentId } }, body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: invalidate,
  });
  const update = useMutation({
    mutationFn: async (v: { rootCause: BreachRootCause; input: BreachRootCauseInput }) => {
      const { data, error } = await client.PUT("/admin/v1/breach/root-causes/{id}", {
        params: { path: { id: v.rootCause.id }, header: ifMatch(v.rootCause.row_version) }, body: v.input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: invalidate,
  });
  const remove = useMutation({
    mutationFn: async (rootCause: BreachRootCause) => {
      const { error } = await client.DELETE("/admin/v1/breach/root-causes/{id}", {
        params: { path: { id: rootCause.id }, query: { incident_id: incidentId } },
      });
      if (error) throw error;
    },
    onSuccess: invalidate,
  });
  const completeAction = useMutation({
    mutationFn: async (v: { rootCause: BreachRootCause; actionId: string }) => {
      const { data, error } = await client.POST("/admin/v1/breach/root-causes/{id}/corrective-actions/{actionId}/complete", {
        params: { path: { id: v.rootCause.id, actionId: v.actionId }, header: ifMatch(v.rootCause.row_version) },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: invalidate,
  });
  return { add, update, remove, completeAction };
}
