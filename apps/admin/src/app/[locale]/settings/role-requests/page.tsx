import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { RoleRequestsContent } from "./role-requests-content";

/** IAM-06 dual control (admin: /settings/role-requests): ORGADMIN requests granting ORGADMIN to someone,
 * SEC decides via the generic /approvals inbox, ORGADMIN publishes once approved — the action has no
 * effect until then. */
export default async function RoleRequestsPage() {
  const t = await getTranslations("roleRequests");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <RoleRequestsContent />
    </GrantsProvider>
  );
}
