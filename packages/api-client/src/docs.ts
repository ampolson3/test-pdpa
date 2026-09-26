import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type DocumentTypesInfo = components["schemas"]["DocumentTypesInfo"];
export type DocumentSummary = components["schemas"]["DocumentSummary"];
export type ComposedDocument = components["schemas"]["Document"];
export type DocumentDraft = components["schemas"]["DocumentDraft"];
export type DocumentContent = components["schemas"]["DocumentContent"];
export type PublishedDocumentVersion = components["schemas"]["PublishedDocumentVersion"];
export type DocumentComparison = components["schemas"]["DocumentComparison"];
export type DocumentClause = components["schemas"]["DocumentClause"];
export type DocumentClauseDetail = components["schemas"]["DocumentClauseDetail"];
export type DocumentClauseInput = components["schemas"]["DocumentClauseInput"];
export type DocumentTemplate = components["schemas"]["DocumentTemplate"];
export type DocType = DocumentSummary["doc_type"];

const ifMatch = (v: number) => ({ "If-Match": `"${v}"` });
const docKey = (id: string) => ["docs", "document", id];

/** Document types the caller can read, with what they may do, and the merge field catalog (PLT-16). */
export function useDocumentTypes(client: ApiClient) {
  return useQuery({
    queryKey: ["docs", "types"],
    staleTime: 5 * 60_000,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/documents/types");
      if (error) throw error;
      return data;
    },
  });
}

export function useDocuments(client: ApiClient, filter: { doc_type?: DocType; q?: string }) {
  return useInfiniteQuery({
    queryKey: ["docs", "documents", filter],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/platform/documents", {
        params: { query: { doc_type: filter.doc_type, q: filter.q || undefined, cursor: pageParam, limit: 50 } },
      });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

export function useComposedDocument(client: ApiClient, id: string) {
  return useQuery({
    queryKey: docKey(id),
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/documents/{id}", { params: { path: { id } } });
      if (error) throw error;
      return data;
    },
  });
}

/** Published versions and their rendered files; polled while a rendering is pending. */
export function usePublishedDocumentVersions(client: ApiClient, id: string) {
  return useQuery({
    queryKey: [...docKey(id), "published"],
    refetchInterval: (q) => (q.state.data?.some((v) => v.render_status === "pending") ? 2000 : false),
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/documents/{id}/published", { params: { path: { id } } });
      if (error) throw error;
      return data.data;
    },
  });
}

export function useDocumentComparison(client: ApiClient, id: string, from?: string, to?: string, language: "th" | "en" = "th") {
  return useQuery({
    queryKey: [...docKey(id), "compare", from, to, language],
    enabled: !!from && !!to,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/documents/{id}/compare", {
        params: { path: { id }, query: { from: from!, to: to!, language } },
      });
      if (error) throw error;
      return data;
    },
  });
}

/** Link that downloads a version (default: the newest) as PDF or Word through the BFF. */
export function documentExportHref(bffBaseUrl: string, id: string, language: "th" | "en", format: "pdf" | "docx", versionId?: string): string {
  const q = new URLSearchParams({ language, format });
  if (versionId) q.set("version_id", versionId);
  return `${bffBaseUrl}/admin/v1/platform/documents/${id}/export?${q}`;
}

export function useDocumentMutations(client: ApiClient) {
  const qc = useQueryClient();
  const refresh = (id?: string) => {
    void qc.invalidateQueries({ queryKey: ["docs", "documents"] });
    if (id) void qc.invalidateQueries({ queryKey: docKey(id) });
    // Saving replaces the open PLT-08 version (new row_version), so the version bar must refetch it.
    void qc.invalidateQueries({ queryKey: ["platform", "versions"] });
    void qc.invalidateQueries({ queryKey: ["platform", "record-versions"] });
  };
  return {
    create: useMutation({
      mutationFn: async (body: { doc_type: DocType; title: string; legal_entity_id?: string; template_id?: string }) => {
        const { data, error } = await client.POST("/admin/v1/platform/documents", { body });
        if (error) throw error;
        return data;
      },
      onSuccess: (d) => refresh(d.id),
    }),
    save: useMutation({
      mutationFn: async (v: { id: string; rowVersion: number; draft: DocumentDraft }) => {
        const { data, error } = await client.PUT("/admin/v1/platform/documents/{id}/draft", {
          params: { path: { id: v.id }, header: ifMatch(v.rowVersion) },
          body: v.draft,
        });
        if (error) throw error;
        return data;
      },
      onSuccess: (d) => {
        qc.setQueryData(docKey(d.id), d);
        refresh(d.id);
      },
    }),
  };
}

export function useClauses(client: ApiClient, filter: { category?: string; applies_to?: DocType; published_only?: boolean } = {}) {
  return useQuery({
    queryKey: ["docs", "clauses", filter],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/document-clauses", { params: { query: filter } });
      if (error) throw error;
      return data.data;
    },
  });
}

export function useClause(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: ["docs", "clause", id],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/document-clauses/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

export function useClauseMutations(client: ApiClient) {
  const qc = useQueryClient();
  const refresh = () => void qc.invalidateQueries({ queryKey: ["docs"] });
  return {
    create: useMutation({
      mutationFn: async (body: DocumentClauseInput) => {
        const { data, error } = await client.POST("/admin/v1/platform/document-clauses", { body });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    update: useMutation({
      mutationFn: async (v: { id: string; rowVersion: number; body: DocumentClauseInput }) => {
        const { data, error } = await client.PUT("/admin/v1/platform/document-clauses/{id}", {
          params: { path: { id: v.id }, header: ifMatch(v.rowVersion) },
          body: v.body,
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    publish: useMutation({
      mutationFn: async (c: DocumentClause) => {
        const { data, error } = await client.POST("/admin/v1/platform/document-clauses/{id}/publish", { params: { path: { id: c.id }, header: ifMatch(c.row_version) } });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    retire: useMutation({
      mutationFn: async (c: DocumentClause) => {
        const { data, error } = await client.POST("/admin/v1/platform/document-clauses/{id}/retire", { params: { path: { id: c.id }, header: ifMatch(c.row_version) } });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
  };
}

export function useDocumentTemplates(client: ApiClient, filter: { doc_type?: DocType; published_only?: boolean } = {}) {
  return useQuery({
    queryKey: ["docs", "templates", filter],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/platform/document-templates", { params: { query: filter } });
      if (error) throw error;
      return data.data;
    },
  });
}

export function useDocumentTemplateMutations(client: ApiClient) {
  const qc = useQueryClient();
  const refresh = () => void qc.invalidateQueries({ queryKey: ["docs", "templates"] });
  return {
    create: useMutation({
      mutationFn: async (body: { doc_type: DocType; code: string; name: string; content: DocumentContent }) => {
        const { data, error } = await client.POST("/admin/v1/platform/document-templates", { body });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    update: useMutation({
      mutationFn: async (v: { id: string; rowVersion: number; name: string; content: DocumentContent }) => {
        const { data, error } = await client.PUT("/admin/v1/platform/document-templates/{id}", {
          params: { path: { id: v.id }, header: ifMatch(v.rowVersion) },
          body: { name: v.name, content: v.content },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    publish: useMutation({
      mutationFn: async (t: DocumentTemplate) => {
        const { data, error } = await client.POST("/admin/v1/platform/document-templates/{id}/publish", { params: { path: { id: t.id }, header: ifMatch(t.row_version) } });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
  };
}
