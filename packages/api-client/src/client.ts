import createClient from "openapi-fetch";
import type { paths } from "./generated/schema";

/**
 * Typed fetch client for the PDPA API. baseUrl should point at this Next.js app's own BFF proxy
 * (`/api/bff`), never at the Go API directly — the BFF attaches the Keycloak access token
 * server-side so it never reaches the browser (docs/architecture/code-structure.md, Frontend).
 */
export function createApiClient(baseUrl: string) {
  return createClient<paths>({ baseUrl });
}

export type ApiClient = ReturnType<typeof createApiClient>;
export type { paths } from "./generated/schema";
export type { components } from "./generated/schema";
