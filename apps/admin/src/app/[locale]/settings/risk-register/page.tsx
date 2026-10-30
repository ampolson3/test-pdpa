import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { RiskRegisterContent } from "./risk-register-content";

/** RRA-09: one shared risk register for DPIA/RoPA/vendor-sourced risks (admin: /settings/risk-register). */
export default async function RiskRegisterPage() {
  const t = await getTranslations("riskRegister");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <RiskRegisterContent />
    </GrantsProvider>
  );
}
