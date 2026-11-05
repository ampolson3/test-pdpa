"use client";

import { useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Button } from "@pdpa/ui";
import { formatDate, type Locale } from "@pdpa/i18n";
import { createApiClient, useMyTasks, useWorkflowMutations, type MyTask } from "@pdpa/api-client";
import { SlaBadge } from "@/components/sla-badge";
import { WorkflowPanel, localized } from "@/components/workflow-panel";

const COLUMNS = ["open", "in_progress"] as const;

export function TasksContent({ currentUserId }: { currentUserId: string }) {
  const t = useTranslations("workflow");
  const locale = useLocale() as Locale;
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const tasks = useMyTasks(client);
  const m = useWorkflowMutations(client);
  const [selected, setSelected] = useState<string>();

  return (
    <main className="mx-auto grid max-w-7xl gap-6 p-8 lg:grid-cols-[3fr_2fr]">
      <section className="space-y-3">
        <h1 className="text-xl font-semibold">{t("board.title")}</h1>
        {tasks.isPending ? (
          <p className="text-slate-500">{t("loading")}</p>
        ) : tasks.isError ? (
          <p className="text-red-700">{t("loadError")}</p>
        ) : tasks.data.length === 0 ? (
          <p className="text-slate-500">{t("board.empty")}</p>
        ) : (
          <div className="grid gap-4 md:grid-cols-2">
            {COLUMNS.map((col) => (
              <div key={col} className="space-y-2 rounded-md bg-slate-50 p-3" data-column={col}>
                <h2 className="text-sm font-semibold text-slate-600">{t(`board.${col}`)}</h2>
                {tasks.data.filter((x) => x.status === col).map((task) => (
                  <Card key={task.id} task={task} locale={locale} selected={task.instance_id === selected} onSelect={() => setSelected(task.instance_id)}
                    onClaim={() => m.updateTask.mutate({ task, assigneeUserId: currentUserId })}
                    onStart={() => m.updateTask.mutate({ task, status: "in_progress" })}
                    busy={m.updateTask.isPending} currentUserId={currentUserId} />
                ))}
              </div>
            ))}
          </div>
        )}
      </section>
      <aside className="rounded-md border border-slate-200 bg-slate-50/50 p-4">
        {selected ? <WorkflowPanel instanceId={selected} currentUserId={currentUserId} /> : <p className="text-sm text-slate-500">{t("panel.select")}</p>}
      </aside>
    </main>
  );
}

function Card({ task, locale, selected, onSelect, onClaim, onStart, busy, currentUserId }: {
  task: MyTask; locale: Locale; selected: boolean; onSelect: () => void; onClaim: () => void; onStart: () => void; busy: boolean; currentUserId: string;
}) {
  const t = useTranslations("workflow");
  return (
    <article className={`space-y-2 rounded-md border bg-white p-3 text-sm shadow-sm ${selected ? "border-slate-900" : "border-slate-200"}`}>
      <button className="block w-full text-left" onClick={onSelect}>
        <p className="font-medium">{localized(task.title, locale)}</p>
        <p className="text-xs text-slate-500">{task.workflow_name} · {localized(task.state_label, locale)}</p>
      </button>
      <div className="flex flex-wrap items-center gap-2">
        <SlaBadge status={task.sla_status} dueAt={task.sla_due_at} />
        {task.due_at && <span className="text-xs text-slate-500">{t("board.taskDue", { date: formatDate(task.due_at, locale, { month: "short" }) })}</span>}
      </div>
      {!task.assignee_user_id && task.group_name && (
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs text-slate-500">{t("board.group", { name: task.group_name })}</span>
          <Button variant="secondary" onClick={onClaim} disabled={busy}>{t("board.claim")}</Button>
        </div>
      )}
      {task.assignee_user_id === currentUserId && task.status === "open" && (
        <Button variant="secondary" onClick={onStart} disabled={busy}>{t("board.start")}</Button>
      )}
    </article>
  );
}
