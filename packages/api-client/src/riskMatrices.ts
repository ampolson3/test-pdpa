import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type RiskMatrix = components["schemas"]["RiskMatrix"];
export type RiskMatrixInput = components["schemas"]["RiskMatrixInput"];
export type RiskThreshold = components["schemas"]["RiskThreshold"];
export type RiskLevel = components["schemas"]["RiskLevel"];

const ifMatch = (v: number) => ({ "If-Match": `"${v}"` });
const matricesKey = ["risk", "matrices"] as const;

/** GET /admin/v1/risk/matrices (RRA-02), default first. */
export function useRiskMatrices(client: ApiClient) {
  return useQuery({
    queryKey: matricesKey,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/risk/matrices");
      if (error) throw error;
      return data.data;
    },
  });
}

/** GET /admin/v1/risk/matrices/{id}. */
export function useRiskMatrix(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...matricesKey, id ?? ""],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/risk/matrices/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** Create (no id) or update (id + row_version as If-Match) a risk matrix. */
export function useSaveRiskMatrix(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id?: string; rowVersion?: number; input: RiskMatrixInput }) => {
      if (!v.id) {
        const { data, error } = await client.POST("/admin/v1/risk/matrices", { body: v.input });
        if (error) throw error;
        return data;
      }
      const { data, error } = await client.PUT("/admin/v1/risk/matrices/{id}", {
        params: { path: { id: v.id }, header: ifMatch(v.rowVersion!) },
        body: v.input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: matricesKey }),
  });
}

/** DELETE /admin/v1/risk/matrices/{id}. */
export function useDeleteRiskMatrix(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; rowVersion: number }) => {
      const { error } = await client.DELETE("/admin/v1/risk/matrices/{id}", {
        params: { path: { id: v.id }, header: ifMatch(v.rowVersion) },
      });
      if (error) throw error;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: matricesKey }),
  });
}
