import { useInfiniteQuery } from "@tanstack/react-query";
import type { ApiClient, components } from "../client";

export type JobState = components["schemas"]["JobState"];

export interface JobsFilter {
  state?: JobState[];
  kind?: string;
}

/**
 * GET /admin/v1/platform/jobs — the tenant's background jobs, newest first, one cursor page at a
 * time (PLT-10). Refreshes every 10 s while the page is open so running/retrying jobs move on their own.
 */
export function useJobs(client: ApiClient, filter: JobsFilter, pageSize = 50) {
  return useInfiniteQuery({
    queryKey: ["platform", "jobs", filter, pageSize],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await client.GET("/admin/v1/platform/jobs", {
        params: {
          query: {
            limit: pageSize,
            cursor: pageParam,
            state: filter.state?.length ? filter.state : undefined,
            kind: filter.kind || undefined,
          },
        },
        // The contract declares `state` as style=form, explode=false: state=running,retryable
        querySerializer: { array: { style: "form", explode: false } },
      });
      if (error) throw error;
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    refetchInterval: 10_000,
  });
}
