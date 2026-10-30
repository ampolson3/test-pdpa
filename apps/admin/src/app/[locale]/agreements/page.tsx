import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { AgreementsContent } from "./agreements-content";

/** DPA-02 agreement engine (admin: /agreements) — create a DPA from a vendor + its RoPA activities. */
export default async function AgreementsPage() {
  const t = await getTranslations("agreements");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <AgreementsContent />
    </GrantsProvider>
  );
}
