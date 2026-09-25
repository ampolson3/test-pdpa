import { getTranslations } from "next-intl/server";
import { loadMe } from "@/lib/me";
import { TasksContent } from "./tasks-content";

/** PLT-05 task board (admin: /tasks): the signed-in user's open workflow tasks. */
export default async function TasksPage() {
  const t = await getTranslations("workflow");
  const me = await loadMe();
  if (!me) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("signInRequired")}</main>;
  return <TasksContent currentUserId={me.id} />;
}
