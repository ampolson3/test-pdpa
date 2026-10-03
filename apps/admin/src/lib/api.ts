import { headers } from "next/headers";
import { getLocale } from "next-intl/server";
import { auth } from "@/auth";

const API_BASE = process.env.API_BASE_URL ?? "http://localhost:8080";

/**
 * Server-only fetch against the Go API with the signed-in user's access token attached — the same
 * thing `app/api/bff/[...path]/route.ts` does for client components, reused directly by Server
 * Components so they don't need a self-hop through their own BFF route.
 *
 * Sends this page's resolved locale (next-intl's getLocale(), from the `/[locale]` segment) as
 * Accept-Language, not the browser's raw header — api/openapi/openapi.yaml's AcceptLanguage
 * parameter only declares the literal values "th"/"en" and the request validator (backend
 * internal/pkg/validate) rejects anything else with 400, while a real browser sends something
 * like "en-US,en;q=0.9" (PLT-03).
 */
export async function apiFetch(path: string, init?: RequestInit): Promise<Response> {
  const session = await auth();
  if (!session?.accessToken) {
    throw new Response(null, { status: 401, statusText: "authn.required" });
  }

  const locale = await getLocale();

  return fetch(new URL(path, API_BASE), {
    ...init,
    headers: {
      "Accept-Language": locale,
      ...(await browser()),
      ...init?.headers,
      Authorization: `Bearer ${session.accessToken}`,
      Accept: "application/json",
    },
    cache: "no-store",
  });
}

/**
 * The browser's address chain and user agent, passed on so the API's request audit and rate limiter see the
 * person rather than this server (decisions.md D-23). The API trusts X-Forwarded-For only from addresses in
 * its TRUSTED_PROXIES, so list the admin app there; left out, every request is attributed to this server.
 */
async function browser(): Promise<Record<string, string>> {
  const h = await headers();
  const out: Record<string, string> = {};
  const xff = h.get("x-forwarded-for");
  if (xff) out["X-Forwarded-For"] = xff;
  const ua = h.get("user-agent");
  if (ua) out["User-Agent"] = ua;
  return out;
}
