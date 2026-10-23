import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type DsarRequestType = components["schemas"]["DsarRequestType"];
export type DsarRequest = components["schemas"]["DsarRequest"];
export type DsarRequestStatus = components["schemas"]["DsarRequestStatus"];
export type DsarRequestChannel = components["schemas"]["DsarRequestChannel"];
export type DsarOutcome = components["schemas"]["DsarOutcome"];
export type DsarContactKind = components["schemas"]["DsarContactKind"];
export type DsarRequestInput = components["schemas"]["DsarRequestInput"];
export type DsarTransitionInput = components["schemas"]["DsarTransitionInput"];
export type DsarTransitionResult = components["schemas"]["DsarTransitionResult"];
export type DsarSlaStatus = components["schemas"]["DsarSlaStatus"];
export type DsarVerification = components["schemas"]["DsarVerification"];
export type DsarVerificationMethod = components["schemas"]["DsarVerificationMethod"];
export type DsarIdentityRedaction = components["schemas"]["DsarIdentityRedaction"];
export type DsarSubtask = components["schemas"]["DsarSubtask"];
export type DsarSubtaskAction = components["schemas"]["DsarSubtaskAction"];
export type DsarSubtaskStatus = components["schemas"]["DsarSubtaskStatus"];

const requestsKey = ["dsar", "requests"] as const;
const requestKey = (id: string) => [...requestsKey, id] as const;
const ifMatch = (v: number) => ({ "If-Match": `"${v}"` });

/** GET /admin/v1/dsar/request-types (DSAR-13) — the 9 fixed right types, for the intake form's picker. */
export function useDsarRequestTypes(client: ApiClient) {
  return useQuery({
    queryKey: ["dsar", "request-types"] as const,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dsar/request-types", {});
      if (error) throw error;
      return data.data;
    },
  });
}

/** GET /admin/v1/dsar/requests (ST-02), newest first. `search` is DSAR-17's history search: a request number
 *  (substring) or an e-mail (exact blind-index match). `request_type_id` is DSAR-05's own filter — narrows
 *  to one right type (e.g. complaint/inquiry) in the same queue every other right type already shares. */
