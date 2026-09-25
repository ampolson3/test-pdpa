import { useQuery } from "@tanstack/react-query";
import type { ApiClient } from "../client";

/** GET /admin/v1/me — SEQ-01, cached client-side for the same 60s the server caches grants. */
export function useMe(client: ApiClient) {
  return useQuery({
    queryKey: ["me"],
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/me");
      if (error) throw error;
      return data;
    },
    staleTime: 60_000,
  });
}
