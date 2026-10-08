import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type Agreement = components["schemas"]["Agreement"];
export type AgreementCreateInput = components["schemas"]["AgreementCreateInput"];
export type AgreementType = components["schemas"]["AgreementType"];

const agreementsKey = ["agreement", "agreements"] as const;

/** GET /admin/v1/agreements (DPA-02), newest first. */
export function useAgreements(client: ApiClient, filter: { agreementType?: AgreementType; vendorId?: string } = {}) {
  return useInfiniteQuery({
    queryKey: [...agreementsKey, filter],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/agreements", {
        params: { query: { agreement_type: filter.agreementType, vendor_id: filter.vendorId, cursor: pageParam, limit: 50 } },
      });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

/** GET /admin/v1/agreements/{id}. */
export function useAgreement(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...agreementsKey, id ?? ""],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/agreements/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** POST /admin/v1/agreements — DPA-02's wizard: one call drafts a complete agreement document from a
 * vendor + its RoPA activities (template or manual mode). */
export function useCreateAgreement(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: AgreementCreateInput) => {
      const { data, error } = await client.POST("/admin/v1/agreements", { body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: agreementsKey }),
  });
}
