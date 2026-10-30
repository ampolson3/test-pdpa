import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type RoleAssignmentRequest = components["schemas"]["IamRoleAssignmentRequest"];

/** GET /admin/v1/iam/role-assignment-requests — history (IAM-06). */
export function useRoleAssignmentRequests(client: ApiClient) {
  return useQuery({
    queryKey: ["iam", "role-assignment-requests"],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/iam/role-assignment-requests");
      if (error) throw error;
      return data.data;
    },
  });
}

/** POST /admin/v1/iam/role-assignment-requests — ask that a user be granted ORGADMIN; SEC still has to
 * decide and publish (the generic PLT-08 approvals inbox) before it takes effect. */
export function useRequestRoleAssignment(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (targetUserId: string) => {
      const { data, error } = await client.POST("/admin/v1/iam/role-assignment-requests", { body: { target_user_id: targetUserId } });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["iam", "role-assignment-requests"] }),
  });
}
