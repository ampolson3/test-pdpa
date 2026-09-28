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
