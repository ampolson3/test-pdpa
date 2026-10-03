import { useEffect, useState } from "react";
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type NotificationTemplate = components["schemas"]["NotificationTemplate"];
export type NotificationTemplateInput = components["schemas"]["NotificationTemplateInput"];
export type NotificationDelivery = components["schemas"]["NotificationDelivery"];
export type NotificationStatus = components["schemas"]["NotificationStatus"];
export type NotificationChannel = components["schemas"]["NotificationChannel"];
export type Inbox = components["schemas"]["Inbox"];

const templatesKey = ["platform", "notification-templates"] as const;

/** GET /admin/v1/platform/notification-templates (PLT-04). */
export function useNotificationTemplates(client: ApiClient) {
  return useQuery({
    queryKey: templatesKey,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/notification-templates");
      if (error) throw error;
      return data.data;
    },
  });
}

/** Create, or save (PATCH with If-Match = row_version), or delete a template. */
export function useSaveNotificationTemplate(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id?: string; rowVersion?: number; input: NotificationTemplateInput }) => {
      if (!v.id) {
        const { data, error } = await client.POST("/admin/v1/platform/notification-templates", { body: v.input });
        if (error) throw error;
        return data;
      }
      const { data, error } = await client.PATCH("/admin/v1/platform/notification-templates/{id}", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: { subject: v.input.subject, body: v.input.body, variables: v.input.variables },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: templatesKey }),
  });
}

export function useDeleteNotificationTemplate(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; rowVersion: number }) => {
      const { error } = await client.DELETE("/admin/v1/platform/notification-templates/{id}", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
      });
      if (error) throw error;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: templatesKey }),
  });
}

/** POST …/preview — re-rendered whenever the draft changes (callers debounce). */
export function useTemplatePreview(
  client: ApiClient,
  draft: { subject?: string; body: string; variables?: string[]; values?: Record<string, string> },
) {
  return useQuery({
    queryKey: ["platform", "notification-templates", "preview", draft],
    enabled: draft.body.trim() !== "",
    retry: false,
    queryFn: async () => {
      const { data, error } = await client.POST("/admin/v1/platform/notification-templates/preview", { body: draft });
      if (error) throw error;
      return data;
    },
  });
}

/** GET /admin/v1/platform/notifications — the delivery log, newest first, refreshed every 10 s. */
export function useNotificationDeliveries(client: ApiClient, filter: { status?: NotificationStatus; channel?: NotificationChannel }) {
  return useInfiniteQuery({
    queryKey: ["platform", "notifications", filter],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/platform/notifications", {
        params: { query: { cursor: pageParam, limit: 50, status: filter.status, channel: filter.channel } },
      });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    refetchInterval: 10_000,
  });
}

const inboxKey = ["platform", "inbox"] as const;

/** GET /admin/v1/platform/inbox — the bell's list. */
export function useInbox(client: ApiClient, enabled: boolean) {
  return useQuery({
    queryKey: inboxKey,
    enabled,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/inbox", { params: { query: { limit: 20 } } });
      if (error) throw error;
      return data;
    },
  });
}

export function useMarkInboxRead(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      const { error } = await client.POST("/admin/v1/platform/inbox/{id}/read", { params: { path: { id } } });
      if (error) throw error;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: inboxKey }),
  });
}

/**
 * Unread in-app count pushed by the server (SSE, GET /admin/v1/platform/inbox/stream through the BFF).
 * EventSource reconnects on its own; each new count also refreshes the inbox list.
 */
export function useUnreadCount(bffBaseUrl: string): number | undefined {
  const qc = useQueryClient();
  const [unread, setUnread] = useState<number>();
  useEffect(() => {
    const es = new EventSource(`${bffBaseUrl}/admin/v1/platform/inbox/stream`);
    es.addEventListener("unread", (e) => {
      const n = (JSON.parse((e as MessageEvent<string>).data) as { unread: number }).unread;
      setUnread(n);
      void qc.invalidateQueries({ queryKey: inboxKey });
    });
    return () => es.close();
  }, [bffBaseUrl, qc]);
  return unread;
}
