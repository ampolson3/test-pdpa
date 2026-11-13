import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { OrganizationContent } from "./organization-content";

/** ORG-01 legal entities + ORG-04 org-unit tree (admin: /settings/organization). Gated on org.structure.read. */
export default async function OrganizationPage() {
  const t = await getTranslations("organization");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <OrganizationContent />
    </GrantsProvider>
  );
}
