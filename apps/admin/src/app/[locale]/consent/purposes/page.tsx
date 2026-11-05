import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { PurposesContent } from "./purposes-content";

/** CON-12 consent purposes, with CON-10 explicit consent for sensitive data (admin: /consent/purposes). Gated on consent.purpose.read in PurposesContent. */
export default async function PurposesPage() {
  const t = await getTranslations("consent");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <PurposesContent />
    </GrantsProvider>
  );
}
