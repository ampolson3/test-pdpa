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
 *  (substring) or an e-mail (exact blind-index match). */
export function useDsarRequests(client: ApiClient, filter: { status?: DsarRequestStatus; search?: string } = {}) {
  return useInfiniteQuery({
    queryKey: [...requestsKey, filter],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/dsar/requests", {
        params: { query: { status: filter.status, search: filter.search || undefined, cursor: pageParam, limit: 50 } },
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
