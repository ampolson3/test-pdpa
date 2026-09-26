import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { AuditContent } from "./audit-content";

/** ORG-19 audit log (admin: /settings/audit). Gated on admin.audit.read in AuditContent. */
export default async function AuditPage() {
  const t = await getTranslations("auditLog");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <AuditContent />
    </GrantsProvider>
  );
}
