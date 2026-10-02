import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type ProcessingActivity = components["schemas"]["ProcessingActivity"];
export type ProcessingActivityInput = components["schemas"]["ProcessingActivityInput"];
export type ActivityRole = components["schemas"]["ActivityRole"];
export type ActivityStatus = components["schemas"]["ActivityStatus"];
export type ActivityPurpose = components["schemas"]["ActivityPurpose"];
export type ActivityPurposeInput = components["schemas"]["ActivityPurposeInput"];
export type ActivityData = components["schemas"]["ActivityData"];
export type ActivityDataInput = components["schemas"]["ActivityDataInput"];
export type ActivityDataSource = components["schemas"]["ActivityDataSource"];
export type ActivityVolumeBand = components["schemas"]["ActivityVolumeBand"];
export type ActivityDisposalMethod = components["schemas"]["ActivityDisposalMethod"];
export type RetentionRule = components["schemas"]["RetentionRule"];
export type RetentionRuleInput = components["schemas"]["RetentionRuleInput"];
export type ActivityRecipient = components["schemas"]["ActivityRecipient"];
export type ActivityRecipientInput = components["schemas"]["ActivityRecipientInput"];
export type ActivityRecipientRole = components["schemas"]["ActivityRecipientRole"];
export type ActivityTransfer = components["schemas"]["ActivityTransfer"];
export type ActivityTransferInput = components["schemas"]["ActivityTransferInput"];
export type ActivityTransferBasis = components["schemas"]["ActivityTransferBasis"];
export type SecurityControl = components["schemas"]["SecurityControl"];
export type SecurityControlCategory = components["schemas"]["SecurityControlCategory"];
export type ActivityControl = components["schemas"]["ActivityControl"];
export type ActivityControlInput = components["schemas"]["ActivityControlInput"];
export type ActivityRejection = components["schemas"]["ActivityRejection"];
export type ProcessingActivityFromTemplateInput = components["schemas"]["ProcessingActivityFromTemplateInput"];

const ifMatch = (v: number) => ({ "If-Match": `"${v}"` });
const activitiesKey = ["ropa", "activities"] as const;
const activityKey = (id: string) => [...activitiesKey, id] as const;

/** The processor RoPA CSV export URL through the BFF (ROPA-04) — a plain link, the browser downloads it. */
export function processorActivitiesExportHref(bffBaseUrl: string): string {
  return `${bffBaseUrl}/admin/v1/ropa/activities/processor-export`;
}

/** GET /admin/v1/ropa/activities (ROPA-03), newest first. */
export function useActivities(client: ApiClient, filter: { org_unit_id?: string; status?: ActivityStatus; q?: string } = {}) {
  return useInfiniteQuery({
    queryKey: [...activitiesKey, filter],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/ropa/activities", {
        params: { query: { org_unit_id: filter.org_unit_id, status: filter.status, q: filter.q || undefined, cursor: pageParam, limit: 50 } },
      });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

/** GET /admin/v1/ropa/activities/{id} — includes the computed completeness/missing_items. */
export function useProcessingActivity(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: activityKey(id ?? ""),
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/ropa/activities/{id}", { params: { path: { id: id! } } });
      if (error) throw error;
      return data;
    },
  });
}

export function useActivityPurposes(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...activityKey(id ?? ""), "purposes"],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/ropa/activities/{id}/purposes", { params: { path: { id: id! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

export function useActivityData(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...activityKey(id ?? ""), "data"],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/ropa/activities/{id}/data", { params: { path: { id: id! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

export function useRetentionRules(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...activityKey(id ?? ""), "retention-rules"],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/ropa/activities/{id}/retention-rules", { params: { path: { id: id! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

export function useActivityRecipients(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...activityKey(id ?? ""), "recipients"],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/ropa/activities/{id}/recipients", { params: { path: { id: id! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

export function useActivityTransfers(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...activityKey(id ?? ""), "transfers"],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/ropa/activities/{id}/transfers", { params: { path: { id: id! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** GET /admin/v1/ropa/security-controls (ROPA-09) — the ม.37(1) catalog, global + this tenant's own. */
export function useSecurityControls(client: ApiClient) {
  return useQuery({
    queryKey: ["ropa", "security-controls"],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/ropa/security-controls", {});
      if (error) throw error;
      return data.data;
    },
  });
}

export function useActivityControls(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...activityKey(id ?? ""), "controls"],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/ropa/activities/{id}/controls", { params: { path: { id: id! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** GET /admin/v1/ropa/activities/{id}/rejections (ROPA-10) — DSAR rejections logged here automatically from
 *  the dsar.rejected event; read-only, this module never writes these rows itself. */
export function useActivityRejections(client: ApiClient, id: string | undefined) {
  return useQuery({
    queryKey: [...activityKey(id ?? ""), "rejections"],
    enabled: !!id,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/ropa/activities/{id}/rejections", { params: { path: { id: id! } } });
      if (error) throw error;
      return data.data;
    },
  });
}

