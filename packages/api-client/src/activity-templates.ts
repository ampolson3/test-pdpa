import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type RopaTemplateSet = components["schemas"]["RopaTemplateSet"];
export type RopaTemplateSetInput = components["schemas"]["RopaTemplateSetInput"];
export type RopaCloneTemplateSetInput = components["schemas"]["RopaCloneTemplateSetInput"];
export type RopaActivityTemplate = components["schemas"]["RopaActivityTemplate"];
export type RopaActivityTemplateInput = components["schemas"]["RopaActivityTemplateInput"];
export type RopaTemplateSuggestions = components["schemas"]["RopaTemplateSuggestions"];
export type RopaOutdatedTemplateActivity = components["schemas"]["RopaOutdatedTemplateActivity"];

const ifMatch = (v: number) => ({ "If-Match": `"${v}"` });
const ownSetsKey = ["ropa", "templates", "custom"] as const;

/** GET /admin/v1/ropa/templates/custom (RTG-08): this tenant's own custom template sets, any status —
 * the management list, distinct from useTemplateSets' own published-only browse list. */
export function useOwnTemplateSets(client: ApiClient) {
  return useQuery({
    queryKey: ownSetsKey,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/ropa/templates/custom", {});
      if (error) throw error;
      return data.data;
    },
  });
}

/** RTG-08: create/clone/publish/retire a tenant's own custom template set, and add/edit/remove its own
 * activities — "คัดลอก template กลางมาปรับ และกำหนด template มาตรฐานของกลุ่มบริษัท". */
export function useTemplateSetManagement(client: ApiClient) {
  const qc = useQueryClient();
  const refresh = () => {
    qc.invalidateQueries({ queryKey: ownSetsKey });
    qc.invalidateQueries({ queryKey: ["ropa", "templates"] });
  };
  return {
    create: useMutation({
      mutationFn: async (body: RopaTemplateSetInput) => {
        const { data, error } = await client.POST("/admin/v1/ropa/templates/custom", { body });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    clone: useMutation({
      mutationFn: async (v: { sourceSetId: string; body: RopaCloneTemplateSetInput }) => {
        const { data, error } = await client.POST("/admin/v1/ropa/templates/{id}/clone", {
          params: { path: { id: v.sourceSetId } },
          body: v.body,
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    publish: useMutation({
      mutationFn: async (set: RopaTemplateSet) => {
        const { data, error } = await client.POST("/admin/v1/ropa/templates/{id}/publish", {
          params: { path: { id: set.id }, header: ifMatch(set.row_version!) },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    retire: useMutation({
      mutationFn: async (set: RopaTemplateSet) => {
        const { data, error } = await client.POST("/admin/v1/ropa/templates/{id}/retire", {
          params: { path: { id: set.id }, header: ifMatch(set.row_version!) },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    addActivity: useMutation({
      mutationFn: async (v: { setId: string; body: RopaActivityTemplateInput }) => {
        const { data, error } = await client.POST("/admin/v1/ropa/templates/{id}/activities", {
          params: { path: { id: v.setId } },
          body: v.body,
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    updateActivity: useMutation({
      mutationFn: async (v: { activity: RopaActivityTemplate; body: RopaActivityTemplateInput }) => {
        const { data, error } = await client.PUT("/admin/v1/ropa/templates/activities/{activityTemplateId}", {
          params: { path: { activityTemplateId: v.activity.id }, header: ifMatch(v.activity.row_version!) },
          body: v.body,
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    deleteActivity: useMutation({
      mutationFn: async (activity: RopaActivityTemplate) => {
        const { error } = await client.DELETE("/admin/v1/ropa/templates/activities/{activityTemplateId}", {
          params: { path: { activityTemplateId: activity.id }, header: ifMatch(activity.row_version!) },
        });
        if (error) throw error;
      },
      onSuccess: refresh,
    }),
  };
}

/** GET /admin/v1/ropa/templates (RTG-01): published standard-activity template sets. */
export function useTemplateSets(client: ApiClient) {
  return useQuery({
    queryKey: ["ropa", "templates"],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/ropa/templates", {});
      if (error) throw error;
      return data.data;
    },
  });
}

/** GET /admin/v1/ropa/templates/{id}/activities, optionally filtered by job_category. */
export function useActivityTemplates(client: ApiClient, templateSetId: string | undefined, jobCategory?: string) {
  return useQuery({
    queryKey: ["ropa", "templates", templateSetId ?? "", "activities", jobCategory ?? ""],
    enabled: !!templateSetId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/ropa/templates/{id}/activities", {
        params: { path: { id: templateSetId! }, query: { job_category: jobCategory } },
      });
      if (error) throw error;
      return data.data;
    },
  });
}

/** GET /admin/v1/ropa/templates/activities/{activityTemplateId}: the full ม.39 defaults + rationale. */
export function useActivityTemplate(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: ["ropa", "templates", "activities", id ?? ""],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/ropa/templates/activities/{activityTemplateId}", {
        params: { path: { activityTemplateId: id! } },
      });
      if (error) throw error;
      return data;
    },
  });
}

/** GET /admin/v1/ropa/templates/activities/{activityTemplateId}/suggest (RTG-06): each item's own
 * recommendation, decorated with a rationale citing a real legal article — read-only. */
export function useTemplateSuggestions(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: ["ropa", "templates", "activities", id ?? "", "suggest"],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/ropa/templates/activities/{activityTemplateId}/suggest", {
        params: { path: { activityTemplateId: id! } },
      });
      if (error) throw error;
      return data;
    },
  });
}

/** GET /admin/v1/ropa/templates/activities/{activityTemplateId}/versions (RTG-10): every version ever
 * issued of this template's own (template_set_id, code), newest first — "หน้าประวัติ template". */
export function useTemplateVersions(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: ["ropa", "templates", "activities", id ?? "", "versions"],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/ropa/templates/activities/{activityTemplateId}/versions", {
        params: { path: { activityTemplateId: id! } },
      });
      if (error) throw error;
      return data.data;
    },
  });
}

/** GET /admin/v1/ropa/activities/outdated-templates (RTG-10): every activity whose own template has
 * since been reissued at a newer version — "รายการที่ต้องทบทวน". */
export function useOutdatedTemplateActivities(client: ApiClient) {
  return useQuery({
    queryKey: ["ropa", "activities", "outdated-templates"],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/ropa/activities/outdated-templates", {});
      if (error) throw error;
      return data.data;
    },
  });
}
