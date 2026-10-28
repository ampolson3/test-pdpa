import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type RetentionSchedule = components["schemas"]["RetentionSchedule"];
export type RetentionScheduleInput = components["schemas"]["RetentionScheduleInput"];
export type RetentionScheduleStatus = components["schemas"]["RetentionScheduleStatus"];
export type DisposalJob = components["schemas"]["DisposalJob"];
export type DisposalJobStatus = components["schemas"]["DisposalJobStatus"];
export type DisposalJobCompleteInput = components["schemas"]["DisposalJobCompleteInput"];
export type GovPolicy = components["schemas"]["GovPolicy"];
export type GovPolicyInput = components["schemas"]["GovPolicyInput"];
export type GovPolicyAttestation = components["schemas"]["GovPolicyAttestation"];
export type GovMissingAttestation = components["schemas"]["GovMissingAttestation"];

const schedulesKey = ["gov", "retention-schedules"] as const;
const jobsKey = ["gov", "disposal-jobs"] as const;
const policiesKey = ["gov", "policies"] as const;
const attestationsKey = ["gov", "policy-attestations"] as const;
const missingAttestationsKey = ["gov", "missing-attestations"] as const;

/** GET /admin/v1/governance/retention-schedules (DPX-05), newest first. */
export function useRetentionSchedules(client: ApiClient, filter: { assetId?: string; status?: RetentionScheduleStatus } = {}) {
  return useInfiniteQuery({
    queryKey: [...schedulesKey, filter],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/governance/retention-schedules", {
        params: { query: { asset_id: filter.assetId, status: filter.status, cursor: pageParam, limit: 50 } },
      });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

/** Create a retention schedule against a RoPA retention rule and asset. */
export function useCreateRetentionSchedule(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: RetentionScheduleInput) => {
      const { data, error } = await client.POST("/admin/v1/governance/retention-schedules", { body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: schedulesKey }),
  });
}

/** PATCH /admin/v1/governance/retention-schedules/{id} — pause / resume / retire. */
export function useSetRetentionScheduleStatus(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; rowVersion: number; status: RetentionScheduleStatus }) => {
      const { data, error } = await client.PATCH("/admin/v1/governance/retention-schedules/{id}", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: { status: v.status },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: schedulesKey }),
  });
}

/** GET /admin/v1/governance/disposal-jobs — created automatically by the daily due-date sweep. */
export function useDisposalJobs(client: ApiClient, filter: { status?: DisposalJobStatus } = {}) {
  return useInfiniteQuery({
    queryKey: [...jobsKey, filter],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/governance/disposal-jobs", {
        params: { query: { status: filter.status, cursor: pageParam, limit: 50 } },
      });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

/** POST /admin/v1/governance/disposal-jobs/{id}/approve — refused if the asset is under an active legal hold. */
export function useApproveDisposalJob(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; rowVersion: number; assigneeUserId?: string }) => {
      const { data, error } = await client.POST("/admin/v1/governance/disposal-jobs/{id}/approve", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: { assignee_user_id: v.assigneeUserId },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: jobsKey }),
  });
}

/** POST /admin/v1/governance/disposal-jobs/{id}/start — IT begins the actual disposal work. */
export function useStartDisposalJob(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; rowVersion: number }) => {
      const { data, error } = await client.POST("/admin/v1/governance/disposal-jobs/{id}/start", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: jobsKey }),
  });
}

/** POST /admin/v1/governance/disposal-jobs/{id}/complete — the acceptance criterion's gate: refused without evidence. */
export function useCompleteDisposalJob(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; rowVersion: number; input: DisposalJobCompleteInput }) => {
      const { data, error } = await client.POST("/admin/v1/governance/disposal-jobs/{id}/complete", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: v.input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: jobsKey }),
  });
}

/** POST /admin/v1/governance/disposal-jobs/{id}/release-hold — once the legal hold has been lifted. */
export function useReleaseDisposalJobHold(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; rowVersion: number }) => {
      const { data, error } = await client.POST("/admin/v1/governance/disposal-jobs/{id}/release-hold", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: jobsKey }),
  });
}

/** GET /admin/v1/governance/policies (DPX-02) — every tracked policy, newest published first. */
export function usePolicies(client: ApiClient) {
  return useQuery({
    queryKey: policiesKey,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/governance/policies", {});
      if (error) throw error;
      return data.data;
    },
  });
}

/** POST /admin/v1/governance/policies — track an already-published PLT-16 document's current version. */
export function usePublishPolicy(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: GovPolicyInput) => {
      const { data, error } = await client.POST("/admin/v1/governance/policies", { body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: policiesKey }),
  });
}

/** POST /admin/v1/governance/policies/{id}/attest — the signed-in user acknowledges the current version. */
export function useAttestPolicy(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (policyId: string) => {
      const { data, error } = await client.POST("/admin/v1/governance/policies/{id}/attest", { params: { path: { id: policyId } } });
      if (error) throw error;
      return data;
    },
    onSuccess: (_d, policyId) => {
      qc.invalidateQueries({ queryKey: [...attestationsKey, policyId] });
      qc.invalidateQueries({ queryKey: [...missingAttestationsKey, policyId] });
    },
  });
}

/** GET /admin/v1/governance/policies/{id}/attestations — every attestation recorded, oldest first. */
export function usePolicyAttestations(client: ApiClient, policyId: string) {
  return useQuery({
    queryKey: [...attestationsKey, policyId],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/governance/policies/{id}/attestations", { params: { path: { id: policyId } } });
      if (error) throw error;
      return data.data;
    },
    enabled: !!policyId,
  });
}

/** GET /admin/v1/governance/policies/{id}/missing-attestations — the acceptance criterion itself. */
export function useMissingAttestations(client: ApiClient, policyId: string) {
  return useQuery({
    queryKey: [...missingAttestationsKey, policyId],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/governance/policies/{id}/missing-attestations", { params: { path: { id: policyId } } });
      if (error) throw error;
      return data.data;
    },
    enabled: !!policyId,
  });
}