/** Every write for one activity and its ม.39 child rows, grouped like the breach module's incident mutations. */
export function useActivityMutations(client: ApiClient, id?: string) {
  const qc = useQueryClient();
  const refresh = () => {
    qc.invalidateQueries({ queryKey: activitiesKey });
    if (id) qc.invalidateQueries({ queryKey: activityKey(id) });
  };
  return {
    save: useMutation({
      mutationFn: async (v: { activity?: ProcessingActivity; input: ProcessingActivityInput }) => {
        if (!v.activity) {
          const { data, error } = await client.POST("/admin/v1/ropa/activities", { body: v.input });
          if (error) throw error;
          return data;
        }
        const { data, error } = await client.PATCH("/admin/v1/ropa/activities/{id}", {
          params: { path: { id: v.activity.id }, header: ifMatch(v.activity.row_version) },
          body: v.input,
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    /** ROPA-05: create an activity from one of RTG-01's standard templates — purposes, data, retention and
     * security controls arrive already filled in from the template's own ม.39 defaults. */
    createFromTemplate: useMutation({
      mutationFn: async (body: ProcessingActivityFromTemplateInput) => {
        const { data, error } = await client.POST("/admin/v1/ropa/activities/from-template", { body });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    submit: useMutation({
      mutationFn: async (activity: ProcessingActivity) => {
        const { data, error } = await client.POST("/admin/v1/ropa/activities/{id}/submit", {
          params: { path: { id: activity.id }, header: ifMatch(activity.row_version) },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    addPurpose: useMutation({
      mutationFn: async (body: ActivityPurposeInput) => {
        const { data, error } = await client.POST("/admin/v1/ropa/activities/{id}/purposes", { params: { path: { id: id! } }, body });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    deletePurpose: useMutation({
      mutationFn: async (purposeId: string) => {
        const { error } = await client.DELETE("/admin/v1/ropa/activities/{id}/purposes/{purposeId}", { params: { path: { id: id!, purposeId } } });
        if (error) throw error;
      },
      onSuccess: refresh,
    }),
    addData: useMutation({
      mutationFn: async (body: ActivityDataInput) => {
        const { data, error } = await client.POST("/admin/v1/ropa/activities/{id}/data", { params: { path: { id: id! } }, body });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    deleteData: useMutation({
      mutationFn: async (dataId: string) => {
        const { error } = await client.DELETE("/admin/v1/ropa/activities/{id}/data/{dataId}", { params: { path: { id: id!, dataId } } });
        if (error) throw error;
      },
      onSuccess: refresh,
    }),
    addRetentionRule: useMutation({
      mutationFn: async (body: RetentionRuleInput) => {
        const { data, error } = await client.POST("/admin/v1/ropa/activities/{id}/retention-rules", { params: { path: { id: id! } }, body });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    deleteRetentionRule: useMutation({
      mutationFn: async (ruleId: string) => {
        const { error } = await client.DELETE("/admin/v1/ropa/activities/{id}/retention-rules/{ruleId}", { params: { path: { id: id!, ruleId } } });
        if (error) throw error;
      },
      onSuccess: refresh,
    }),
    addRecipient: useMutation({
      mutationFn: async (body: ActivityRecipientInput) => {
        const { data, error } = await client.POST("/admin/v1/ropa/activities/{id}/recipients", { params: { path: { id: id! } }, body });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    deleteRecipient: useMutation({
      mutationFn: async (recipientId: string) => {
        const { error } = await client.DELETE("/admin/v1/ropa/activities/{id}/recipients/{recipientId}", { params: { path: { id: id!, recipientId } } });
        if (error) throw error;
      },
      onSuccess: refresh,
    }),
    addTransfer: useMutation({
      mutationFn: async (body: ActivityTransferInput) => {
        const { data, error } = await client.POST("/admin/v1/ropa/activities/{id}/transfers", { params: { path: { id: id! } }, body });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    deleteTransfer: useMutation({
      mutationFn: async (transferId: string) => {
        const { error } = await client.DELETE("/admin/v1/ropa/activities/{id}/transfers/{transferId}", { params: { path: { id: id!, transferId } } });
        if (error) throw error;
      },
      onSuccess: refresh,
    }),
    addControl: useMutation({
      mutationFn: async (body: ActivityControlInput) => {
        const { data, error } = await client.POST("/admin/v1/ropa/activities/{id}/controls", { params: { path: { id: id! } }, body });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    deleteControl: useMutation({
      mutationFn: async (controlId: string) => {
        const { error } = await client.DELETE("/admin/v1/ropa/activities/{id}/controls/{controlId}", { params: { path: { id: id!, controlId } } });
        if (error) throw error;
      },
      onSuccess: refresh,
    }),
  };
}
