// Server-only module (reads request headers, holds the API address) — CON-24's own recipient link: the
// campaign's own public key (X-Public-Key) names the tenant/campaign, the token in the query string names
// which recipient; neither identifies anything on its own without the other.
import { headers } from "next/headers";

const API_BASE = process.env.API_BASE_URL ?? "http://localhost:8080";

export type Problem = { status: number; code: string; title: string; errors?: { field: string; code: string }[] };

export type CampaignPurpose = {
  code: string;
  version_no: number;
  name: string;
  text: string;
  explicit_text?: string;
  is_sensitive: boolean;
};

export type CampaignView = {
  campaign_name: string;
  purposes: CampaignPurpose[];
  responded: boolean;
};

async function forwarded(): Promise<Record<string, string>> {
  const h = await headers();
  const out: Record<string, string> = {};
  const ua = h.get("user-agent");
  if (ua) out["User-Agent"] = ua;
  const xff = h.get("x-forwarded-for");
  if (xff) out["X-Forwarded-For"] = xff;
  return out;
}

async function post<T>(key: string, path: string, idempotencyKey: string | undefined, locale: string, body: unknown): Promise<{ ok: true; data: T } | { ok: false; problem: Problem }> {
  const headersInit: Record<string, string> = { "Content-Type": "application/json", "X-Public-Key": key, "Accept-Language": locale, ...(await forwarded()) };
  if (idempotencyKey) headersInit["Idempotency-Key"] = idempotencyKey;
  const res = await fetch(`${API_BASE}/public/v1/consent-campaigns${path}`, {
    method: "POST",
    headers: headersInit,
    body: JSON.stringify(body),
    cache: "no-store",
  });
  const json = await res.json().catch(() => ({}));
  if (res.ok) return { ok: true, data: json as T };
  return { ok: false, problem: { status: res.status, code: json.code ?? "unknown", title: json.title ?? "", errors: json.errors } };
}

/** Resolves the recipient's own link and marks it opened — no Idempotency-Key needed, this is a read. */
export async function viewCampaign(key: string, token: string, locale: string) {
  return post<CampaignView>(key, "/view", undefined, locale, { token });
}

export async function respondToCampaign(
  key: string,
  idempotencyKey: string,
  token: string,
  decisions: { purpose_code: string; purpose_version_no: number; decision: "CONSENTED" | "NOT_CONSENTED" }[],
  locale: string,
) {
  return post<{ receipt_no: string }>(key, "/respond", idempotencyKey, locale, { token, decisions });
}
