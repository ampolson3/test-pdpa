import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type LegalEntity = components["schemas"]["LegalEntity"];
export type LegalEntityInput = components["schemas"]["LegalEntityInput"];
export type OrgUnit = components["schemas"]["OrgUnit"];
export type OrgUnitType = components["schemas"]["OrgUnitType"];

const entitiesKey = ["org", "legal-entities"] as const;

/** GET /admin/v1/org/legal-entities (ORG-01). */
export function useLegalEntities(client: ApiClient) {
  return useQuery({
    queryKey: entitiesKey,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/org/legal-entities");
      if (error) throw error;
      return data.data;
    },
  });
}

/** Create (no id) or update (id + row_version as If-Match) a legal entity. */
export function useSaveLegalEntity(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id?: string; rowVersion?: number; input: LegalEntityInput }) => {
      if (!v.id) {
        const { data, error } = await client.POST("/admin/v1/org/legal-entities", { body: v.input });
        if (error) throw error;
        return data;
      }
      const { data, error } = await client.PATCH("/admin/v1/org/legal-entities/{id}", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: v.input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: entitiesKey }),
  });
}

/** GET /admin/v1/org/units (ORG-04), in path order: parents before children. */
export function useOrgUnits(client: ApiClient, legalEntityId: string | undefined, includeClosed = false) {
  return useQuery({
    queryKey: ["org", "units", legalEntityId, includeClosed],
    enabled: !!legalEntityId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/org/units", { params: { query: { legal_entity_id: legalEntityId, include_closed: includeClosed } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** Create, rename, move and close units; each refreshes the tree. */
export function useOrgUnitMutations(client: ApiClient) {
  const qc = useQueryClient();
  const refresh = () => qc.invalidateQueries({ queryKey: ["org", "units"] });
  const ifMatch = (u: OrgUnit) => ({ "If-Match": `"${u.row_version}"` });
  return {
    create: useMutation({
      mutationFn: async (v: { legalEntityId: string; parentId?: string; code: string; nameTh: string; nameEn?: string; unitType: OrgUnitType }) => {
        const { data, error } = await client.POST("/admin/v1/org/units", {
          body: { legal_entity_id: v.legalEntityId, parent_id: v.parentId, code: v.code, name_th: v.nameTh, name_en: v.nameEn || undefined, unit_type: v.unitType },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    update: useMutation({
      mutationFn: async (v: { unit: OrgUnit; code: string; nameTh: string; nameEn?: string; unitType: OrgUnitType }) => {
        const { data, error } = await client.PATCH("/admin/v1/org/units/{id}", {
          params: { path: { id: v.unit.id }, header: ifMatch(v.unit) },
          body: { code: v.code, name_th: v.nameTh, name_en: v.nameEn || undefined, unit_type: v.unitType },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    move: useMutation({
      mutationFn: async (v: { unit: OrgUnit; parentId: string | null }) => {
        const { data, error } = await client.POST("/admin/v1/org/units/{id}/move", { params: { path: { id: v.unit.id }, header: ifMatch(v.unit) }, body: { parent_id: v.parentId } });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    close: useMutation({
      mutationFn: async (unit: OrgUnit) => {
        const { data, error } = await client.POST("/admin/v1/org/units/{id}/close", { params: { path: { id: unit.id }, header: ifMatch(unit) } });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
  };
}
