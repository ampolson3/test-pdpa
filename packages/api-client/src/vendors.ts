import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type Vendor = components["schemas"]["Vendor"];
export type VendorInput = components["schemas"]["VendorInput"];
export type VendorStatus = components["schemas"]["VendorStatus"];
export type VendorTier = components["schemas"]["VendorTier"];
export type VendorIntake = components["schemas"]["VendorIntake"];
export type VendorIntakeInput = components["schemas"]["VendorIntakeInput"];
export type VendorIntakeResult = components["schemas"]["VendorIntakeResult"];

const vendorsKey = ["vendor", "vendors"] as const;

/** GET /admin/v1/vendors (VEN-01), newest first. */
export function useVendors(client: ApiClient, filter: { status?: VendorStatus } = {}) {
  return useInfiniteQuery({
    queryKey: [...vendorsKey, filter],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/vendors", {
        params: { query: { status: filter.status, cursor: pageParam, limit: 50 } },
      });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

/** GET /admin/v1/vendors/{id} — the single page the acceptance criterion calls for. */
export function useVendor(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...vendorsKey, id ?? ""],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/vendors/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** Create (no id) or update (id + row_version as If-Match) a vendor's profile. */
export function useSaveVendor(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id?: string; rowVersion?: number; input: VendorInput }) => {
      if (!v.id) {
        const { data, error } = await client.POST("/admin/v1/vendors", { body: v.input });
        if (error) throw error;
        return data;
      }
      const { data, error } = await client.PATCH("/admin/v1/vendors/{id}", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: v.input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: vendorsKey }),
  });
}

/** GET /admin/v1/vendors/{id}/intakes (VEN-02) — a vendor's tiering rounds, newest first. */
export function useVendorIntakes(client: ApiClient, vendorId: string | undefined) {
  return useQuery({
    queryKey: [...vendorsKey, vendorId ?? "", "intakes"],
    enabled: !!vendorId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/vendors/{id}/intakes", { params: { path: { id: vendorId! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** POST /admin/v1/vendors/{id}/intakes — answer the intake questionnaire once; the system computes the tier. */
export function useRecordVendorIntake(client: ApiClient, vendorId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: VendorIntakeInput) => {
      const { data, error } = await client.POST("/admin/v1/vendors/{id}/intakes", {
        params: { path: { id: vendorId } },
        body: input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [...vendorsKey, vendorId] });
    },
  });
}
