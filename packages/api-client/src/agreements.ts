import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type Agreement = components["schemas"]["Agreement"];
export type AgreementCreateInput = components["schemas"]["AgreementCreateInput"];
export type AgreementType = components["schemas"]["AgreementType"];
export type AgreementClause = components["schemas"]["AgreementClause"];
export type AgreementClauseInput = components["schemas"]["AgreementClauseInput"];
export type AgreementMissingClause = components["schemas"]["AgreementMissingClause"];
export type AgreementProcessingSchedule = components["schemas"]["AgreementProcessingSchedule"];
export type AgreementScheduleInput = components["schemas"]["AgreementScheduleInput"];
export type VendorContractStatus = components["schemas"]["VendorContractStatus"];
export type AgreementTypeRecommendation = components["schemas"]["AgreementTypeRecommendation"];
export type AgreementParty = components["schemas"]["AgreementParty"];
export type AgreementPartyInput = components["schemas"]["AgreementPartyInput"];
export type AgreementCounterpartyRole = "processor" | "controller" | "joint_controller";

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

/** GET /admin/v1/agreements/{id}/parties (DSA-02) — every party, the original counterparty plus any added. */
export function useAgreementParties(client: ApiClient, agreementId: string | undefined) {
  return useQuery({
    queryKey: [...agreementsKey, agreementId ?? "", "parties"],
    enabled: !!agreementId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/agreements/{id}/parties", { params: { path: { id: agreementId! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** POST /admin/v1/agreements/{id}/parties — add a party beyond the original counterparty. */
export function useAddAgreementParty(client: ApiClient, agreementId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: AgreementPartyInput) => {
      const { data, error } = await client.POST("/admin/v1/agreements/{id}/parties", { params: { path: { id: agreementId } }, body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: [...agreementsKey, agreementId, "parties"] }),
  });
}

/** DELETE /admin/v1/agreements/{id}/parties/{partyRowId}. */
export function useRemoveAgreementParty(client: ApiClient, agreementId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (partyRowId: string) => {
      const { error } = await client.DELETE("/admin/v1/agreements/{id}/parties/{partyRowId}", {
        params: { path: { id: agreementId, partyRowId } },
      });
      if (error) throw error;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: [...agreementsKey, agreementId, "parties"] }),
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

/** PATCH /admin/v1/agreements/{id}/schedule — DPA-10: the registry's own start/end dates and renewal
 * settings; reschedules the expiry reminder. */
export function useSetAgreementSchedule(client: ApiClient, agreementId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { rowVersion: number; input: AgreementScheduleInput }) => {
      const { data, error } = await client.PATCH("/admin/v1/agreements/{id}/schedule", {
        params: { path: { id: agreementId }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: v.input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: [...agreementsKey, agreementId] }),
  });
}

/** GET /admin/v1/agreements/vendor-contract-status — VEN-11: flags a processor vendor with no DPA on
 * record, computed live from the agreement registry (never persisted, so it can't go stale). */
export function useVendorContractStatus(client: ApiClient, vendorId: string | undefined) {
  return useQuery({
    queryKey: [...agreementsKey, "vendor-contract-status", vendorId ?? ""],
    enabled: !!vendorId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/agreements/vendor-contract-status", {
        params: { query: { vendor_id: vendorId! } },
      });
      if (error) throw error;
      return data;
    },
  });
}

/** GET /admin/v1/agreements/type-check (DSA-01) — picked on demand from the wizard, one role at a time. */
export function useAgreementTypeCheck(client: ApiClient) {
  return useMutation({
    mutationFn: async (counterpartyRole: AgreementCounterpartyRole) => {
      const { data, error } = await client.GET("/admin/v1/agreements/type-check", {
        params: { query: { counterparty_role: counterpartyRole } },
      });
      if (error) throw error;
      return data;
    },
  });
}
