import { randomUUID } from "node:crypto";
import { notFound } from "next/navigation";
import { setRequestLocale } from "next-intl/server";
import { ConfirmForm } from "./confirm-form";

// CON-23: the e-mailed/SMS link a double-opt-in-gated collection point sends. The key names the collection
// point (and so the tenant); the token in the query string is the one the server issued when the subject's
// own decision was recorded — neither identifies anything on its own without the other.
export const dynamic = "force-dynamic";

export default async function ConsentConfirmPage({
  params,
  searchParams,
}: {
  params: Promise<{ locale: string; key: string }>;
  searchParams: Promise<{ token?: string }>;
}) {
  const { locale, key } = await params;
  const { token } = await searchParams;
  setRequestLocale(locale);
  if (!token) notFound();
  return (
    <main className="mx-auto max-w-2xl p-4 sm:p-8">
      <ConfirmForm publicKey={key} locale={locale} token={token} idem={randomUUID()} />
    </main>
  );
}
