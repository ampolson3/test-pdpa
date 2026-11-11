import { useMutation, useQuery, useQueryClient, useInfiniteQuery } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type Connector = components["schemas"]["Connector"];
export type ConnectorInput = components["schemas"]["ConnectorInput"];
export type ConnectorType = components["schemas"]["ConnectorType"];
export type ConnectorTestResult = components["schemas"]["ConnectorTestResult"];
export type ConnectorStatus = components["schemas"]["ConnectorStatus"];

const connectorTypesKey = ["platform", "connector-types"] as const;
const connectorsKey = ["platform", "connectors"] as const;
const connectorKey = (id: string) => [...connectorsKey, id] as const;

/** GET /admin/v1/platform/connector-types (PLT-20) — read live off the process's own Registry, so a new
 *  connector_type registered in internal/wiring.Connectors() shows up here with no frontend change. */
export function useConnectorTypes(client: ApiClient) {
  return useQuery({
    queryKey: connectorTypesKey,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/connector-types", {});
      if (error) throw error;
      return data.data;
    },
  });
}

/** GET /admin/v1/platform/connectors (PLT-20) — the tenant's registered connectors. */
export function useConnectors(client: ApiClient, filter: { connector_type?: string; status?: ConnectorStatus } = {}) {
  return useInfiniteQuery({
    queryKey: [...connectorsKey, filter],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/platform/connectors", {
        params: {
          query: {
            connector_type: filter.connector_type,
            status: filter.status as never,
            cursor: pageParam,
            limit: 50,
          },
        },
      });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

/** GET /admin/v1/platform/connectors/{id}. */
export function useConnector(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: connectorKey(id ?? ""),
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/connectors/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** Create (no id) or update (id + row_version as If-Match) a connector. `secret` is write-only: omit it to
 *  leave an already-sealed secret unchanged, send "" to clear it. */
export function useSaveConnector(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id?: string; rowVersion?: number; input: ConnectorInput }) => {
      if (!v.id) {
        const { data, error } = await client.POST("/admin/v1/platform/connectors", { body: v.input });
        if (error) throw error;
        return data;
      }
      const { data, error } = await client.PATCH("/admin/v1/platform/connectors/{id}", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: v.input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: connectorsKey });
      if (data?.id) qc.invalidateQueries({ queryKey: connectorKey(data.id) });
    },
  });
}

/** DELETE /admin/v1/platform/connectors/{id}. */
export function useDeleteConnector(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id: string; rowVersion: number }) => {
      const { error } = await client.DELETE("/admin/v1/platform/connectors/{id}", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
      });
      if (error) throw error;
    },
    onSuccess: (_data, v) => {
      qc.invalidateQueries({ queryKey: connectorsKey });
      qc.removeQueries({ queryKey: connectorKey(v.id) });
    },
  });
}

/** POST /admin/v1/platform/connectors/{id}/test — PLT-20's own literal acceptance criterion, "ทดสอบการ
 *  เชื่อมต่อจากหน้าจอได้": never throws on a bad connection, the result's own `ok` reports that. */
export function useTestConnector(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      const { data, error } = await client.POST("/admin/v1/platform/connectors/{id}/test", { params: { path: { id } } });
      if (error) throw error;
      return data as ConnectorTestResult;
    },
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: connectorKey(id) });
      qc.invalidateQueries({ queryKey: connectorsKey });
    },
  });
}
