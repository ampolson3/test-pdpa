import { useInfiniteQuery, useMutation } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type AuditEntry = components["schemas"]["AuditEntry"];

export interface AuditFilter {
  actor_id?: string;
  entity_type?: string;
  entity_id?: string;
  action_prefix?: string;
  from?: string; // RFC 3339
  to?: string;
  kind?: "changes" | "requests" | "all";
}

function clean(f: AuditFilter): AuditFilter {
  return Object.fromEntries(Object.entries(f).filter(([, v]) => v !== undefined && v !== "")) as AuditFilter;
}

/** GET /admin/v1/platform/audit-log (ORG-19), newest first, one cursor page at a time. */
export function useAuditLog(client: ApiClient, filter: AuditFilter, pageSize = 50) {
  const query = clean(filter);
  return useInfiniteQuery({
    queryKey: ["platform", "audit-log", query, pageSize],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/platform/audit-log", { params: { query: { ...query, limit: pageSize, cursor: pageParam } } });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

/** The CSV export URL through the BFF (a plain link: the browser downloads it). */
export function auditExportHref(bffBaseUrl: string, filter: AuditFilter): string {
  const qs = new URLSearchParams(clean(filter) as Record<string, string>).toString();
  return `${bffBaseUrl}/admin/v1/platform/audit-log/export${qs ? `?${qs}` : ""}`;
}

/** POST …/verify — replay the tenant's hash chain now. */
export function useVerifyAuditLog(client: ApiClient) {
  return useMutation({
    mutationFn: async () => {
      const { data, error } = await client.POST("/admin/v1/platform/audit-log/verify");
      if (error) throw error;
      return data;
    },
  });
}
