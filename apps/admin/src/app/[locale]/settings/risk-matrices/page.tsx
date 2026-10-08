import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { RiskMatricesContent } from "./risk-matrices-content";

/** RRA-02: configurable likelihood x impact risk matrices (admin: /settings/risk-matrices). */
export default async function RiskMatricesPage() {
  const t = await getTranslations("riskMatrices");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <RiskMatricesContent />
    </GrantsProvider>
  );
}
