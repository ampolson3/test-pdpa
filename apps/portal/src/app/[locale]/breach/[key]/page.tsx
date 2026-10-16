import { randomUUID } from "node:crypto";
import { notFound } from "next/navigation";
import { setRequestLocale } from "next-intl/server";
import { getPublicReportForm } from "@/lib/breach";
import { BreachReportForm } from "./breach-report-form";

// BRE-01's public incident-report form. The key in the URL is the report form's public key; an unknown,
// retired or malformed key is a 404 — same pattern as the DSAR intake form (DSAR-01) and the published
// notice page (PNG-06).
export const dynamic = "force-dynamic";

export default async function BreachReportFormPage({ params }: { params: Promise<{ locale: string; key: string }> }) {
  const { locale, key } = await params;
  setRequestLocale(locale);
  const form = await getPublicReportForm(key, locale);
  if (!form) notFound();
  return (
    <main className="mx-auto max-w-2xl p-4 sm:p-8">
      <BreachReportForm form={form} publicKey={key} locale={locale} idem={randomUUID()} />
    </main>
  );
}
