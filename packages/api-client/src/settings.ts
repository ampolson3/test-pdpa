import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type OrgSettings = components["schemas"]["OrgSettings"];
export type OrgSettingsInput = components["schemas"]["OrgSettingsInput"];

const settingsKey = ["org", "settings"] as const;

/** GET /admin/v1/org/settings (ORG-20 — language, date era, branding). Defaults with row_version 0 when the tenant never saved one. */
export function useOrgSettings(client: ApiClient) {
  return useQuery({
    queryKey: settingsKey,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/org/settings");
      if (error) throw error;
      return data;
    },
  });
}

/** PUT /admin/v1/org/settings with If-Match = row_version (0 for the first save). */
export function useSaveOrgSettings(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { rowVersion: number; input: OrgSettingsInput }) => {
      const { data, error } = await client.PUT("/admin/v1/org/settings", {
        params: { header: { "If-Match": `"${v.rowVersion}"` } },
        body: v.input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: (d) => qc.setQueryData(settingsKey, d),
  });
}
