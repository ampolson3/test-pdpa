import NextAuth, { type NextAuthConfig } from "next-auth";
import Credentials from "next-auth/providers/credentials";
import Keycloak from "next-auth/providers/keycloak";
import { devLoginEnabled, devUser, signDevAccessToken } from "@/lib/dev-auth";

/**
 * Auth.js + Keycloak, OIDC code + PKCE (docs/architecture/code-structure.md, Frontend). The
 * session lives server-side; the access token is kept in the encrypted JWT session cookie and
 * only ever read on the server (app/api/bff route), never sent to the browser as JSON —
 * `api/openapi/openapi.yaml`'s adminJwt scheme: "Browsers never hold it."
 *
 * Requires AUTH_SECRET, AUTH_KEYCLOAK_ISSUER, AUTH_KEYCLOAK_ID, AUTH_KEYCLOAK_SECRET
 * (see .env.example) — realm/client setup itself is pending decisions.md Q-18's PoC (T13).
 * Locally without Keycloak, AUTH_DEV_LOGIN=true signs in as the `make dev-seed` demo user instead
 * (lib/dev-auth.ts); the Keycloak provider is only registered when its issuer is configured.
 */
const providers: NextAuthConfig["providers"] = [];
if (process.env.AUTH_KEYCLOAK_ISSUER) providers.push(Keycloak);
if (devLoginEnabled()) {
  providers.push(
    Credentials({
      id: "dev",
      name: "Dev login",
      credentials: {},
      authorize: () => {
        const u = devUser();
        return u ? { id: u.userId, name: "dev" } : null;
      },
    }),
  );
}

/** The provider the sign-in button uses. */
export const signInProvider = devLoginEnabled() ? "dev" : "keycloak";

export const { handlers, auth, signIn, signOut } = NextAuth({
  providers,
  session: { strategy: "jwt" },
  callbacks: {
    async jwt({ token, account }) {
      if (account?.provider === "dev" || token.devLogin) {
        // Re-sign the short-lived dev access token whenever it is about to expire.
        const u = devUser();
        const exp = token.devTokenExp as number | undefined;
        if (u && devLoginEnabled() && (!exp || exp - 60 < Date.now() / 1000)) {
          const { token: at, expiresAt } = signDevAccessToken(u.userId, u.tenantId);
          Object.assign(token, { accessToken: at, devTokenExp: expiresAt, devLogin: true, tenantId: u.tenantId });
        }
        return token;
      }
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
