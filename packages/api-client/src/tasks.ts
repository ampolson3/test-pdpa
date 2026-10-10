import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";
import type { DpoRemediationTask } from "./dpo";

export type DpoTaskStatus = DpoRemediationTask["status"];

const taskKey = (id: string) => ["dpo", "task", id] as const;

/** GET /admin/v1/dpo/tasks/{id} (RRA-07) — one remediation job's current status, whatever opened it. */
export function useTask(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: taskKey(id ?? ""),
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/dpo/tasks/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** POST /admin/v1/dpo/tasks/{id}/status (RRA-07) — move the task forward; closing a "ropa_gap" one
 *  re-checks that finding's rule on the backend. */
export function useUpdateTaskStatus(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, status, rowVersion }: { id: string; status: DpoTaskStatus; rowVersion: number }) => {
      const { data, error } = await client.POST("/admin/v1/dpo/tasks/{id}/status", {
        params: { path: { id }, header: { "If-Match": `"${rowVersion}"` } },
        body: { status },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (_, { id }) => qc.invalidateQueries({ queryKey: taskKey(id) }),
  });
}
