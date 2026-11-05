import { notFound } from "next/navigation";
import { setRequestLocale } from "next-intl/server";
import { getPublicPackage } from "@/lib/dsar-package";
import { DownloadPanel } from "./download-panel";

// DSAR-14's public secure delivery link. The key in the URL is the package's own public key; an unknown,
// expired or already-purged package is a 404 — same pattern as the DSAR intake form (DSAR-01) and the
// breach report form (BRE-01).
export const dynamic = "force-dynamic";

export default async function DsarPackagePage({ params }: { params: Promise<{ locale: string; key: string }> }) {
  const { locale, key } = await params;
  setRequestLocale(locale);
  const pkg = await getPublicPackage(key, locale);
  if (!pkg) notFound();
  return (
    <main className="mx-auto max-w-xl p-4 sm:p-8">
      <DownloadPanel pkg={pkg} publicKey={key} locale={locale} />
    </main>
  );
}
