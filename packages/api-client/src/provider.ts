import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type ProviderTenant = components["schemas"]["ProviderTenant"];
export type ProviderTenantModule = components["schemas"]["ProviderTenantModule"];
export type ProviderCreateTenantInput = components["schemas"]["ProviderCreateTenantInput"];
export type ProviderSetTenantModuleInput = components["schemas"]["ProviderSetTenantModuleInput"];
export type ProviderCreateAdminInput = components["schemas"]["ProviderCreateAdminInput"];
export type ProviderHealth = components["schemas"]["ProviderHealth"];

const ifMatch = (v: number) => ({ "If-Match": `"${v}"` });
const tenantsKey = ["provider", "tenants"] as const;
const modulesKey = (tenantId: string) => ["provider", "tenants", tenantId, "modules"] as const;

/** GET /provider/v1/tenants (IAM-03) — SUPER only, no pagination: the platform's own roster. */
export function useTenants(client: ApiClient) {
  return useQuery({
    queryKey: tenantsKey,
    queryFn: async () => {
      const { data, error } = await client.GET("/provider/v1/tenants", {});
      if (error) throw error;
      return data.data;
    },
  });
}

export function useTenant(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...tenantsKey, id ?? ""],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/provider/v1/tenants/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

export function useCreateTenant(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: ProviderCreateTenantInput) => {
      const { data, error } = await client.POST("/provider/v1/tenants", { body: input });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: tenantsKey }),
  });
}

export function useSuspendTenant(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { tenant: ProviderTenant }) => {
      const { data, error } = await client.POST("/provider/v1/tenants/{id}/suspend", {
        params: { path: { id: v.tenant.id }, header: ifMatch(v.tenant.row_version) },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: tenantsKey }),
  });
}

export function useReactivateTenant(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { tenant: ProviderTenant }) => {
      const { data, error } = await client.POST("/provider/v1/tenants/{id}/reactivate", {
        params: { path: { id: v.tenant.id }, header: ifMatch(v.tenant.row_version) },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: tenantsKey }),
  });
}

/** GET /provider/v1/tenants/{id}/modules — every purchasable module, enabled by default (opt-out). */
export function useTenantModules(client: ApiClient, tenantId: string | undefined) {
  return useQuery({
    queryKey: modulesKey(tenantId ?? ""),
    enabled: !!tenantId,
    queryFn: async () => {
      const { data, error } = await client.GET("/provider/v1/tenants/{id}/modules", { params: { path: { id: tenantId! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** PUT /provider/v1/tenants/{id}/modules/{moduleCode} — the acceptance criterion itself: flip this and
 *  the tenant's menu/API for that module becomes unusable (internal/pkg/modulegate). */
export function useSetTenantModule(client: ApiClient, tenantId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { module: ProviderTenantModule; input: ProviderSetTenantModuleInput }) => {
      const { data, error } = await client.PUT("/provider/v1/tenants/{id}/modules/{moduleCode}", {
        params: { path: { id: tenantId, moduleCode: v.module.module_code }, header: ifMatch(v.module.row_version) },
        body: v.input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: modulesKey(tenantId) }),
  });
}

/** POST /provider/v1/tenants/{id}/admins — DB-only first admin (no Keycloak account in this environment). */
export function useCreateFirstAdmin(client: ApiClient, tenantId: string) {
  return useMutation({
    mutationFn: async (input: ProviderCreateAdminInput) => {
      const { data, error } = await client.POST("/provider/v1/tenants/{id}/admins", {
        params: { path: { id: tenantId } },
        body: input,
      });
      if (error) throw error;
      return data;
    },
  });
}

/** GET /provider/v1/platform/health (PLT-19) — cmd/api's own recent error rate and latency, polled
 * every 15s so the console reflects an alert soon after it crosses its threshold. */
export function useSystemHealth(client: ApiClient) {
  return useQuery({
    queryKey: ["provider", "health"] as const,
    refetchInterval: 15_000,
    queryFn: async () => {
      const { data, error } = await client.GET("/provider/v1/platform/health", {});
      if (error) throw error;
      return data;
    },
  });
}
