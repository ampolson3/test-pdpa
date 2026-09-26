import { createHash, createPrivateKey, createPublicKey, generateKeyPairSync, sign, type KeyObject } from "node:crypto";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";

/**
 * Dev login: sign in without Keycloak when running locally (AUTH_DEV_LOGIN=true, never in production
 * builds). The admin app signs an RS256 access token for the demo user seeded by `make dev-seed`
 * (AUTH_DEV_USER_ID in AUTH_DEV_TENANT_ID) and publishes its public key at /api/dev-auth/jwks; the Go API
 * trusts it only when pointed there (OIDC_ISSUER / OIDC_JWKS_URL, see .env.example). A production API keeps
 * Keycloak's issuer and JWKS, so these tokens are refused there even if the flag were set by mistake.
 */
export function devLoginEnabled(): boolean {
  return process.env.AUTH_DEV_LOGIN === "true" && process.env.NODE_ENV !== "production";
}

export const devIssuer = () => process.env.AUTH_DEV_ISSUER ?? "http://localhost:3000/dev-auth";

/** The dev user's ids, as .env names them. */
export function devUser(): { userId: string; tenantId: string } | null {
  const userId = process.env.AUTH_DEV_USER_ID;
  const tenantId = process.env.AUTH_DEV_TENANT_ID;
  return userId && tenantId ? { userId, tenantId } : null;
}

// One key per checkout, kept in .dev-auth/ (git-ignored) so API restarts and hot reloads agree on it.
let cached: { key: KeyObject; kid: string } | undefined;
function signingKey(): { key: KeyObject; kid: string } {
  if (cached) return cached;
  const file = path.join(process.cwd(), ".dev-auth", "signing-key.pem");
  let pem: string;
  try {
    pem = readFileSync(file, "utf8");
  } catch {
    pem = generateKeyPairSync("rsa", { modulusLength: 2048 }).privateKey.export({ type: "pkcs8", format: "pem" }).toString();
    mkdirSync(path.dirname(file), { recursive: true });
    writeFileSync(file, pem, { mode: 0o600 });
  }
  const key = createPrivateKey(pem);
  const n = (createPublicKey(key).export({ format: "jwk" }) as { n: string }).n;
  cached = { key, kid: "dev-" + createHash("sha256").update(n).digest("base64url").slice(0, 16) };
  return cached;
}

/** The JSON Web Key Set the Go API fetches (OIDC_JWKS_URL). */
export function devJwks() {
  const { key, kid } = signingKey();
  const jwk = createPublicKey(key).export({ format: "jwk" }) as { kty: string; n: string; e: string };
  return { keys: [{ kid, kty: jwk.kty, n: jwk.n, e: jwk.e, use: "sig", alg: "RS256" }] };
}

export const DEV_TOKEN_SECONDS = 60 * 60;

/** An access token for the dev user, shaped like Keycloak's (sub = iam.users.id, tid = tenant). */
export function signDevAccessToken(userId: string, tenantId: string): { token: string; expiresAt: number } {
  const { key, kid } = signingKey();
  const now = Math.floor(Date.now() / 1000);
  const b64 = (v: object) => Buffer.from(JSON.stringify(v)).toString("base64url");
  const input = `${b64({ alg: "RS256", typ: "JWT", kid })}.${b64({ iss: devIssuer(), sub: userId, tid: tenantId, iat: now, exp: now + DEV_TOKEN_SECONDS })}`;
  return { token: `${input}.${sign("RSA-SHA256", Buffer.from(input), key).toString("base64url")}`, expiresAt: now + DEV_TOKEN_SECONDS };
}
