// Server-only module (reads request headers, holds the API address) — the CON-18 self-service preference
// centre: view and edit every active purpose's own consent, OTP-verified first (CON-19's own primitive, under
// its own "preference_center" purpose so this OTP can't double as a plain consent-form one or vice versa).
import { headers } from "next/headers";

const API_BASE = process.env.API_BASE_URL ?? "http://localhost:8080";

export type Problem = { status: number; code: string; title: string; errors?: { field: string; code: string }[] };

export type PreferenceItem = {
  code: string;
  version: number;
  name: string;
  description?: string;
  text?: string;
  explicit_text?: string;
  is_sensitive: boolean;
  decision?: "ACTIVE" | "NOT_GIVEN" | "WITHDRAWN" | "EXPIRED";
  preferences?: { code: string; name: string; pref_type: string; options?: { value: string; label: string }[] }[];
  current?: Record<string, unknown> | null;
  expires_at?: string;
};

export type PreferenceCenter = {
  identifiers: { type: string; masked: string; primary: boolean; verified: boolean }[];
  purposes: PreferenceItem[];
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

async function post<T>(key: string, path: string, idempotencyKey: string, locale: string, body: unknown): Promise<{ ok: true; data: T } | { ok: false; problem: Problem }> {
  const res = await fetch(`${API_BASE}/public/v1/preference-centers/${encodeURIComponent(key)}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json", "Idempotency-Key": idempotencyKey, "Accept-Language": locale, ...(await forwarded()) },
    body: JSON.stringify(body),
    cache: "no-store",
  });
  const json = await res.json().catch(() => ({}));
  if (res.ok) return { ok: true, data: json as T };
  return { ok: false, problem: { status: res.status, code: json.code ?? "unknown", title: json.title ?? "", errors: json.errors } };
}

export async function startPreferenceVerification(key: string, idempotencyKey: string, identifier: { type: string; value: string }, locale: string) {
  return post<{ verification_id: string; channel: string }>(key, "/start-verification", idempotencyKey, locale, { identifier });
}

export async function viewPreferenceCenter(
  key: string,
  idempotencyKey: string,
  verificationId: string,
  code: string,
  identifier: { type: string; value: string },
  locale: string,
) {
  return post<PreferenceCenter>(key, "/view", idempotencyKey, locale, { verification_id: verificationId, code: code || undefined, identifier });
}

export async function updatePreferences(
  key: string,
  idempotencyKey: string,
  verificationId: string,
  identifier: { type: string; value: string },
  decisions: { purpose_code: string; purpose_version_no: number; decision: "CONSENTED" | "NOT_CONSENTED" | "WITHDRAWN" }[],
  locale: string,
) {
  return post<{ receipt_no: string }>(key, "/update", idempotencyKey, locale, { verification_id: verificationId, identifier, decisions, language: locale });
}
