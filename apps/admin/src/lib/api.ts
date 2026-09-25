import { auth } from "@/auth";

const API_BASE = process.env.API_BASE_URL ?? "http://localhost:8080";

/**
 * Server-only fetch against the Go API with the signed-in user's access token attached — the same
 * thing `app/api/bff/[...path]/route.ts` does for client components, reused directly by Server
 * Components so they don't need a self-hop through their own BFF route.
 */
export async function apiFetch(path: string, init?: RequestInit): Promise<Response> {
  const session = await auth();
  if (!session?.accessToken) {
    throw new Response(null, { status: 401, statusText: "authn.required" });
  }

  return fetch(new URL(path, API_BASE), {
    ...init,
    headers: {
      ...init?.headers,
      Authorization: `Bearer ${session.accessToken}`,
      Accept: "application/json",
    },
    cache: "no-store",
  });
}
