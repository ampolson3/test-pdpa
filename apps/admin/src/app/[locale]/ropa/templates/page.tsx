import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { TemplatesContent } from "./templates-content";

/** RTG-01 standard activity library (admin: /ropa/templates). Gated on ropa.template.read in TemplatesContent. */
export default async function RopaTemplatesPage() {
  const t = await getTranslations("ropaTemplates");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <TemplatesContent />
    </GrantsProvider>
  );
}
