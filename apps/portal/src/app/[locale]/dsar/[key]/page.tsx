import { randomUUID } from "node:crypto";
import { notFound } from "next/navigation";
import { setRequestLocale } from "next-intl/server";
import { getPublicIntakeForm } from "@/lib/dsar";
import { DsarForm } from "./dsar-form";

// DSAR-01's public intake form. The key in the URL is the intake form's public key; an unknown, retired or
// malformed key is a 404 — same pattern as the consent form (CON-09) and the published notice page (PNG-06).
export const dynamic = "force-dynamic";

export default async function DsarFormPage({ params }: { params: Promise<{ locale: string; key: string }> }) {
  const { locale, key } = await params;
  setRequestLocale(locale);
  const form = await getPublicIntakeForm(key, locale);
  if (!form) notFound();
  return (
    <main className="mx-auto max-w-2xl p-4 sm:p-8">
      <DsarForm form={form} publicKey={key} locale={locale} idem={randomUUID()} />
    </main>
  );
}
