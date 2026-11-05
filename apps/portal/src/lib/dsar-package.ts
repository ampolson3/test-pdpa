// Server-only module (reads request headers, holds the API address). DSAR-14's public secure-delivery
// download: the browser never talks to the API directly — same pattern as dsar.ts (DSAR-01).
import { headers } from "next/headers";

const API_BASE = process.env.API_BASE_URL ?? "http://localhost:8080";

export type DsarPublicPackage = { format: string; password_protected: boolean; expires_at: string };
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

export async function getPublicPackage(key: string, locale: string): Promise<DsarPublicPackage | null> {
  const res = await fetch(`${API_BASE}/public/v1/dsar-packages/${encodeURIComponent(key)}`, {
    headers: { "Accept-Language": locale, ...(await forwarded()) },
    cache: "no-store",
  });
  if (res.status === 404 || res.status === 400) return null;
  if (!res.ok) throw new Error(`dsar package: ${res.status}`);
  return res.json();
}

export async function downloadPublicPackage(
  key: string,
  locale: string,
  accessCode: string,
  idempotencyKey: string,
): Promise<{ ok: true; downloadUrl: string } | { ok: false; problem: Problem }> {
  const res = await fetch(`${API_BASE}/public/v1/dsar-packages/${encodeURIComponent(key)}/download`, {
    method: "POST",
    headers: { "Content-Type": "application/json", "Idempotency-Key": idempotencyKey, "Accept-Language": locale, ...(await forwarded()) },
    body: JSON.stringify(accessCode ? { access_code: accessCode } : {}),
    cache: "no-store",
  });
  if (res.ok) {
    const data = await res.json();
    return { ok: true, downloadUrl: data.download_url };
  }
  return { ok: false, problem: await res.json() };
}
