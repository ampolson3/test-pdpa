import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { DsarFormsContent } from "./dsar-forms-content";

/** DSAR-01 public intake forms (admin: /settings/dsar-forms). Gated on dsar.form.read in DsarFormsContent. */
export default async function DsarFormsPage() {
  const t = await getTranslations("dsarForms");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <DsarFormsContent portalUrl={process.env.PORTAL_PUBLIC_URL ?? "http://localhost:3001"} />
    </GrantsProvider>
  );
}
