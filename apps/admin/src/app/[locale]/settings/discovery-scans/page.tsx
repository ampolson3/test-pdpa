import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { DiscoveryContent } from "./discovery-content";

/** DFG-09: automated personal-data discovery over registered connectors (admin: /settings/discovery-scans). */
export default async function DiscoveryPage() {
  const t = await getTranslations("discovery");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <DiscoveryContent />
    </GrantsProvider>
  );
}
