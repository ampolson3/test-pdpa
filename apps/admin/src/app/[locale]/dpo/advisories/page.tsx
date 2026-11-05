import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { AdvisoriesContent } from "./advisories-content";

/** DPO-07 advisory requests + searchable knowledge base (admin: /dpo/advisories). Gated on
 *  dpo.advisory.read in AdvisoriesContent. */
export default async function DpoAdvisoriesPage() {
  const t = await getTranslations("dpoAdvisories");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <AdvisoriesContent />
    </GrantsProvider>
  );
}
