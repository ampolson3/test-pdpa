import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type Agreement = components["schemas"]["Agreement"];
export type AgreementCreateInput = components["schemas"]["AgreementCreateInput"];
export type AgreementType = components["schemas"]["AgreementType"];
export type AgreementClause = components["schemas"]["AgreementClause"];
export type AgreementClauseInput = components["schemas"]["AgreementClauseInput"];
export type AgreementMissingClause = components["schemas"]["AgreementMissingClause"];
export type AgreementProcessingSchedule = components["schemas"]["AgreementProcessingSchedule"];

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

/** GET /admin/v1/agreements/{id}/clauses — DPA-03: every clause attached to an agreement. */
export function useAgreementClauses(client: ApiClient, agreementId: string | undefined) {
  return useQuery({
    queryKey: [...agreementsKey, agreementId ?? "", "clauses"],
    enabled: !!agreementId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/agreements/{id}/clauses", { params: { path: { id: agreementId! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** GET /admin/v1/agreements/{id}/missing-clauses — DPA-03's own "แผงตรวจ clause ที่ขาด", live. */
export function useMissingClauses(client: ApiClient, agreementId: string | undefined) {
  return useQuery({
    queryKey: [...agreementsKey, agreementId ?? "", "missing-clauses"],
    enabled: !!agreementId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/agreements/{id}/missing-clauses", { params: { path: { id: agreementId! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

function invalidateClauses(qc: ReturnType<typeof useQueryClient>, agreementId: string) {
  qc.invalidateQueries({ queryKey: [...agreementsKey, agreementId, "clauses"] });
  qc.invalidateQueries({ queryKey: [...agreementsKey, agreementId, "missing-clauses"] });
}

/** POST /admin/v1/agreements/{id}/clauses — attach a published clause from the library. */
export function useAddAgreementClause(client: ApiClient, agreementId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: AgreementClauseInput) => {
      const { data, error } = await client.POST("/admin/v1/agreements/{id}/clauses", { params: { path: { id: agreementId } }, body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: () => invalidateClauses(qc, agreementId),
  });
}

/** DELETE /admin/v1/agreements/{id}/clauses/{clauseRowId} — detach a clause. */
export function useRemoveAgreementClause(client: ApiClient, agreementId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (clauseRowId: string) => {
      const { error } = await client.DELETE("/admin/v1/agreements/{id}/clauses/{clauseRowId}", {
        params: { path: { id: agreementId, clauseRowId } },
      });
      if (error) throw error;
    },
    onSuccess: () => invalidateClauses(qc, agreementId),
  });
}

/** GET /admin/v1/agreements/{id}/processing-schedule — DPA-04's own ม.40 annex, composed live from every
 * RoPA activity the agreement covers. */
export function useProcessingSchedule(client: ApiClient, agreementId: string | undefined) {
  return useQuery({
    queryKey: [...agreementsKey, agreementId ?? "", "processing-schedule"],
    enabled: !!agreementId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/agreements/{id}/processing-schedule", { params: { path: { id: agreementId! } } });
      if (error) throw error;
      return data;
    },
  });
}
