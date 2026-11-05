import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { DsarWorkflowContent } from "./dsar-workflow-content";

/** DSAR-20 configurable workflow per right type (admin: /settings/dsar-workflow). Gated on dsar.form.read. */
export default async function DsarWorkflowPage() {
  const t = await getTranslations("dsarWorkflow");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <DsarWorkflowContent />
    </GrantsProvider>
  );
}
