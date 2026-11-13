import { randomUUID } from "node:crypto";
import { setRequestLocale } from "next-intl/server";
import { TrackForm } from "./track-form";

// DSAR-19's public status-tracking portal. Unlike the intake form (DSAR-01), there is no server-side GET
// to resolve first — every real check (does this key name a real published tracking portal, does the
// request number/contact pair resolve) happens behind the OTP gate on the first POST, so an unknown key
// reads identically to a wrong request number/contact — no information leak either way, decided entirely
// by the API rather than this page.
export const dynamic = "force-dynamic";

export default async function DsarStatusPage({ params }: { params: Promise<{ locale: string; key: string }> }) {
  const { locale, key } = await params;
  setRequestLocale(locale);
  return (
    <main className="mx-auto max-w-2xl p-4 sm:p-8">
      <TrackForm publicKey={key} locale={locale} idem={randomUUID()} />
    </main>
  );
}
