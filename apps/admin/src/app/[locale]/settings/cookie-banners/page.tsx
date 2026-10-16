import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { CookieBannersContent } from "./cookie-banners-content";

/** CON-01 cookie consent banner (admin: /settings/cookie-banners). Gated on cookie.banner.read in CookieBannersContent. */
export default async function CookieBannersPage() {
  const t = await getTranslations("cookieBanners");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <CookieBannersContent apiBase={process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080"} />
    </GrantsProvider>
  );
}
