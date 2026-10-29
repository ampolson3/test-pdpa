import { getTranslations } from "next-intl/server";
import { GrantsProvider } from "@pdpa/authz";
import { loadMe } from "@/lib/me";
import { DpoTasksContent } from "./tasks-content";

/** DPO-06 tasks management (admin: /dpo/tasks): the kanban/list view of every dpo.tasks job every other
 *  feature auto-creates, with an assign action. Gated on dpo.task.read in DpoTasksContent. */
export default async function DpoTasksPage() {
  const t = await getTranslations("dpoTasks");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return (
    <GrantsProvider grants={{ roles: me.roles, permissions: me.permissions, scopes: me.scopes }}>
      <DpoTasksContent currentUserId={me.id} />
    </GrantsProvider>
  );
}
