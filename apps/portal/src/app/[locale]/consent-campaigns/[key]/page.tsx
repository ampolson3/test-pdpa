import { randomUUID } from "node:crypto";
import { notFound } from "next/navigation";
import { setRequestLocale } from "next-intl/server";
import { viewCampaign } from "@/lib/campaigns";
import { CampaignForm } from "./campaign-form";

// CON-24: the e-mailed/SMS link a bulk consent request campaign sends. The key names the campaign (and so
// the tenant); the token in the query string names which recipient — neither works without the other.
export const dynamic = "force-dynamic";

export default async function ConsentCampaignPage({
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
  const res = await viewCampaign(key, token, locale);
  if (!res.ok) notFound();
  return (
    <main className="mx-auto max-w-2xl p-4 sm:p-8">
      <CampaignForm view={res.data} publicKey={key} locale={locale} token={token} idem={randomUUID()} />
    </main>
  );
}
