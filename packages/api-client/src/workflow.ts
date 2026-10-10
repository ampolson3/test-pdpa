import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type WorkflowDefinition = components["schemas"]["WorkflowDefinition"];
export type WorkflowDefinitionInput = components["schemas"]["WorkflowDefinitionInput"];
export type WorkflowDefinitionBody = components["schemas"]["WorkflowDefinitionBody"];
export type WorkflowState = components["schemas"]["WorkflowState"];
export type WorkflowInstance = components["schemas"]["WorkflowInstance"];
export type WorkflowTask = components["schemas"]["WorkflowTask"];
export type MyTask = components["schemas"]["MyTask"];
export type SlaStatus = components["schemas"]["SlaStatus"];
export type LocalizedText = components["schemas"]["LocalizedText"];

const definitionsKey = ["platform", "workflow-definitions"] as const;
const myTasksKey = ["platform", "my-tasks"] as const;

/** GET /admin/v1/platform/workflow-definitions (PLT-05). */
export function useWorkflowDefinitions(client: ApiClient) {
  return useQuery({
    queryKey: definitionsKey,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/workflow-definitions");
      if (error) throw error;
      return data.data;
    },
  });
}

/** Create a workflow (no id) or save the next version of one (id + row_version as If-Match). */
export function useSaveWorkflowDefinition(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id?: string; rowVersion?: number; input: WorkflowDefinitionInput }) => {
      if (!v.id) {
        const { data, error } = await client.POST("/admin/v1/platform/workflow-definitions", { body: v.input });
        if (error) throw error;
        return data;
      }
      const { data, error } = await client.POST("/admin/v1/platform/workflow-definitions/{id}/versions", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: v.input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: definitionsKey }),
  });
}

/** Group picker for task assignees and escalation. */
export function useGroupSearch(client: ApiClient, q: string | null) {
  return useQuery({
    queryKey: ["platform", "assignable-groups", q],
    enabled: q !== null,
    staleTime: 30_000,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/assignable-groups", { params: { query: { q: q ?? "" } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** GET /admin/v1/platform/my-tasks — refreshed every 30 s (SLA badges move with time). */
export function useMyTasks(client: ApiClient) {
  return useQuery({
    queryKey: myTasksKey,
    refetchInterval: 30_000,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/my-tasks");
      if (error) throw error;
      return data.data;
    },
  });
}

/** GET /admin/v1/platform/workflow-instances/{id}. */
export function useWorkflowInstance(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: ["platform", "workflow-instances", id],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/workflow-instances/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** Transitions and task changes; both refresh the instance and the caller's board. */
export function useWorkflowMutations(client: ApiClient) {
  const qc = useQueryClient();
  const refresh = (instanceId: string) => {
    void qc.invalidateQueries({ queryKey: myTasksKey });
    void qc.invalidateQueries({ queryKey: ["platform", "workflow-instances", instanceId] });
  };
  return {
    transition: useMutation({
      mutationFn: async (v: { instance: WorkflowInstance; to: string; comment?: string }) => {
        const { data, error } = await client.POST("/admin/v1/platform/workflow-instances/{id}/transitions", {
          params: { path: { id: v.instance.id }, header: { "If-Match": `"${v.instance.row_version}"` } },
          body: { to: v.to, comment: v.comment || undefined },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: (inst) => {
        qc.setQueryData(["platform", "workflow-instances", inst.id], inst);
        refresh(inst.id);
      },
    }),
    updateTask: useMutation({
      mutationFn: async (v: { task: WorkflowTask; assigneeUserId?: string; status?: "open" | "in_progress" }) => {
        const { data, error } = await client.PATCH("/admin/v1/platform/workflow-tasks/{id}", {
          params: { path: { id: v.task.id }, header: { "If-Match": `"${v.task.row_version}"` } },
          body: { assignee_user_id: v.assigneeUserId, status: v.status },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: (t) => refresh(t.instance_id),
    }),
  };
}
