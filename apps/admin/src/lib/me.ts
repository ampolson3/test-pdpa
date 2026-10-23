import type { components } from "@pdpa/api-client";
import { apiFetch } from "@/lib/api";

export type Me = components["schemas"]["Me"];

/** The signed-in user (GET /admin/v1/me), or null when there is no usable session. */
export async function loadMe(): Promise<Me | null> {
  try {
    const res = await apiFetch("/admin/v1/me");
    return res.ok ? ((await res.json()) as Me) : null;
  } catch {
    return null;
  }
}
