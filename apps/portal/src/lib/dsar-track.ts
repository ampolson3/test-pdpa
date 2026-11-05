// Server-only module (reads request headers, holds the API address). DSAR-19's public status-tracking
// portal: the browser never talks to the API directly — same pattern as dsar.ts (DSAR-01).
import { headers } from "next/headers";

const API_BASE = process.env.API_BASE_URL ?? "http://localhost:8080";

export type DsarTrackMethod = "otp_sms" | "otp_email";
export type DsarTrackVerificationResult = { verification_id: string; channel: "sms" | "email" };
export type DsarTrackedStatus = {
  request_no: string;
  status: string;
  received_at: string;
  due_at: string;
  closed_at?: string | null;
  outcome?: string | null;
  rejection_reason_code?: string | null;
};
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

export async function startTrackingVerification(
  key: string,
  idempotencyKey: string,
  locale: string,
  body: { request_no: string; method: DsarTrackMethod; contact: string },
): Promise<{ ok: true; result: DsarTrackVerificationResult } | { ok: false; problem: Problem }> {
  const res = await fetch(`${API_BASE}/public/v1/dsar-tracking/${encodeURIComponent(key)}/start-verification`, {
    method: "POST",
    headers: { "Content-Type": "application/json", "Idempotency-Key": idempotencyKey, "Accept-Language": locale, ...(await forwarded()) },
    body: JSON.stringify(body),
    cache: "no-store",
  });
  if (res.ok) return { ok: true, result: await res.json() };
  return { ok: false, problem: await res.json() };
}

export async function getTrackedStatus(
  key: string,
  idempotencyKey: string,
  locale: string,
  body: { request_no: string; method: DsarTrackMethod; contact: string; verification_id: string; code?: string },
): Promise<{ ok: true; status: DsarTrackedStatus } | { ok: false; problem: Problem }> {
  const res = await fetch(`${API_BASE}/public/v1/dsar-tracking/${encodeURIComponent(key)}/status`, {
    method: "POST",
    headers: { "Content-Type": "application/json", "Idempotency-Key": idempotencyKey, "Accept-Language": locale, ...(await forwarded()) },
    body: JSON.stringify(body),
    cache: "no-store",
  });
  if (res.ok) return { ok: true, status: await res.json() };
  return { ok: false, problem: await res.json() };
}

export async function submitTrackingDocument(
  key: string,
  idempotencyKey: string,
  locale: string,
  fields: { request_no: string; method: DsarTrackMethod; contact: string; verification_id: string; code?: string },
  file: File,
): Promise<{ ok: true; fileId: string } | { ok: false; problem: Problem }> {
  const form = new FormData();
  form.set("request_no", fields.request_no);
  form.set("method", fields.method);
  form.set("contact", fields.contact);
  form.set("verification_id", fields.verification_id);
  if (fields.code) form.set("code", fields.code);
  form.set("file", file, file.name);
  const res = await fetch(`${API_BASE}/public/v1/dsar-tracking/${encodeURIComponent(key)}/documents`, {
    method: "POST",
    headers: { "Idempotency-Key": idempotencyKey, "Accept-Language": locale, ...(await forwarded()) },
    body: form,
    cache: "no-store",
  });
  if (res.ok) return { ok: true, fileId: (await res.json()).file_id };
  return { ok: false, problem: await res.json() };
}
