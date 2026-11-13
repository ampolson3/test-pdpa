import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { ExternalPartiesContent } from "./external-parties-content";

/** ORG-06 external parties directory (admin: /settings/external-parties). Gated on org.party.read in ExternalPartiesContent. */
export default async function ExternalPartiesPage() {
  const t = await getTranslations("externalParties");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <ExternalPartiesContent />
    </GrantsProvider>
  );
}
