import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { IncidentsContent } from "./incidents-content";

/** BRE-02 / 13 breach register (admin: /incidents). What the caller sees is decided by the API. */
export default async function IncidentsPage() {
  const t = await getTranslations("breach");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <IncidentsContent />
    </GrantsProvider>
  );
}
