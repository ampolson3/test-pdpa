"use client";

import { useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Button } from "@pdpa/ui";
import { formatDate, type Locale } from "@pdpa/i18n";
import { createApiClient, useWorkflowInstance, useWorkflowMutations, type LocalizedText, type WorkflowInstance, type WorkflowTask } from "@pdpa/api-client";
import { SlaBadge } from "./sla-badge";

export function localized(text: LocalizedText | undefined, locale: string): string {
  if (!text) return "";
  return (locale === "en" ? text.en : undefined) || text.th;
}

/**
 * One workflow (PLT-05): current step with its SLA badge, the moves the viewer may make (with a comment),
 * the tasks and the timeline. Any module shows its record's workflow with it.
 */
export function WorkflowPanel({ instanceId, currentUserId }: { instanceId: string; currentUserId: string }) {
  const t = useTranslations("workflow");
  const locale = useLocale() as Locale;
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const q = useWorkflowInstance(client, instanceId);
  const m = useWorkflowMutations(client);
  const [comment, setComment] = useState("");

  if (q.isPending) return <p className="text-slate-500">{t("loading")}</p>;
  if (q.isError) return <p className="text-red-700">{t("loadError")}</p>;
  const inst = q.data;
  const def = inst.workflow.definition;
  const stateLabel = (key: string) => localized(def.states.find((s) => s.key === key)?.label, locale) || key;
  const timer = inst.timers.find((x) => !x.stopped_at) ?? inst.timers[0];
  const date = (iso: string) => formatDate(iso, locale, { month: "short", hour: "2-digit", minute: "2-digit" });

  return (
    <div className="space-y-5 text-sm" data-testid="workflow-panel">
      <header className="space-y-1">
        <p className="text-xs text-slate-500">{inst.workflow.name} · {t("panel.version", { version: inst.workflow.version })}</p>
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-slate-600">{t("panel.state")}:</span>
          <span className="text-base font-semibold" data-testid="workflow-state">{stateLabel(inst.state)}</span>
          <SlaBadge status={inst.sla_status} dueAt={timer?.due_at} />
        </div>
        <p className="text-xs text-slate-500">
          {t("panel.started", { date: date(inst.started_at) })}
          {inst.completed_at && ` · ${t("panel.completed", { date: date(inst.completed_at) })}`}
        </p>
      </header>

      <section className="space-y-2 rounded-md border border-slate-200 bg-white p-3">
        <h3 className="font-medium">{t("panel.actions")}</h3>
        {inst.transitions.length === 0 ? (
          <p className="text-slate-500">{t("panel.noActions")}</p>
        ) : (
          <>
            <textarea aria-label={t("panel.comment")} placeholder={t("panel.comment")} className="h-16 w-full rounded-md border border-slate-300 px-2 py-1" value={comment} onChange={(e) => setComment(e.target.value)} />
            <div className="flex flex-wrap gap-2">
              {inst.transitions.map((tr) => (
                <Button key={tr.to} disabled={m.transition.isPending} onClick={() => m.transition.mutate({ instance: inst, to: tr.to, comment }, { onSuccess: () => setComment("") })}>
                  {localized(tr.label, locale) || stateLabel(tr.to)}
                </Button>
              ))}
            </div>
            {m.transition.isError && <p className="text-red-700" role="alert">{t("panel.moveError")}</p>}
          </>
        )}
      </section>

      <section className="space-y-2">
        <h3 className="font-medium">{t("panel.tasks")}</h3>
        <ul className="divide-y divide-slate-100 rounded-md border border-slate-200 bg-white">
          {inst.tasks.map((task) => (
            <TaskRow key={task.id} task={task} currentUserId={currentUserId} stateLabel={stateLabel} date={date} onClaim={() => m.updateTask.mutate({ task, assigneeUserId: currentUserId })}
              onStart={() => m.updateTask.mutate({ task, status: "in_progress" })} busy={m.updateTask.isPending} />
          ))}
        </ul>
      </section>

      {timer && timer.reminders.length > 0 && (
        <section className="space-y-1">
          <h3 className="font-medium">{t("panel.reminders")}</h3>
          <ul className="text-xs text-slate-600">
            {timer.reminders.map((r) => (
              <li key={r.at}>{date(r.at)} {r.sent && <span className="text-amber-700">· {t("panel.reminderSent")}</span>}</li>
            ))}
          </ul>
        </section>
      )}

      <section className="space-y-2">
        <h3 className="font-medium">{t("panel.timeline")}</h3>
        <ol className="space-y-2 border-l border-slate-200 pl-4" data-testid="workflow-timeline">
          {inst.history.map((h, i) => (
            <li key={i} className="relative">
              <span className="absolute -left-[21px] top-1.5 h-2 w-2 rounded-full bg-slate-400" />
              <p>{historyText(t, h, stateLabel)}</p>
              <p className="text-xs text-slate-500">{date(h.at)} · {h.actor_name || t("history.system")}</p>
            </li>
          ))}
        </ol>
      </section>
    </div>
  );
}

type History = WorkflowInstance["history"][number];

function historyText(t: ReturnType<typeof useTranslations<"workflow">>, h: History, stateLabel: (k: string) => string): string {
  const kind = h.action.replace(/^platform\.workflow\./, "");
  switch (kind) {
    case "transition":
      return t("history.transition", { from: stateLabel(String(h.before?.state ?? "")), to: stateLabel(String(h.after?.state ?? "")) });
    case "start":
    case "task_update":
    case "sla_reminder":
    case "sla_breached":
      return t(`history.${kind}`);
    default:
      return h.action;
  }
}

function TaskRow({ task, currentUserId, stateLabel, date, onClaim, onStart, busy }: {
  task: WorkflowTask; currentUserId: string; stateLabel: (k: string) => string; date: (iso: string) => string; onClaim: () => void; onStart: () => void; busy: boolean;
}) {
  const t = useTranslations("workflow");
  const locale = useLocale();
  const active = task.status === "open" || task.status === "in_progress";
  const who = task.assignee_name || (task.group_name ? t("board.group", { name: task.group_name }) : t("panel.unassigned"));
  return (
    <li className="flex flex-wrap items-center justify-between gap-2 px-3 py-2">
      <div>
        <p className="font-medium">{localized(task.title, locale)}</p>
        <p className="text-xs text-slate-500">
          {stateLabel(task.state)} · {who} · {t(`taskStatus.${task.status}`)}
          {task.due_at && ` · ${t("board.taskDue", { date: date(task.due_at) })}`}
          {task.outcome && ` ${t("panel.outcome", { state: stateLabel(task.outcome) })}`}
        </p>
        {task.comment && <p className="mt-1 whitespace-pre-wrap text-xs text-slate-700">“{task.comment}”</p>}
      </div>
      {active && !task.assignee_user_id && task.assignee_group_id && (
        <Button variant="secondary" onClick={onClaim} disabled={busy}>{t("board.claim")}</Button>
      )}
      {active && task.assignee_user_id === currentUserId && task.status === "open" && (
        <Button variant="secondary" onClick={onStart} disabled={busy}>{t("board.start")}</Button>
      )}
    </li>
  );
}
