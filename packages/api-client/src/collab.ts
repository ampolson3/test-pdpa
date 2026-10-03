import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type Comment = components["schemas"]["Comment"];
export type Attachment = components["schemas"]["Attachment"];
export type Activity = components["schemas"]["Activity"];

type Rec = { entityType: string; entityId: string };
const key = (r: Rec, part: string) => ["platform", "records", r.entityType, r.entityId, part] as const;
const path = (r: Rec) => ({ path: { entityType: r.entityType, entityId: r.entityId } });

/** PLT-07: a record's comments, attachments and activity — the same hooks for every module. */
export function useComments(client: ApiClient, r: Rec) {
  return useQuery({
    queryKey: key(r, "comments"),
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/records/{entityType}/{entityId}/comments", { params: path(r) });
      if (error) throw error;
      return data.data;
    },
  });
}

export function useAttachments(client: ApiClient, r: Rec) {
  return useQuery({
    queryKey: key(r, "attachments"),
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/records/{entityType}/{entityId}/attachments", { params: path(r) });
      if (error) throw error;
      return data.data;
    },
    refetchInterval: (q) => (q.state.data?.some((a) => a.av_status === "pending") ? 2_000 : false),
  });
}

export function useActivity(client: ApiClient, r: Rec) {
  return useQuery({
    queryKey: key(r, "activity"),
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/records/{entityType}/{entityId}/activity", { params: { ...path(r), query: { limit: 100 } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** All comment and attachment changes of a record; each refreshes the record's lists and activity. */
export function useCollabMutations(client: ApiClient, r: Rec) {
  const qc = useQueryClient();
  const refresh = () => qc.invalidateQueries({ queryKey: ["platform", "records", r.entityType, r.entityId] });
  const unwrap = <T,>(res: { data?: T; error?: unknown }) => {
    if (res.error) throw res.error;
    return res.data as T;
  };
  return {
    add: useMutation({
      mutationFn: async (v: { body: string; parentId?: string }) =>
        unwrap(await client.POST("/admin/v1/platform/records/{entityType}/{entityId}/comments", { params: path(r), body: { body: v.body, parent_id: v.parentId } })),
      onSuccess: refresh,
    }),
    edit: useMutation({
      mutationFn: async (v: { id: string; rowVersion: number; body: string }) =>
        unwrap(await client.PATCH("/admin/v1/platform/comments/{id}", { params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } }, body: { body: v.body } })),
      onSuccess: refresh,
    }),
    remove: useMutation({
      mutationFn: async (v: { id: string; rowVersion: number }) =>
        unwrap(await client.DELETE("/admin/v1/platform/comments/{id}", { params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } } })),
      onSuccess: refresh,
    }),
    resolve: useMutation({
      mutationFn: async (v: { id: string; resolved: boolean }) =>
        unwrap(await client.POST("/admin/v1/platform/comments/{id}/resolve", { params: { path: { id: v.id } }, body: { resolved: v.resolved } })),
      onSuccess: refresh,
    }),
    attach: useMutation({
      mutationFn: async (fileId: string) =>
        unwrap(await client.POST("/admin/v1/platform/records/{entityType}/{entityId}/attachments", { params: path(r), body: { file_id: fileId } })),
      onSuccess: refresh,
    }),
  };
}

/** @mention picker: active users whose name starts with q. */
export function useMentionSearch(client: ApiClient, q: string | null) {
  return useQuery({
    queryKey: ["platform", "mentionable-users", q],
    enabled: !!q,
    staleTime: 30_000,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/mentionable-users", { params: { query: { q: q! } } });
      if (error) throw error;
      return data.data;
    },
  });
}
