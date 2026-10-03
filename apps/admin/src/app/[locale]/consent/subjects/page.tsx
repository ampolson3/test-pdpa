import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { SubjectsContent } from "./subjects-content";

/** CON-13/15/17 consent records by data subject (admin: /consent/subjects). Gated on consent.record.read in SubjectsContent. */
export default async function SubjectsPage() {
  const t = await getTranslations("consent");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <SubjectsContent />
    </GrantsProvider>
  );
}
