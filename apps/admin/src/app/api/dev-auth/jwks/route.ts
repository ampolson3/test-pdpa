import { devJwks, devLoginEnabled } from "@/lib/dev-auth";

/** Public key of the dev login (lib/dev-auth.ts) for the Go API's OIDC_JWKS_URL; 404 unless dev login is on. */
export function GET() {
  if (!devLoginEnabled()) return new Response(null, { status: 404 });
  return Response.json(devJwks());
}
