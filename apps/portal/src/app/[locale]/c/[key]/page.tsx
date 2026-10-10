import { randomUUID } from "node:crypto";
import { notFound } from "next/navigation";
import { setRequestLocale } from "next-intl/server";
import { getCollectionPoint } from "@/lib/consent";
import { ConsentForm } from "./consent-form";

// The hosted consent form of a published collection point (CON-09, BP-01 t1–t4). The key in the URL is the
// collection point's public key; an unknown, retired or malformed key is a 404.
export const dynamic = "force-dynamic";

export default async function ConsentPage({ params }: { params: Promise<{ locale: string; key: string }> }) {
  const { locale, key } = await params;
  setRequestLocale(locale);
  const cp = await getCollectionPoint(key, locale);
  if (!cp) notFound();
  return (
    <main className="mx-auto max-w-2xl p-4 sm:p-8">
      <ConsentForm cp={cp} publicKey={key} locale={locale} idem={randomUUID()} />
    </main>
  );
}
