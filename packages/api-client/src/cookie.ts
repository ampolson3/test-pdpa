import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type CookieDomain = components["schemas"]["CookieDomain"];
export type CookieCategory = components["schemas"]["CookieCategory"];
export type CookieBannerConfig = components["schemas"]["CookieBannerConfig"];
export type CookieBannerTexts = components["schemas"]["CookieBannerTexts"];
export type CookieBannerConfigInput = components["schemas"]["CookieBannerConfigInput"];
export type CookieCookie = components["schemas"]["CookieCookie"];
export type CookieCreateCookieInput = components["schemas"]["CookieCreateCookieInput"];
export type CookieScriptRule = components["schemas"]["CookieScriptRule"];
export type CookieCreateScriptRuleInput = components["schemas"]["CookieCreateScriptRuleInput"];

const domainsKey = ["cookie", "domains"] as const;
const categoriesKey = ["cookie", "categories"] as const;

/** GET /admin/v1/cookie/domains (CON-01). */
export function useCookieDomains(client: ApiClient) {
  return useQuery({
    queryKey: domainsKey,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/cookie/domains");
      if (error) throw error;
      return data.data;
    },
  });
}

/** Register a new domain for the cookie banner. */
export function useRegisterCookieDomain(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { domain: string; legalEntityId: string }) => {
      const { data, error } = await client.POST("/admin/v1/cookie/domains", {
        body: { domain: v.domain, legal_entity_id: v.legalEntityId },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: domainsKey }),
  });
}

/** Confirm ownership of a domain and mark it active. */
export function useVerifyCookieDomain(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { domain: CookieDomain }) => {
      const { data, error } = await client.POST("/admin/v1/cookie/domains/{id}/verify", {
        params: { path: { id: v.domain.id }, header: { "If-Match": `"${v.domain.row_version}"` } },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: domainsKey }),
  });
}

/** GET /admin/v1/cookie/categories (CON-01). */
export function useCookieCategories(client: ApiClient) {
  return useQuery({
    queryKey: categoriesKey,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/cookie/categories");
      if (error) throw error;
      return data.data;
    },
  });
}

export function useCookieCategoryMutations(client: ApiClient) {
  const qc = useQueryClient();
  const refresh = () => qc.invalidateQueries({ queryKey: categoriesKey });
  return {
    create: useMutation({
      mutationFn: async (v: { code: string; nameTh: string; nameEn?: string; descriptionTh?: string; descriptionEn?: string; isRequired: boolean; displayOrder: number }) => {
        const { data, error } = await client.POST("/admin/v1/cookie/categories", {
          body: {
            code: v.code,
            name_th: v.nameTh,
            name_en: v.nameEn || undefined,
            description_th: v.descriptionTh || undefined,
            description_en: v.descriptionEn || undefined,
            is_required: v.isRequired,
            display_order: v.displayOrder,
          },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    remove: useMutation({
      mutationFn: async (id: string) => {
        const { error } = await client.DELETE("/admin/v1/cookie/categories/{id}", { params: { path: { id } } });
        if (error) throw error;
      },
      onSuccess: refresh,
    }),
  };
}

/** GET /admin/v1/cookie/domains/{id}/banner-configs — every version, newest first. */
export function useCookieBannerConfigs(client: ApiClient, domainId: string | undefined) {
  return useQuery({
    queryKey: ["cookie", "banner-configs", domainId],
    enabled: !!domainId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/cookie/domains/{id}/banner-configs", { params: { path: { id: domainId! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

export function useCookieBannerConfigMutations(client: ApiClient, domainId: string | undefined) {
  const qc = useQueryClient();
  const refresh = () => qc.invalidateQueries({ queryKey: ["cookie", "banner-configs", domainId] });
  return {
    create: useMutation({
      mutationFn: async (input: CookieBannerConfigInput) => {
        const { data, error } = await client.POST("/admin/v1/cookie/domains/{id}/banner-configs", {
          params: { path: { id: domainId! } },
          body: input,
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    update: useMutation({
      mutationFn: async (v: { config: CookieBannerConfig; input: CookieBannerConfigInput }) => {
        const { data, error } = await client.PUT("/admin/v1/cookie/banner-configs/{id}", {
          params: { path: { id: v.config.id }, header: { "If-Match": `"${v.config.row_version}"` } },
          body: v.input,
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    publish: useMutation({
      mutationFn: async (config: CookieBannerConfig) => {
        const { data, error } = await client.POST("/admin/v1/cookie/banner-configs/{id}/publish", {
          params: { path: { id: config.id }, header: { "If-Match": `"${config.row_version}"` } },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
  };
}

/** GET /admin/v1/cookie/domains/{id}/cookies — the catalogue of cookies under a domain (CON-02). */
export function useCookieCookies(client: ApiClient, domainId: string | undefined) {
  return useQuery({
    queryKey: ["cookie", "cookies", domainId],
    enabled: !!domainId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/cookie/domains/{id}/cookies", { params: { path: { id: domainId! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

export function useCookieCookieMutations(client: ApiClient, domainId: string | undefined) {
  const qc = useQueryClient();
  const refresh = () => qc.invalidateQueries({ queryKey: ["cookie", "cookies", domainId] });
  return {
    create: useMutation({
      mutationFn: async (input: CookieCreateCookieInput) => {
        const { data, error } = await client.POST("/admin/v1/cookie/domains/{id}/cookies", {
          params: { path: { id: domainId! } },
          body: input,
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    remove: useMutation({
      mutationFn: async (id: string) => {
        const { error } = await client.DELETE("/admin/v1/cookie/cookies/{id}", { params: { path: { id } } });
        if (error) throw error;
      },
      onSuccess: refresh,
    }),
  };
}

/** GET /admin/v1/cookie/domains/{id}/script-rules — which scripts belong to which category (CON-02). */
export function useCookieScriptRules(client: ApiClient, domainId: string | undefined) {
  return useQuery({
    queryKey: ["cookie", "script-rules", domainId],
    enabled: !!domainId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/cookie/domains/{id}/script-rules", { params: { path: { id: domainId! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

export function useCookieScriptRuleMutations(client: ApiClient, domainId: string | undefined) {
  const qc = useQueryClient();
  const refresh = () => qc.invalidateQueries({ queryKey: ["cookie", "script-rules", domainId] });
  return {
    create: useMutation({
      mutationFn: async (input: CookieCreateScriptRuleInput) => {
        const { data, error } = await client.POST("/admin/v1/cookie/domains/{id}/script-rules", {
          params: { path: { id: domainId! } },
          body: input,
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    remove: useMutation({
      mutationFn: async (id: string) => {
        const { error } = await client.DELETE("/admin/v1/cookie/script-rules/{id}", { params: { path: { id } } });
        if (error) throw error;
      },
      onSuccess: refresh,
    }),
  };
}
