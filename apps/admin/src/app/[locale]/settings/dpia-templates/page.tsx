import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { DpiaTemplatesContent } from "./dpia-templates-content";

/** DPIA-03 template library (admin: /settings/dpia-templates). Gated on assessment.template.read in the content component. */
export default async function DpiaTemplatesPage() {
  const t = await getTranslations("dpia");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("templates.forbidden")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <DpiaTemplatesContent />
    </GrantsProvider>
  );
}
