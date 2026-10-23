import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type ImportJob = components["schemas"]["ImportJob"];

const busy = new Set(["validating", "importing"]);

/** GET /admin/v1/platform/imports/{id}, polled while the worker is busy with it (PLT-14). */
export function useImport(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: ["platform", "imports", id],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/imports/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
    refetchInterval: (q) => {
      const j = q.state.data;
      return j && (busy.has(j.status) || (j.status === "queued" && j.headers.length === 0)) ? 1_000 : false;
    },
  });
}

export function useImportMutations(client: ApiClient) {
  const qc = useQueryClient();
  const set = (j: ImportJob) => qc.setQueryData(["platform", "imports", j.id], j);
  return {
    create: useMutation({
      mutationFn: async (v: { importType: string; fileId: string }) => {
        const { data, error } = await client.POST("/admin/v1/platform/imports", { body: { import_type: v.importType, file_id: v.fileId } });
        if (error) throw error;
        return data;
      },
      onSuccess: set,
    }),
    map: useMutation({
      mutationFn: async (v: { job: ImportJob; columns: Record<string, string> }) => {
        const { data, error } = await client.PUT("/admin/v1/platform/imports/{id}/mapping", {
          params: { path: { id: v.job.id }, header: { "If-Match": `"${v.job.row_version}"` } },
          body: { columns: v.columns },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: set,
    }),
    confirm: useMutation({
      mutationFn: async (job: ImportJob) => {
        const { data, error } = await client.POST("/admin/v1/platform/imports/{id}/confirm", {
          params: { path: { id: job.id }, header: { "If-Match": `"${job.row_version}"` } },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: set,
    }),
  };
}

/** Link to the error report: the BFF answers 302 to a short-lived signed URL. */
export function importErrorsHref(bffBaseUrl: string, id: string): string {
  return `${bffBaseUrl}/admin/v1/platform/imports/${id}/errors`;
}
