import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type ExternalParty = components["schemas"]["ExternalParty"];
export type ExternalPartyInput = components["schemas"]["ExternalPartyInput"];
export type ExternalPartyType = components["schemas"]["ExternalPartyType"];
export type ExternalPartyDuplicateGroup = components["schemas"]["ExternalPartyDuplicateGroup"];

const partiesKey = ["org", "external-parties"] as const;

/** GET /admin/v1/org/external-parties (ORG-06), newest first. */
export function useExternalParties(client: ApiClient, filter: { party_type?: ExternalPartyType; country_code?: string; q?: string } = {}) {
  return useInfiniteQuery({
    queryKey: [...partiesKey, filter],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/org/external-parties", {
        params: { query: { party_type: filter.party_type, country_code: filter.country_code, q: filter.q || undefined, cursor: pageParam, limit: 50 } },
      });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

export function useExternalPartyDuplicates(client: ApiClient) {
  return useQuery({
    queryKey: [...partiesKey, "duplicates"],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/org/external-parties/duplicates");
      if (error) throw error;
      return data.data;
    },
  });
}

export function useExternalPartyMutations(client: ApiClient) {
  const qc = useQueryClient();
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: partiesKey });
  };
  return {
    save: useMutation({
      mutationFn: async (v: { id?: string; rowVersion?: number; input: ExternalPartyInput }) => {
        if (!v.id) {
          const { data, error } = await client.POST("/admin/v1/org/external-parties", { body: v.input });
          if (error) throw error;
          return data;
        }
        const { data, error } = await client.PATCH("/admin/v1/org/external-parties/{id}", {
          params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
          body: v.input,
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    merge: useMutation({
      mutationFn: async (v: { sourceId: string; targetId: string }) => {
        // The duplicates list is a slim view with no row_version, so read the current one right
        // before merging (If-Match must match the live row, not a stale one from that list).
        const cur = await client.GET("/admin/v1/org/external-parties/{id}", { params: { path: { id: v.sourceId } } });
        if (cur.error) throw cur.error;
        const { data, error } = await client.POST("/admin/v1/org/external-parties/{id}/merge", {
          params: { path: { id: v.sourceId }, header: { "If-Match": `"${cur.data.row_version}"` } },
          body: { target_id: v.targetId },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
  };
}
