import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type Notice = components["schemas"]["Notice"];
export type NoticeType = components["schemas"]["NoticeType"];
export type NoticeStatus = components["schemas"]["NoticeStatus"];
export type NoticeWizardInput = components["schemas"]["NoticeWizardInput"];

const noticesKey = ["notice", "notices"] as const;
const noticeKey = (id: string) => [...noticesKey, id] as const;

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
