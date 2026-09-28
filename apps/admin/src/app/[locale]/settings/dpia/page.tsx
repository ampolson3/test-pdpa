import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { DpiaSettingsContent } from "./dpia-settings-content";

/** DPIA-02 screening thresholds (admin: /settings/dpia). Gated on assessment.template.read in DpiaSettingsContent. */
export default async function DpiaSettingsPage() {
  const t = await getTranslations("dpia");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-3xl p-8 text-slate-600">{t("forbidden")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <DpiaSettingsContent />
    </GrantsProvider>
  );
}
