import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { RopaWizardContent } from "./ropa-wizard-content";

/** RTG-05 plain-language wizard (admin: /ropa/wizard). Gated on ropa.activity.create in RopaWizardContent. */
export default async function RopaWizardPage() {
  const t = await getTranslations("ropaWizard");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <RopaWizardContent />
    </GrantsProvider>
  );
}
