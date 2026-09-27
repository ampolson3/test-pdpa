import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type Notice = components["schemas"]["Notice"];
export type NoticeType = components["schemas"]["NoticeType"];
export type NoticeStatus = components["schemas"]["NoticeStatus"];
export type NoticeWizardInput = components["schemas"]["NoticeWizardInput"];
export type NoticeChecklistItem = components["schemas"]["NoticeChecklistItem"];
export type IndirectCollection = components["schemas"]["IndirectCollection"];
export type IndirectCollectionInput = components["schemas"]["IndirectCollectionInput"];
export type IndirectCollectionStatus = components["schemas"]["IndirectCollectionStatus"];
export type IndirectCollectionMethod = components["schemas"]["IndirectCollectionMethod"];
export type RecordIndirectNoticeInput = components["schemas"]["RecordIndirectNoticeInput"];

const noticesKey = ["notice", "notices"] as const;
const noticeKey = (id: string) => [...noticesKey, id] as const;
const indirectKey = ["notice", "indirect-collections"] as const;
const ifMatch = (v: number) => ({ "If-Match": `"${v}"` });

/** GET /admin/v1/notices (PNG-01), newest first. */
export function useNotices(client: ApiClient, filter: { notice_type?: NoticeType; status?: NoticeStatus } = {}) {
  return useInfiniteQuery({
    queryKey: [...noticesKey, filter],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/notices", {
        params: { query: { notice_type: filter.notice_type, status: filter.status, cursor: pageParam, limit: 50 } },
      });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

/** GET /admin/v1/notices/{id}. */
export function useNotice(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: noticeKey(id ?? ""),
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/notices/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** GET /admin/v1/notices/{id}/checklist (PNG-02) — the six ม.23 mandatory topics and whether each is filled in. */
export function useNoticeChecklist(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...noticeKey(id ?? ""), "checklist"],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/notices/{id}/checklist", { params: { path: { id: id! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** GET /admin/v1/notices/{id}/translation-status (PNG-05) — whether the English content is stale relative to the current Thai draft. */
export function useNoticeTranslationStatus(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...noticeKey(id ?? ""), "translation-status"],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/notices/{id}/translation-status", { params: { path: { id: id! } } });
      if (error) throw error;
      return data.stale;
    },
  });
}

/** POST /admin/v1/notices — the wizard itself: composes a draft document from the linked RoPA activities. */
export function useCreateNoticeWizard(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: NoticeWizardInput) => {
      const { data, error } = await client.POST("/admin/v1/notices", { body });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: noticesKey }),
  });
}

/** GET /admin/v1/notices/indirect-collections (PNG-04, ม.25), soonest deadline first. */
export function useIndirectCollections(client: ApiClient, filter: { status?: IndirectCollectionStatus } = {}) {
  return useInfiniteQuery({
    queryKey: [...indirectKey, filter],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/notices/indirect-collections", {
        params: { query: { status: filter.status, cursor: pageParam, limit: 50 } },
      });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

export function useIndirectCollection(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...indirectKey, id ?? ""],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/notices/indirect-collections/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** Registering an event and recording the eventual notice (PNG-04's acceptance criterion). */
export function useIndirectCollectionMutations(client: ApiClient) {
  const qc = useQueryClient();
  const refresh = () => qc.invalidateQueries({ queryKey: indirectKey });
  return {
    register: useMutation({
      mutationFn: async (body: IndirectCollectionInput) => {
        const { data, error } = await client.POST("/admin/v1/notices/indirect-collections", { body });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    recordNotice: useMutation({
      mutationFn: async (v: { collection: IndirectCollection; input: RecordIndirectNoticeInput }) => {
        const { data, error } = await client.POST("/admin/v1/notices/indirect-collections/{id}/notify", {
          params: { path: { id: v.collection.id }, header: ifMatch(v.collection.row_version) },
          body: v.input,
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
  };
}
