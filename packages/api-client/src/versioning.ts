import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type RecordVersion = components["schemas"]["RecordVersion"];
export type VersionChange = components["schemas"]["VersionChange"];
export type ApprovalInboxItem = components["schemas"]["ApprovalInboxItem"];

/** GET /admin/v1/platform/records/{type}/{id}/versions (PLT-08), newest first. */
export function useRecordVersions(client: ApiClient, entityType: string, entityId: string) {
  return useQuery({
    queryKey: ["platform", "versions", entityType, entityId],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/records/{entityType}/{entityId}/versions", { params: { path: { entityType, entityId } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** GET …/record-versions/{id} — one version with its approval steps. */
export function useRecordVersion(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: ["platform", "record-versions", id],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/record-versions/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** GET …/record-versions/{from}/compare?with={to}. */
export function useCompareVersions(client: ApiClient, from: string | undefined, to: string | undefined) {
  return useQuery({
    queryKey: ["platform", "record-versions", from, "compare", to],
    enabled: !!from && !!to && from !== to,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/record-versions/{id}/compare", { params: { path: { id: from! }, query: { with: to! } } });
      if (error) throw error;
      return data.changes;
    },
  });
}

/** GET /admin/v1/platform/my-approvals — the approval inbox. */
export function useMyApprovals(client: ApiClient) {
  return useQuery({
    queryKey: ["platform", "my-approvals"],
    refetchInterval: 30_000,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/my-approvals");
      if (error) throw error;
      return data.data;
    },
  });
}

/** Submit, publish and decide; each refreshes the versions, the version and the inbox. */
export function useVersionMutations(client: ApiClient) {
  const qc = useQueryClient();
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["platform", "versions"] });
    void qc.invalidateQueries({ queryKey: ["platform", "record-versions"] });
    void qc.invalidateQueries({ queryKey: ["platform", "my-approvals"] });
    // Records whose module reacts to publishing (e.g. PLT-16 documents render their files) show the result at once.
    void qc.invalidateQueries({ queryKey: ["docs"] });
  };
  const ifMatch = (v: number) => ({ "If-Match": `"${v}"` });
  return {
    submit: useMutation({
      mutationFn: async (v: RecordVersion) => {
        const { data, error } = await client.POST("/admin/v1/platform/record-versions/{id}/submit", { params: { path: { id: v.id }, header: ifMatch(v.row_version) } });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    publish: useMutation({
      mutationFn: async (v: RecordVersion) => {
        const { data, error } = await client.POST("/admin/v1/platform/record-versions/{id}/publish", { params: { path: { id: v.id }, header: ifMatch(v.row_version) } });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    decide: useMutation({
      mutationFn: async (v: { approvalId: string; rowVersion: number; decision: "approved" | "returned" | "rejected"; reason?: string }) => {
        const { data, error } = await client.POST("/admin/v1/platform/approvals/{id}/decision", {
          params: { path: { id: v.approvalId }, header: ifMatch(v.rowVersion) },
          body: { decision: v.decision, reason: v.reason || undefined },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
  };
}
