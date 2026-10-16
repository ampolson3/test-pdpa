import { notFound } from "next/navigation";
import { getTranslations, setRequestLocale } from "next-intl/server";
import { getPublicNotice, getPublicNoticeVersion, getPublicNoticeVersions } from "@/lib/notice";

// The hosted public page of a published notice (PNG-06, ม.23): the key in the URL is the notice's own public
// key. An unknown, unpublished or malformed key is a 404 — same pattern as the /c/[key] consent form.
export const dynamic = "force-dynamic";

export default async function NoticePage({
  params,
  searchParams,
}: {
  params: Promise<{ locale: string; key: string }>;
  searchParams: Promise<{ v?: string }>;
}) {
  const { locale, key } = await params;
  const { v } = await searchParams;
  setRequestLocale(locale);
  const t = await getTranslations("portal.notice");

  const viewingVersion = v ? Number.parseInt(v, 10) : undefined;
  const notice = viewingVersion
    ? await getPublicNoticeVersion(key, viewingVersion, locale)
    : await getPublicNotice(key, locale);
  if (!notice) notFound();

  const versions = await getPublicNoticeVersions(key, locale);
  const lang = notice.languages.includes(locale) ? locale : notice.languages[0];
  const html = notice.content[lang] ?? "";

  return (
    <main className="mx-auto max-w-3xl space-y-6 p-4 sm:p-8">
      <header className="space-y-1 border-b border-slate-200 pb-4">
        <h1 className="text-2xl font-semibold">{notice.title}</h1>
        <p className="text-sm text-slate-600">
          {viewingVersion ? t("viewingVersion", { no: notice.version_no }) : t("currentVersion", { no: notice.version_no })}
          {" · "}
          {t("effectiveFrom")} {notice.effective_from}
        </p>
        {viewingVersion && (
          <a className="inline-block text-sm text-sky-700 underline" href={`/${locale}/n/${key}`}>
            {t("backToCurrent")}
          </a>
        )}
      </header>

      <article className="prose prose-sm max-w-none" dangerouslySetInnerHTML={{ __html: html }} />

      <section className="border-t border-slate-200 pt-4">
        <h2 className="text-sm font-semibold text-slate-700">{t("history")}</h2>
        {versions.length <= 1 ? (
          <p className="text-sm text-slate-500">{t("historyEmpty")}</p>
        ) : (
          <ul className="mt-2 space-y-1 text-sm">
            {versions.map((ver) => (
              <li key={ver.version_no} className="flex items-center justify-between gap-2">
                <span>
                  v{ver.version_no} — {t("effectiveFrom")} {ver.effective_from} ({t("published")} {new Date(ver.published_at).toLocaleDateString(locale)})
                </span>
                <a className="text-sky-700 underline" href={`/${locale}/n/${key}?v=${ver.version_no}`}>
                  {t("view")}
                </a>
              </li>
            ))}
          </ul>
        )}
      </section>
    </main>
  );
}
