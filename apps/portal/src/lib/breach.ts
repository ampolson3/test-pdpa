// Server-only module (reads request headers, holds the API address). BRE-01's public incident-report form:
// the browser never talks to the API directly — same pattern as dsar.ts (DSAR-01) and notice.ts (PNG-06).
import { headers } from "next/headers";

const API_BASE = process.env.API_BASE_URL ?? "http://localhost:8080";

export type BreachType = "confidentiality" | "integrity" | "availability";
export type BreachPublicReportForm = { id: string; breach_types: BreachType[] };
export type BreachPublicReportResult = { incident_no: string; status: string };
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

export async function getPublicReportForm(key: string, locale: string): Promise<BreachPublicReportForm | null> {
  const res = await fetch(`${API_BASE}/public/v1/breach-report-forms/${encodeURIComponent(key)}`, {
    headers: { "Accept-Language": locale, ...(await forwarded()) },
    cache: "no-store",
  });
  if (res.status === 404 || res.status === 400) return null;
  if (!res.ok) throw new Error(`breach report form: ${res.status}`);
  return res.json();
}

export async function submitPublicReport(
  key: string,
  idempotencyKey: string,
  locale: string,
  body: { title: string; description: string; breach_types: BreachType[]; occurred_at?: string; reporter_contact?: string },
): Promise<{ ok: true; result: BreachPublicReportResult } | { ok: false; problem: Problem }> {
  const res = await fetch(`${API_BASE}/public/v1/breach-report-forms/${encodeURIComponent(key)}/incidents`, {
    method: "POST",
    headers: { "Content-Type": "application/json", "Idempotency-Key": idempotencyKey, "Accept-Language": locale, ...(await forwarded()) },
    body: JSON.stringify(body),
    cache: "no-store",
  });
  if (res.ok) return { ok: true, result: await res.json() };
  return { ok: false, problem: await res.json() };
}
