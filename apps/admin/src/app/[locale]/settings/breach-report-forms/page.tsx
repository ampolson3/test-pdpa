import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { BreachReportFormsContent } from "./breach-report-forms-content";

/** BRE-01 public incident-report forms (admin: /settings/breach-report-forms). Gated on breach.form.read in BreachReportFormsContent. */
export default async function BreachReportFormsPage() {
  const t = await getTranslations("breachReportForms");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <BreachReportFormsContent portalUrl={process.env.PORTAL_PUBLIC_URL ?? "http://localhost:3001"} />
    </GrantsProvider>
  );
}
