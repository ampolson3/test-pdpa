import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";
import type { DpoRemediationTask } from "./dpo";

export type DpoTaskStatus = DpoRemediationTask["status"];
export type DpoAssignTaskInput = components["schemas"]["DpoAssignTaskInput"];

const taskKey = (id: string) => ["dpo", "task", id] as const;
const taskListKey = ["dpo", "tasks"] as const;

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

/** GET /admin/v1/dpo/tasks (DPO-06) — the kanban/list view of every dpo.tasks job, whatever module opened
 *  it, filterable by status/source_type/priority/assignee. */
export function useTasks(
  client: ApiClient,
  filter: { status?: DpoTaskStatus; source_type?: string; priority?: string; assignee_user_id?: string } = {},
) {
  return useInfiniteQuery({
    queryKey: [...taskListKey, filter],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/dpo/tasks", {
        params: {
          query: {
            status: filter.status as never,
            source_type: filter.source_type as never,
            priority: filter.priority as never,
            assignee_user_id: filter.assignee_user_id,
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

/** POST /admin/v1/dpo/tasks/{id}/assign (DPO-06) — set or change who is assigned, who reviews, the due
 *  date and the priority on an already-created task; needs dpo.task.execute. */
export function useAssignTask(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, rowVersion, input }: { id: string; rowVersion: number; input: DpoAssignTaskInput }) => {
      const { data, error } = await client.POST("/admin/v1/dpo/tasks/{id}/assign", {
        params: { path: { id }, header: { "If-Match": `"${rowVersion}"` } },
        body: input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (_, { id }) => {
      qc.invalidateQueries({ queryKey: taskKey(id) });
      qc.invalidateQueries({ queryKey: taskListKey });
    },
  });
}
