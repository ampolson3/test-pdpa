import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { BatchWizardContent } from "./batch-wizard-content";

/** RTG-04 one-click generation (admin: /ropa/templates/batch). Gated on ropa.activity.create in BatchWizardContent. */
export default async function RopaTemplatesBatchPage() {
  const t = await getTranslations("ropaTemplates");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <BatchWizardContent />
    </GrantsProvider>
  );
}
