import { randomUUID } from "node:crypto";
import { setRequestLocale } from "next-intl/server";
import { PreferenceCenterForm } from "./preference-center-form";

// CON-18: the self-service preference centre. The key in the URL names a legal entity's published link
// (platform.public_keys, entity_type "portal") — there is nothing to fetch before OTP verification, so this
// page renders the form directly; an unknown or revoked key surfaces as an error on the first server action.
export const dynamic = "force-dynamic";

export default async function PreferenceCenterPage({ params }: { params: Promise<{ locale: string; key: string }> }) {
  const { locale, key } = await params;
  setRequestLocale(locale);
  return (
    <main className="mx-auto max-w-2xl p-4 sm:p-8">
      <PreferenceCenterForm publicKey={key} locale={locale} idem={randomUUID()} />
    </main>
  );
}
