// Server-only module (reads request headers, holds the API address). DSAR-01's public intake form: the
// browser never talks to the API directly — same pattern as consent.ts (BP-01) and notice.ts (PNG-06).
import { headers } from "next/headers";

const API_BASE = process.env.API_BASE_URL ?? "http://localhost:8080";

export type DsarRequestType = { id: string; code: string; name_th: string; name_en?: string; legal_ref?: string; sla_days: number };
export type DsarPublicIntakeForm = { id: string; name: string; request_types: DsarRequestType[] };
export type DsarVerification = { id: string; request_id: string; method: string; status: string; created_at: string };
export type DsarPublicSubmissionResult = { request_no: string; status: string; verification?: DsarVerification | null };
export type Problem = { status: number; code: string; title: string; errors?: { field: string; code: string }[] };

async function forwarded(): Promise<Record<string, string>> {
  const h = await headers();
  const out: Record<string, string> = {};
  const ua = h.get("user-agent");
  if (ua) out["User-Agent"] = ua;
  const xff = h.get("x-forwarded-for");
  if (xff) out["X-Forwarded-For"] = xff;
  return out;
}

export async function getPublicIntakeForm(key: string, locale: string): Promise<DsarPublicIntakeForm | null> {
  const res = await fetch(`${API_BASE}/public/v1/dsar-forms/${encodeURIComponent(key)}`, {
    headers: { "Accept-Language": locale, ...(await forwarded()) },
    cache: "no-store",
  });
  if (res.status === 404 || res.status === 400) return null;
  if (!res.ok) throw new Error(`dsar intake form: ${res.status}`);
  return res.json();
}

export async function submitPublicRequest(
  key: string,
  idempotencyKey: string,
  locale: string,
  body: { request_type_code: string; requester_name: string; requester_email?: string; requester_phone?: string; data_source?: string },
): Promise<{ ok: true; result: DsarPublicSubmissionResult } | { ok: false; problem: Problem }> {
  const res = await fetch(`${API_BASE}/public/v1/dsar-forms/${encodeURIComponent(key)}/requests`, {
    method: "POST",
    headers: { "Content-Type": "application/json", "Idempotency-Key": idempotencyKey, "Accept-Language": locale, ...(await forwarded()) },
    body: JSON.stringify(body),
    cache: "no-store",
  });
  if (res.ok) return { ok: true, result: await res.json() };
  return { ok: false, problem: await res.json() };
}

export async function confirmPublicVerification(
  key: string,
  requestId: string,
  idempotencyKey: string,
  locale: string,
  body: { verification_id: string; code: string },
): Promise<{ ok: true; verification: DsarVerification } | { ok: false; problem: Problem }> {
  const res = await fetch(`${API_BASE}/public/v1/dsar-forms/${encodeURIComponent(key)}/requests/${encodeURIComponent(requestId)}/verify`, {
    method: "POST",
    headers: { "Content-Type": "application/json", "Idempotency-Key": idempotencyKey, "Accept-Language": locale, ...(await forwarded()) },
    body: JSON.stringify(body),
    cache: "no-store",
  });
  if (res.ok) return { ok: true, verification: await res.json() };
  return { ok: false, problem: await res.json() };
}
