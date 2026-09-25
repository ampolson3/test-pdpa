import NextAuth from "next-auth";
import Keycloak from "next-auth/providers/keycloak";

/**
 * Auth.js + Keycloak, OIDC code + PKCE (docs/architecture/code-structure.md, Frontend). The
 * session lives server-side; the access token is kept in the encrypted JWT session cookie and
 * only ever read on the server (app/api/bff route), never sent to the browser as JSON —
 * `api/openapi/openapi.yaml`'s adminJwt scheme: "Browsers never hold it."
 *
 * Requires AUTH_SECRET, AUTH_KEYCLOAK_ISSUER, AUTH_KEYCLOAK_ID, AUTH_KEYCLOAK_SECRET
 * (see .env.example) — realm/client setup itself is pending decisions.md Q-18's PoC (T13).
 */
export const { handlers, auth, signIn, signOut } = NextAuth({
  providers: [Keycloak],
  session: { strategy: "jwt" },
  callbacks: {
    async jwt({ token, account }) {
      if (account) {
        token.accessToken = account.access_token;
        token.tenantId = account.id_token ? decodeTenantId(account.id_token) : undefined;
      }
      return token;
    },
    async session({ session, token }) {
      session.accessToken = token.accessToken as string | undefined;
      session.tenantId = token.tenantId as string | undefined;
      return session;
    },
  },
});

/** Best-effort read of the `tid` claim (decisions.md Q-18) from the ID token, for UI display only
 * — the Go API is the source of truth for which tenant a request is scoped to. */
function decodeTenantId(idToken: string): string | undefined {
  try {
    const payload = idToken.split(".")[1];
    const json = JSON.parse(Buffer.from(payload, "base64url").toString("utf8"));
    return typeof json.tid === "string" ? json.tid : undefined;
  } catch {
    return undefined;
  }
}
