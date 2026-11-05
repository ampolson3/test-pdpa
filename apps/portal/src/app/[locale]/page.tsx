import { getTranslations, setRequestLocale } from "next-intl/server";

// PLT-17: the portal's own main page. No per-tenant data to fetch (the subdomain / custom-domain
// resolver docs/architecture/security.md describes is deferred — PLT-02's own note — until a feature
// actually needs tenant context on a page; this one doesn't), so the page is fully static: no database
// call, no client-side fetch, nothing to block the first paint. That's also most of what "loads within
// 2 seconds on 4G" asks for — the rest is the shared layout staying server-rendered with a single small
// client island (the locale switcher).
export const dynamic = "force-static";

export default async function PortalHome({ params }: { params: Promise<{ locale: string }> }) {
  const { locale } = await params;
  setRequestLocale(locale);
  const t = await getTranslations("portal.home");

  return (
    <main className="mx-auto max-w-2xl space-y-8 p-4 sm:p-8">
      <section aria-labelledby="portal-home-title">
        <h1 id="portal-home-title" className="text-2xl font-semibold text-slate-900 sm:text-3xl">
          {t("title")}
        </h1>
        <p className="mt-3 text-base text-slate-700">{t("intro")}</p>
      </section>

      <section aria-labelledby="portal-home-links-title" className="rounded-lg border border-slate-200 bg-white p-6">
        <h2 id="portal-home-links-title" className="text-lg font-semibold text-slate-900">
          {t("linksTitle")}
        </h2>
        <p className="mt-2 text-sm text-slate-700">{t("linksBody")}</p>
      </section>

      <section aria-labelledby="portal-home-rights-title" className="rounded-lg border border-slate-200 bg-white p-6">
        <h2 id="portal-home-rights-title" className="text-lg font-semibold text-slate-900">
          {t("rightsTitle")}
        </h2>
        <ul className="mt-2 list-disc space-y-1 pl-5 text-sm text-slate-700">
          <li>{t("rights.access")}</li>
          <li>{t("rights.rectify")}</li>
          <li>{t("rights.erase")}</li>
          <li>{t("rights.object")}</li>
          <li>{t("rights.withdraw")}</li>
        </ul>
        <p className="mt-3 text-sm text-slate-600">{t("rightsContact")}</p>
      </section>
    </main>
  );
}
