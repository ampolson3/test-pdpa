// Server-only module (reads request headers, holds the API address).
import { headers } from "next/headers";

// Server-side calls to the /public/v1 consent API (BP-01). The browser never talks to the API directly: the
// public key travels in the page URL, and the portal forwards the visitor's address and browser so the consent
// transaction records them — list the portal in the API's TRUSTED_PROXIES (decisions.md D-23), or every consent
// would carry the portal's own address.
const API_BASE = process.env.API_BASE_URL ?? "http://localhost:8080";

export type PublicPurpose = {
  code: string;
  version_no: number;
  name: string;
  description?: string;
  text: string;
  explicit_text?: string;
  is_sensitive: boolean;
  requires_explicit: boolean;
  required?: boolean;
  min_age?: number;
  preferences?: { code: string; name: string; pref_type: string; options?: { value: string; label: string }[] }[];
};

export type PublicCollectionPoint = { code: string; name: string; channel: string; purposes: PublicPurpose[]; age_gate?: boolean };

export type Decision = { purpose_code: string; purpose_version_no: number; decision: "CONSENTED" | "NOT_CONSENTED"; preferences?: Record<string, string[]> };

export type Problem = { status: number; code: string; title: string; errors?: { field: string; code: string }[] };

async function forwarded(): Promise<Record<string, string>> {
  const h = await headers();
  const out: Record<string, string> = {};
  const ua = h.get("user-agent");
  if (ua) out["User-Agent"] = ua;
  // Our own hop appended by the platform's proxy chain is in x-forwarded-for already; pass it on unchanged.
  const xff = h.get("x-forwarded-for");
  if (xff) out["X-Forwarded-For"] = xff;
  return out;
}

export async function getCollectionPoint(key: string, locale: string): Promise<PublicCollectionPoint | null> {
  const res = await fetch(`${API_BASE}/public/v1/collection-points/${encodeURIComponent(key)}`, {
    headers: { "Accept-Language": locale, ...(await forwarded()) },
    cache: "no-store",
  });
  if (res.status === 404 || res.status === 400) return null;
  if (!res.ok) throw new Error(`consent form: ${res.status}`);
  return res.json();
}

export async function submitConsents(
  key: string,
  idempotencyKey: string,
  body: { subject: { identifiers: { type: string; value: string }[] }; decisions: Decision[]; language: string },
): Promise<{ ok: true; receipt_no: string } | { ok: false; problem: Problem }> {
  const res = await fetch(`${API_BASE}/public/v1/consents`, {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-Public-Key": key, "Idempotency-Key": idempotencyKey, "Accept-Language": body.language, ...(await forwarded()) },
    body: JSON.stringify(body),
    cache: "no-store",
  });
  const json = await res.json().catch(() => ({}));
  if (res.ok) return { ok: true, receipt_no: json.receipt_no };
  return { ok: false, problem: { status: res.status, code: json.code ?? "unknown", title: json.title ?? "", errors: json.errors } };
}
