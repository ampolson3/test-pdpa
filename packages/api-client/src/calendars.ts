import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ApiClient, components } from "./client";

export type BusinessCalendar = components["schemas"]["BusinessCalendar"];
export type BusinessCalendarInput = components["schemas"]["BusinessCalendarInput"];
export type Holiday = components["schemas"]["Holiday"];

const calendarsKey = ["org", "calendars"] as const;

/** GET /admin/v1/org/calendars (ORG-20), the default first. */
export function useCalendars(client: ApiClient) {
  return useQuery({
    queryKey: calendarsKey,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/org/calendars");
      if (error) throw error;
      return data.data;
    },
  });
}

/** Create a calendar, or save one (PATCH with If-Match = row_version). */
export function useSaveCalendar(client: ApiClient) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { id?: string; rowVersion?: number; input: BusinessCalendarInput }) => {
      if (!v.id) {
        const { data, error } = await client.POST("/admin/v1/org/calendars", { body: v.input });
        if (error) throw error;
        return data;
      }
      const { data, error } = await client.PATCH("/admin/v1/org/calendars/{id}", {
        params: { path: { id: v.id }, header: { "If-Match": `"${v.rowVersion}"` } },
        body: v.input,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: calendarsKey }),
  });
}

/** GET /admin/v1/org/calendars/{id}/holidays?year= (Gregorian year). */
export function useHolidays(client: ApiClient, calendarId: string | undefined, year: number) {
  return useQuery({
    queryKey: ["org", "calendars", calendarId, "holidays", year],
    enabled: !!calendarId,
    queryFn: async () => {
      const { data, error } = await client.GET("/admin/v1/org/calendars/{id}/holidays", {
        params: { path: { id: calendarId! }, query: { year } },
      });
      if (error) throw error;
      return data.data;
    },
  });
}

/** Add or rename (PUT) and remove (DELETE) a holiday; refreshes the calendar's holiday lists. */
export function useHolidayMutations(client: ApiClient, calendarId: string | undefined) {
  const qc = useQueryClient();
  const refresh = () => qc.invalidateQueries({ queryKey: ["org", "calendars", calendarId, "holidays"] });
  return {
    put: useMutation({
      mutationFn: async (v: { date: string; name: string }) => {
        const { data, error } = await client.PUT("/admin/v1/org/calendars/{id}/holidays/{date}", {
          params: { path: { id: calendarId!, date: v.date } },
          body: { name: v.name },
        });
        if (error) throw error;
        return data;
      },
      onSuccess: refresh,
    }),
    remove: useMutation({
      mutationFn: async (date: string) => {
        const { error } = await client.DELETE("/admin/v1/org/calendars/{id}/holidays/{date}", {
          params: { path: { id: calendarId!, date } },
        });
        if (error) throw error;
      },
      onSuccess: refresh,
    }),
    refresh,
  };
}
