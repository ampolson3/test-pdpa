import { useQuery } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type RopaTemplateSet = components["schemas"]["RopaTemplateSet"];
export type RopaActivityTemplate = components["schemas"]["RopaActivityTemplate"];
export type RopaTemplateSuggestions = components["schemas"]["RopaTemplateSuggestions"];

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
