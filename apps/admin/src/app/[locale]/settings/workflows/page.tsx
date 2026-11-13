import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { WorkflowsContent } from "./workflows-content";

/** PLT-05 workflow & SLA settings (admin: /settings/workflows). Gated on admin.workflow.read in WorkflowsContent. */
export default async function WorkflowsPage() {
  const t = await getTranslations("workflow");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <WorkflowsContent />
    </GrantsProvider>
  );
}