export function useDsarRequests(
  client: ApiClient,
  filter: { status?: DsarRequestStatus; request_type_id?: string; search?: string } = {},
) {
  return useInfiniteQuery({
    queryKey: [...requestsKey, filter],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/dsar/requests", {
        params: {
          query: {
            status: filter.status,
            request_type_id: filter.request_type_id || undefined,
            search: filter.search || undefined,
            cursor: pageParam,
            limit: 50,
          },
        },
      });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

/** GET /admin/v1/dsar/requests/{id}. */
export function useDsarRequest(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: requestKey(id ?? ""),
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dsar/requests/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** POST /admin/v1/dsar/requests (create) and POST .../{id}/transition (ST-02; DSAR-13 auto-generates the
 *  matching response letter on entering awaiting_info/completed/rejected). */
export function useDsarRequestMutations(client: ApiClient) {
  const qc = useQueryClient();
  const create = useMutation({
    mutationFn: async (input: DsarRequestInput) => {
      const { data, error } = await client.POST("/admin/v1/dsar/requests", { body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: requestsKey }),
  });
  const transition = useMutation({
    mutationFn: async ({ request, input }: { request: DsarRequest; input: DsarTransitionInput }) => {
      const { data, error } = await client.POST("/admin/v1/dsar/requests/{id}/transition", {
        params: { path: { id: request.id }, header: ifMatch(request.row_version) },
        body: input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (_data, { request }) => {
      qc.invalidateQueries({ queryKey: requestsKey });
      qc.invalidateQueries({ queryKey: requestKey(request.id) });
    },
  });
  const assign = useMutation({
    mutationFn: async ({ request, userId }: { request: DsarRequest; userId: string | null }) => {
      const { data, error } = await client.POST("/admin/v1/dsar/requests/{id}/assign", {
        params: { path: { id: request.id }, header: ifMatch(request.row_version) },
        body: { assignee_user_id: userId },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (_data, { request }) => {
      qc.invalidateQueries({ queryKey: requestsKey });
      qc.invalidateQueries({ queryKey: requestKey(request.id) });
    },
  });
  return { create, transition, assign };
}

const verificationsKey = (requestId: string) => [...requestKey(requestId), "verifications"] as const;

/** GET /admin/v1/dsar/requests/{id}/verifications (DSAR-06) — every identity-check attempt, oldest first. */
export function useDsarVerifications(client: ApiClient, requestId: string | undefined) {
  return useQuery({
    queryKey: verificationsKey(requestId ?? ""),
    enabled: !!requestId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dsar/requests/{id}/verifications", { params: { path: { id: requestId! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** DSAR-06's identity-verification flow: start/confirm an OTP, submit a redacted ID-card copy, and staff's
 * own pass/fail decision on one. Each invalidates the request itself too (verified_at / status can change). */
export function useDsarVerificationMutations(client: ApiClient) {
  const qc = useQueryClient();
  const refresh = (requestId: string) => {
    qc.invalidateQueries({ queryKey: verificationsKey(requestId) });
    qc.invalidateQueries({ queryKey: requestKey(requestId) });
    qc.invalidateQueries({ queryKey: requestsKey });
  };
  const startOtp = useMutation({
    mutationFn: async ({ requestId, method }: { requestId: string; method: "otp_sms" | "otp_email" }) => {
      const { data, error } = await client.POST("/admin/v1/dsar/requests/{id}/verifications/otp", {
        params: { path: { id: requestId } },
        body: { method },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (_d, { requestId }) => refresh(requestId),
  });
  const confirmOtp = useMutation({
    mutationFn: async ({ requestId, verificationId, code }: { requestId: string; verificationId: string; code: string }) => {
      const { data, error } = await client.POST("/admin/v1/dsar/requests/{id}/verifications/{verificationId}/confirm-otp", {
        params: { path: { id: requestId, verificationId } },
        body: { code },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (_d, { requestId }) => refresh(requestId),
  });
  const submitIdDocument = useMutation({
    mutationFn: async ({ requestId, rawFileId, redactions }: { requestId: string; rawFileId: string; redactions: DsarIdentityRedaction[] }) => {
      const { data, error } = await client.POST("/admin/v1/dsar/requests/{id}/verifications/id-document", {
        params: { path: { id: requestId } },
        body: { raw_file_id: rawFileId, redactions },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (_d, { requestId }) => refresh(requestId),
  });
  const decide = useMutation({
    mutationFn: async ({ requestId, verificationId, pass }: { requestId: string; verificationId: string; pass: boolean }) => {
      const { data, error } = await client.POST("/admin/v1/dsar/requests/{id}/verifications/{verificationId}/decide", {
        params: { path: { id: requestId, verificationId } },
        body: { pass },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (_d, { requestId }) => refresh(requestId),
  });
  return { startOtp, confirmOtp, submitIdDocument, decide };
}

const subtasksKey = (requestId: string) => [...requestKey(requestId), "subtasks"] as const;

/** GET /admin/v1/dsar/requests/{id}/subtasks (DSAR-08) — every subtask of a request, oldest first. */
export function useDsarSubtasks(client: ApiClient, requestId: string | undefined) {
  return useQuery({
    queryKey: subtasksKey(requestId ?? ""),
    enabled: !!requestId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dsar/requests/{id}/subtasks", { params: { path: { id: requestId! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** DSAR-08's workflow & subtasks: assign one, move it along open → in_progress → done|not_applicable, or
 *  remove it. A request can only complete once every subtask is done (the acceptance criterion), so each
 *  mutation also invalidates the request itself. */
export function useDsarSubtaskMutations(client: ApiClient) {
  const qc = useQueryClient();
  const refresh = (requestId: string) => {
    qc.invalidateQueries({ queryKey: subtasksKey(requestId) });
    qc.invalidateQueries({ queryKey: requestKey(requestId) });
    qc.invalidateQueries({ queryKey: requestsKey });
  };
  const create = useMutation({
    mutationFn: async ({ requestId, action, assigneeUserId, assigneeGroupId, dueAt }: {
      requestId: string; action: DsarSubtaskAction; assigneeUserId?: string; assigneeGroupId?: string; dueAt?: string;
    }) => {
      const { data, error } = await client.POST("/admin/v1/dsar/requests/{id}/subtasks", {
        params: { path: { id: requestId } },
        body: { action, assignee_user_id: assigneeUserId, assignee_group_id: assigneeGroupId, due_at: dueAt },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (_d, { requestId }) => refresh(requestId),
  });
  const updateStatus = useMutation({
    mutationFn: async ({ requestId, subtask, status, evidenceFileId }: {
      requestId: string; subtask: DsarSubtask; status: DsarSubtaskStatus; evidenceFileId?: string;
    }) => {
      const { data, error } = await client.POST("/admin/v1/dsar/requests/{id}/subtasks/{subtaskId}/status", {
        params: { path: { id: requestId, subtaskId: subtask.id }, header: ifMatch(subtask.row_version) },
        body: { status, evidence_file_id: evidenceFileId },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (_d, { requestId }) => refresh(requestId),
  });
  const remove = useMutation({
    mutationFn: async ({ requestId, subtaskId }: { requestId: string; subtaskId: string }) => {
      const { error } = await client.DELETE("/admin/v1/dsar/requests/{id}/subtasks/{subtaskId}", {
        params: { path: { id: requestId, subtaskId } },
      });
      if (error) throw error;
    },
    onSuccess: (_d, { requestId }) => refresh(requestId),
  });
  return { create, updateStatus, remove };
}

// DSAR-01: intake forms (public web submission).
export type DsarIntakeForm = components["schemas"]["DsarIntakeForm"];
export type DsarIntakeFormInput = components["schemas"]["DsarIntakeFormInput"];
export type DsarIntakeFormStatus = components["schemas"]["DsarIntakeFormStatus"];

const intakeFormsKey = ["dsar", "intake-forms"] as const;

/** GET /admin/v1/dsar/forms (DSAR-01). */
export function useDsarIntakeForms(client: ApiClient, status?: DsarIntakeFormStatus) {
  return useInfiniteQuery({
    queryKey: [...intakeFormsKey, status ?? null],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/dsar/forms", {
        params: { query: { status, cursor: pageParam, limit: 50 } },
      });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

/** Create / publish / retire an intake form (DSAR-01). */
export function useDsarIntakeFormMutations(client: ApiClient) {
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: intakeFormsKey });
  const create = useMutation({
    mutationFn: async (input: DsarIntakeFormInput) => {
      const { data, error } = await client.POST("/admin/v1/dsar/forms", { body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: invalidate,
  });
  const publish = useMutation({
    mutationFn: async (form: DsarIntakeForm) => {
      const { data, error } = await client.POST("/admin/v1/dsar/forms/{id}/publish", {
        params: { path: { id: form.id }, header: ifMatch(form.row_version) },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: invalidate,
  });
  const retire = useMutation({
    mutationFn: async (form: DsarIntakeForm) => {
      const { data, error } = await client.POST("/admin/v1/dsar/forms/{id}/retire", {
        params: { path: { id: form.id }, header: ifMatch(form.row_version) },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: invalidate,
  });
  return { create, publish, retire };
}

// DSAR-04: the agent who submitted a request on the data subject's own behalf (ม.20), with their
// authority-proof document.
export type DsarAgent = components["schemas"]["DsarAgent"];
export type DsarAuthorityType = components["schemas"]["DsarAuthorityType"];
export type DsarRecordAgentInput = components["schemas"]["DsarRecordAgentInput"];

/** GET /admin/v1/dsar/requests/{id}/agents. */
export function useDsarAgents(client: ApiClient, requestId: string | undefined) {
  return useQuery({
    queryKey: ["dsar", "agents", requestId ?? ""] as const,
    enabled: !!requestId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dsar/requests/{id}/agents", { params: { path: { id: requestId! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** Record an agent (POST .../agents) and confirm their authority document (POST .../agents/{agentId}/verify). */
export function useDsarAgentMutations(client: ApiClient) {
  const qc = useQueryClient();
  const invalidate = (requestId: string) => qc.invalidateQueries({ queryKey: ["dsar", "agents", requestId] });
  const record = useMutation({
    mutationFn: async ({ requestId, input }: { requestId: string; input: DsarRecordAgentInput }) => {
      const { data, error } = await client.POST("/admin/v1/dsar/requests/{id}/agents", { params: { path: { id: requestId } }, body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: (_d, { requestId }) => invalidate(requestId),
  });
  const verify = useMutation({
    mutationFn: async ({ requestId, agentId }: { requestId: string; agentId: string }) => {
      const { data, error } = await client.POST("/admin/v1/dsar/requests/{id}/agents/{agentId}/verify", { params: { path: { id: requestId, agentId } } });
      if (error) throw error;
      return data;
    },
    onSuccess: (_d, { requestId }) => invalidate(requestId),
  });
  return { record, verify };
}

// DSAR-14: secure delivery packages — a loginless link wrapping an already-attached file, expiring per a
// configured setting, optionally gated by a one-time access code.
export type DsarPackage = components["schemas"]["DsarPackage"];
export type DsarPackageFormat = components["schemas"]["DsarPackageFormat"];
export type DsarCreatePackageInput = components["schemas"]["DsarCreatePackageInput"];
export type DsarCreatePackageResult = components["schemas"]["DsarCreatePackageResult"];

/** GET /admin/v1/dsar/requests/{id}/packages. */
export function useDsarPackages(client: ApiClient, requestId: string | undefined) {
  return useQuery({
    queryKey: ["dsar", "packages", requestId ?? ""] as const,
    enabled: !!requestId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dsar/requests/{id}/packages", { params: { path: { id: requestId! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** POST /admin/v1/dsar/requests/{id}/packages — the response carries the access code exactly once. */
export function useDsarPackageMutations(client: ApiClient) {
  const qc = useQueryClient();
  const create = useMutation({
    mutationFn: async ({ requestId, input }: { requestId: string; input: DsarCreatePackageInput }) => {
      const { data, error } = await client.POST("/admin/v1/dsar/requests/{id}/packages", { params: { path: { id: requestId } }, body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: (_d, { requestId }) => qc.invalidateQueries({ queryKey: ["dsar", "packages", requestId] }),
  });
  return { create };
}

// DSAR-15: a machine-readable export of this request's own subject data, delivered through the exact same
// DSAR-14 package mechanism above — no separate list/read, just another way to create one.
export type DsarGeneratePortabilityExportInput = components["schemas"]["DsarGeneratePortabilityExportInput"];

/** POST /admin/v1/dsar/requests/{id}/export — the response carries the access code exactly once. */
export function useDsarPortabilityExport(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ requestId, input }: { requestId: string; input: DsarGeneratePortabilityExportInput }) => {
      const { data, error } = await client.POST("/admin/v1/dsar/requests/{id}/export", { params: { path: { id: requestId } }, body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: (_d, { requestId }) => qc.invalidateQueries({ queryKey: ["dsar", "packages", requestId] }),
  });
}

// DSAR-10 (ม.33): legal holds (tenant-wide, DPO-managed) and the per-request exemption checks run against
// them before an erasure request can close.
export type DsarLegalHold = components["schemas"]["DsarLegalHold"];
export type DsarLegalHoldInput = components["schemas"]["DsarLegalHoldInput"];
export type DsarExemptionCheck = components["schemas"]["DsarExemptionCheck"];
export type DsarExemptionResult = components["schemas"]["DsarExemptionResult"];
export type DsarRunExemptionCheckInput = components["schemas"]["DsarRunExemptionCheckInput"];

/** GET /admin/v1/dsar/legal-holds. */
export function useDsarLegalHolds(client: ApiClient) {
  return useQuery({
    queryKey: ["dsar", "legal-holds"] as const,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dsar/legal-holds", {});
      if (error) throw error;
      return data.data;
    },
  });
}

/** Create (POST) and deactivate (DELETE, ETag/If-Match) a legal hold. */
export function useDsarLegalHoldMutations(client: ApiClient) {
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: ["dsar", "legal-holds"] });
  const create = useMutation({
    mutationFn: async (input: DsarLegalHoldInput) => {
      const { data, error } = await client.POST("/admin/v1/dsar/legal-holds", { body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: invalidate,
  });
  const deactivate = useMutation({
    mutationFn: async ({ id, rowVersion }: { id: string; rowVersion: number }) => {
      const { data, error } = await client.DELETE("/admin/v1/dsar/legal-holds/{id}", {
        params: { path: { id }, header: { "If-Match": `"${rowVersion}"` } },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: invalidate,
  });
  return { create, deactivate };
}

/** GET /admin/v1/dsar/requests/{id}/exemption-checks. */
export function useDsarExemptionChecks(client: ApiClient, requestId: string | undefined) {
  return useQuery({
    queryKey: ["dsar", "exemption-checks", requestId ?? ""] as const,
    enabled: !!requestId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dsar/requests/{id}/exemption-checks", { params: { path: { id: requestId! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** POST /admin/v1/dsar/requests/{id}/exemption-checks — the acceptance criterion itself (ม.33). */
export function useDsarRunExemptionCheck(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ requestId, input }: { requestId: string; input: DsarRunExemptionCheckInput }) => {
      const { data, error } = await client.POST("/admin/v1/dsar/requests/{id}/exemption-checks", { params: { path: { id: requestId } }, body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: (_d, { requestId }) => qc.invalidateQueries({ queryKey: ["dsar", "exemption-checks", requestId] }),
  });
}
