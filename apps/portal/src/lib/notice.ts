// Server-only module: PNG-06's public notice page reads /public/v1/notices/{key} directly, the same way
// consent.ts reads /public/v1/collection-points/{key} — the browser never talks to the API directly.
const API_BASE = process.env.API_BASE_URL ?? "http://localhost:8080";

export type PublicNotice = {
  title: string;
  notice_type: string;
  version_no: number;
  effective_from: string;
  languages: string[];
  content: Record<string, string>;
};

export type PublicNoticeVersion = { version_no: number; effective_from: string; published_at: string };

export async function getPublicNotice(key: string, locale: string): Promise<PublicNotice | null> {
  const res = await fetch(`${API_BASE}/public/v1/notices/${encodeURIComponent(key)}`, {
    headers: { "Accept-Language": locale },
    cache: "no-store",
  });
  if (res.status === 404 || res.status === 400) return null;
  if (!res.ok) throw new Error(`public notice: ${res.status}`);
  return res.json();
}

export async function getPublicNoticeVersions(key: string, locale: string): Promise<PublicNoticeVersion[]> {
  const res = await fetch(`${API_BASE}/public/v1/notices/${encodeURIComponent(key)}/versions`, {
    headers: { "Accept-Language": locale },
    cache: "no-store",
  });
  if (!res.ok) return [];
  const data = await res.json();
  return data.data ?? [];
}

export async function getPublicNoticeVersion(key: string, versionNo: number, locale: string): Promise<PublicNotice | null> {
  const res = await fetch(`${API_BASE}/public/v1/notices/${encodeURIComponent(key)}/versions/${versionNo}`, {
    headers: { "Accept-Language": locale },
    cache: "no-store",
  });
  if (res.status === 404 || res.status === 400) return null;
  if (!res.ok) throw new Error(`public notice version: ${res.status}`);
  return res.json();
}
