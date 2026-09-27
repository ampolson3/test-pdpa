import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { RequestsContent } from "./requests-content";

/** DSAR requests (ST-02, admin: /requests). Gated on dsar.request.read. */
export default async function RequestsPage() {
  const t = await getTranslations("dsarRequests");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <RequestsContent />
    </GrantsProvider>
  );
}
