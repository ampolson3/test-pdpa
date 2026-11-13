// Server-only module: CON-05's public Cookie Policy page reads /public/v1/cookie-banners/{key}/policy
// directly, the same way notice.ts reads /public/v1/notices/{key} — the browser never talks to the API
// directly.
const API_BASE = process.env.API_BASE_URL ?? "http://localhost:8080";

export type PublicCookiePolicyEntry = {
  name: string;
  provider_domain: string;
  category_code?: string;
  category_name_th?: string;
  category_name_en?: string;
  storage_type: string;
  duration?: string;
  description_th?: string;
  description_en?: string;
};

export type PublicCookiePolicy = {
  domain_id: string;
  cookies: PublicCookiePolicyEntry[];
};

export async function getPublicCookiePolicy(key: string, locale: string): Promise<PublicCookiePolicy | null> {
  const res = await fetch(`${API_BASE}/public/v1/cookie-banners/${encodeURIComponent(key)}/policy`, {
    headers: { "Accept-Language": locale },
    cache: "no-store",
  });
  if (res.status === 404 || res.status === 400) return null;
  if (!res.ok) throw new Error(`public cookie policy: ${res.status}`);
  return res.json();
}
