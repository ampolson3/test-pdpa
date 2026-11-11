import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { ConnectorsContent } from "./connectors-content";

/** PLT-20: register and test connectors to customer systems (admin: /settings/connectors). */
export default async function ConnectorsPage() {
  const t = await getTranslations("connectors");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <ConnectorsContent />
    </GrantsProvider>
  );
}
