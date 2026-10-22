import { notFound } from "next/navigation";
import { getTranslations, setRequestLocale } from "next-intl/server";
import { getPublicCookiePolicy } from "@/lib/cookie-policy";

// CON-05's own Cookie Policy page: a live table of this domain's active cookies (CON-02's own manual
// entries plus CON-04's own scanner-found ones), always matching the latest scan since the backend reads
// it live with no snapshot to go stale. Meant to be embedded via iframe on the tenant's own website — see
// proxy.ts's own frame-ancestors exception for this route — or linked to directly. An unknown or
// unpublished key is a 404, the same pattern as the /n/[key] notice page and /c/[key] consent form.
export const dynamic = "force-dynamic";

export default async function CookiePolicyPage({
  params,
}: {
  params: Promise<{ locale: string; key: string }>;
}) {
  const { locale, key } = await params;
  setRequestLocale(locale);
  const t = await getTranslations("portal.cookiePolicy");

  const policy = await getPublicCookiePolicy(key, locale);
  if (!policy) notFound();

  const categoryName = (entry: (typeof policy.cookies)[number]) =>
    (locale === "en" ? entry.category_name_en : entry.category_name_th) || entry.category_code || "—";
  const description = (entry: (typeof policy.cookies)[number]) =>
    (locale === "en" ? entry.description_en : entry.description_th) || "—";

  return (
    <main className="mx-auto max-w-4xl space-y-4 p-4 sm:p-8">
      <header className="space-y-1 border-b border-slate-200 pb-4">
        <h1 className="text-2xl font-semibold">{t("title")}</h1>
        <p className="text-sm text-slate-600">{t("intro")}</p>
      </header>

      {policy.cookies.length === 0 ? (
        <p className="text-sm text-slate-500">{t("empty")}</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[640px] border-collapse text-left text-sm">
            <thead>
              <tr className="border-b border-slate-300 text-slate-700">
                <th className="py-2 pr-3 font-medium">{t("colName")}</th>
                <th className="py-2 pr-3 font-medium">{t("colProvider")}</th>
                <th className="py-2 pr-3 font-medium">{t("colCategory")}</th>
                <th className="py-2 pr-3 font-medium">{t("colDuration")}</th>
                <th className="py-2 font-medium">{t("colDescription")}</th>
              </tr>
            </thead>
            <tbody>
              {policy.cookies.map((entry, i) => (
                <tr key={`${entry.name}-${i}`} className="border-b border-slate-100 align-top">
                  <td className="py-2 pr-3 font-mono text-xs">{entry.name}</td>
                  <td className="py-2 pr-3">{entry.provider_domain}</td>
                  <td className="py-2 pr-3">{categoryName(entry)}</td>
                  <td className="py-2 pr-3">{entry.duration || "—"}</td>
                  <td className="py-2">{description(entry)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </main>
  );
}
