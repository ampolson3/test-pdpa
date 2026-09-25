import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type ConsentPurpose = components["schemas"]["ConsentPurpose"];
export type PurposeContent = components["schemas"]["PurposeContent"];
export type ConsentText = components["schemas"]["ConsentText"];
export type ConsentPreference = components["schemas"]["ConsentPreference"];
export type ConsentCollectionPoint = components["schemas"]["ConsentCollectionPoint"];
export type CollectionPointInput = components["schemas"]["CollectionPointInput"];
export type ConsentChecklist = components["schemas"]["ConsentChecklist"];
export type ConsentSubjectSummary = components["schemas"]["ConsentSubjectSummary"];
export type ConsentSubjectProfile = components["schemas"]["ConsentSubjectProfile"];
export type ConsentVerifyResult = components["schemas"]["ConsentVerifyResult"];
export type ConsentDecisionInput = components["schemas"]["ConsentDecisionInput"];
export type ConsentRecordInput = components["schemas"]["ConsentRecordInput"];
export type ConsentResult = components["schemas"]["ConsentResult"];
export type SubjectIdentifier = components["schemas"]["SubjectIdentifier"];

const ifMatch = (v: number) => ({ "If-Match": `"${v}"` });

/** Purposes (CON-09/12) with their published versions. */
export function useConsentPurposes(client: ApiClient) {
  return useQuery({
    queryKey: ["consent", "purposes"],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/consent/purposes");
      if (error) throw error;
      return data.data;
    },
  });
}

export function useConsentPurpose(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: ["consent", "purposes", id],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/consent/purposes/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

/** Create a purpose (its first draft) and save later drafts; publishing goes through PLT-08 approval. */
export function usePurposeMutations(client: ApiClient) {
  const qc = useQueryClient();
  const refresh = () => qc.invalidateQueries({ queryKey: ["consent", "purposes"] });
  return {
    create: useMutation({
      mutationFn: async (v: { code: string; legal_entity_id: string; content: PurposeContent }) => {
        const { data, error } = await client.POST("/admin/v1/consent/purposes", { body: v });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    saveDraft: useMutation({
      mutationFn: async (v: { id: string; content: PurposeContent }) => {
        const { data, error } = await client.PUT("/admin/v1/consent/purposes/{id}/draft", {
          params: { path: { id: v.id } },
          body: v.content,
        });
        if (error) throw error;
        return data;
      },
      onSuccess: () => {
        refresh();
        qc.invalidateQueries({ queryKey: ["platform", "versions"] });
      },
    }),
    retire: useMutation({
      mutationFn: async (id: string) => {
        const { data, error } = await client.POST("/admin/v1/consent/purposes/{id}/retire", { params: { path: { id } } });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
  };
}

/** Collection points (CON-10): forms, their purposes, the publish checklist and the public key. */
export function useCollectionPoints(client: ApiClient) {
  return useQuery({
    queryKey: ["consent", "collection-points"],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/consent/collection-points");
      if (error) throw error;
      return data.data;
    },
  });
}

export function useCollectionPointMutations(client: ApiClient) {
  const qc = useQueryClient();
  const refresh = () => qc.invalidateQueries({ queryKey: ["consent", "collection-points"] });
  return {
    create: useMutation({
      mutationFn: async (v: CollectionPointInput & { code: string }) => {
        const { data, error } = await client.POST("/admin/v1/consent/collection-points", { body: v });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    update: useMutation({
      mutationFn: async (v: { cp: ConsentCollectionPoint; input: CollectionPointInput }) => {
        const { data, error } = await client.PUT("/admin/v1/consent/collection-points/{id}", {
          params: { path: { id: v.cp.id }, header: ifMatch(v.cp.row_version) },
          body: v.input,
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    publish: useMutation({
      mutationFn: async (v: { cp: ConsentCollectionPoint; checklist: ConsentChecklist }) => {
        const { data, error } = await client.POST("/admin/v1/consent/collection-points/{id}/publish", {
          params: { path: { id: v.cp.id }, header: ifMatch(v.cp.row_version) },
          body: v.checklist,
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    retire: useMutation({
      mutationFn: async (cp: ConsentCollectionPoint) => {
        const { data, error } = await client.POST("/admin/v1/consent/collection-points/{id}/retire", {
          params: { path: { id: cp.id }, header: ifMatch(cp.row_version) },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
  };
}

/** Data subjects (CON-15): recent ones, or an exact search by identifier (sent in the body, never in a URL). */
export function useConsentSubjects(client: ApiClient, search: SubjectIdentifier | null) {
  return useQuery({
    queryKey: ["consent", "subjects", search?.type ?? "", search?.value ?? ""],
    queryFn: async () => {
      if (search?.value) {
        const { data, error } = await client.POST("/admin/v1/consent/subjects/search", { body: { identifier: search } });
        if (error) throw error;
        return data.data;
      }
      const { data, error } = await client.GET("/admin/v1/consent/subjects");
      if (error) throw error;
      return data.data;
    },
  });
}

export function useConsentSubject(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: ["consent", "subject", id],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/consent/subjects/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

export function useConsentSubjectMutations(client: ApiClient) {
  const qc = useQueryClient();
  return {
    verify: useMutation({
      mutationFn: async (id: string) => {
        const { data, error } = await client.POST("/admin/v1/consent/subjects/{id}/verify", { params: { path: { id } } });
        if (error) throw error;
        return data;
      },
    }),
    /** Staff record consent / withdrawal on the subject's behalf (CON-13). */
    record: useMutation({
      mutationFn: async (input: ConsentRecordInput) => {
        const { data, error } = await client.POST("/admin/v1/consent/records", { body: input });
        if (error) throw error;
        return data;
      },
      onSuccess: () => qc.invalidateQueries({ queryKey: ["consent", "subject"] }),
    }),
  };
}

/** Where consent data is stored and how identifiers are protected (CON-17). */
export function useConsentSettings(client: ApiClient) {
  return useQuery({
    queryKey: ["consent", "settings"],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/consent/settings");
      if (error) throw error;
      return data;
    },
  });
}
