"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useTasks,
  useAssignTask,
  useMentionSearch,
  type DpoRemediationTask,
  type DpoTaskStatus,
} from "@pdpa/api-client";
import { RecordCollaboration } from "@/components/record-collaboration";

const STATUSES: DpoTaskStatus[] = ["created", "assigned", "in_review", "done", "closed"];
const SOURCE_TYPES = ["ropa_gap", "dpia", "audit", "breach", "risk", "vendor", "agreement", "consent", "manual"] as const;
const PRIORITIES = ["low", "medium", "high", "urgent"] as const;

const INPUT = "rounded-md border border-slate-300 bg-white px-2 py-1";
const PRIORITY_STYLE: Record<string, string> = {
  low: "bg-slate-100 text-slate-700",
  medium: "bg-sky-100 text-sky-800",
  high: "bg-amber-100 text-amber-800",
  urgent: "bg-red-100 text-red-800",
};

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

/** DPO-06's own "link back": the module that opened the task is always on record (source_type), so every
 *  task at least names where it came from; where the id a module gave us is known to be a real record of
 *  an existing admin page (vendor id for an "agreement" task — DPA-12's own missing-DPA reminder), link
 *  straight there. Every other source_type's own source_id carries a different meaning per caller (a
 *  finding, an activity, an assessment, a risk, a consent purpose — see CLAUDE.md's DPO-06 note), so this
 *  deliberately shows provenance without guessing at a link it can't prove. */
function sourceLink(t: DpoRemediationTask): string | null {
  if (t.source_type === "agreement" && t.source_id) return `/vendors/${t.source_id}`;
  if (t.source_type === "ropa_gap") return "/settings/gap-register";
  if (t.source_type === "risk") return "/settings/risk-register";
  return null;
}

function AssigneePicker({ client, onPick }: { client: ReturnType<typeof createApiClient>; onPick: (u: { id: string; display_name: string }) => void }) {
  const t = useTranslations("dpoTasks");
  const [q, setQ] = useState("");
  const matches = useMentionSearch(client, q.length >= 2 ? q : null);
  return (
    <div className="relative inline-block">
      <input className={INPUT} value={q} onChange={(e) => setQ(e.target.value)} placeholder={t("assign.search")} />
      {q.length >= 2 && !!matches.data?.length && (
        <ul className="absolute z-10 w-48 rounded-md border border-slate-200 bg-white text-sm shadow">
          {matches.data.map((u) => (
            <li key={u.id}>
              <button type="button" className="block w-full px-3 py-1.5 text-left hover:bg-slate-50" onClick={() => { onPick(u); setQ(""); }}>
                {u.display_name}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function AssignForm({ client, task, onDone }: { client: ReturnType<typeof createApiClient>; task: DpoRemediationTask; onDone: () => void }) {
  const t = useTranslations("dpoTasks");
  const [assignee, setAssignee] = useState<{ id: string; display_name: string } | null>(null);
  const [reviewer, setReviewer] = useState<{ id: string; display_name: string } | null>(null);
  const [dueAt, setDueAt] = useState(task.due_at ?? "");
  const [priority, setPriority] = useState<(typeof PRIORITIES)[number]>((task.priority as (typeof PRIORITIES)[number]) ?? "medium");
  const assign = useAssignTask(client);

  const submit = () => {
    assign.mutate(
      {
        id: task.id,
        rowVersion: task.row_version,
        input: {
          assignee_user_id: assignee?.id ?? task.assignee_user_id,
          reviewer_user_id: reviewer?.id ?? task.reviewer_user_id,
          due_at: dueAt || undefined,
          priority,
        },
      },
      { onSuccess: onDone },
    );
  };

  return (
    <div className="flex flex-wrap items-center gap-2 py-2" data-testid="assign-form">
      <span className="text-xs text-slate-500">{t("assign.assignee")}</span>
      <AssigneePicker client={client} onPick={setAssignee} />
      {assignee && <span className="text-xs text-slate-600">{assignee.display_name}</span>}
      <span className="text-xs text-slate-500">{t("assign.reviewer")}</span>
      <AssigneePicker client={client} onPick={setReviewer} />
      {reviewer && <span className="text-xs text-slate-600">{reviewer.display_name}</span>}
      <input className={INPUT} type="date" value={dueAt} onChange={(e) => setDueAt(e.target.value)} />
      <select className={INPUT} value={priority} onChange={(e) => setPriority(e.target.value as typeof priority)}>
        {PRIORITIES.map((p) => (
          <option key={p} value={p}>{t(`priority.${p}`)}</option>
        ))}
      </select>
      <Button onClick={submit} disabled={assign.isPending}>{t("assign.submit")}</Button>
      {assign.isError && <span className="text-red-700" role="alert">{t("assign.error", { detail: detail(assign.error) })}</span>}
    </div>
  );
}

function TaskCard({
  client,
  task,
  canExecute,
  canUpdate,
  currentUserId,
}: {
  client: ReturnType<typeof createApiClient>;
  task: DpoRemediationTask;
  canExecute: boolean;
  canUpdate: boolean;
  currentUserId: string;
}) {
  const t = useTranslations("dpoTasks");
  const [assigning, setAssigning] = useState(false);
  const [discussing, setDiscussing] = useState(false);
  const link = sourceLink(task);
  return (
    <div className="rounded-md border border-slate-200 bg-white p-3 text-sm" data-testid="task-card">
      <div className="flex items-center justify-between">
        <span className="font-mono text-xs text-slate-500">{task.task_no}</span>
        <span className={`rounded px-2 py-0.5 text-xs font-semibold ${PRIORITY_STYLE[task.priority] ?? "bg-slate-100 text-slate-700"}`}>
          {t(`priority.${task.priority}`)}
        </span>
      </div>
      <p className="mt-1 font-medium">{task.title}</p>
      {task.description && <p className="mt-0.5 text-xs text-slate-600">{task.description}</p>}
      <p className="mt-1 text-xs text-slate-500">
        {t("provenance", { source: t(`sourceType.${task.source_type ?? "manual"}`) })}
        {link && (
          <>
            {" · "}
            <a className="text-sky-700 underline" href={link}>{t("viewSource")}</a>
          </>
        )}
      </p>
      {task.due_at && <p className="text-xs text-slate-500">{t("dueAt", { date: task.due_at })}</p>}
      {task.assignee_user_id && <p className="text-xs text-slate-500">{t("assigneeId", { id: task.assignee_user_id.slice(0, 8) })}</p>}
      {task.reviewer_user_id && <p className="text-xs text-slate-500">{t("reviewerId", { id: task.reviewer_user_id.slice(0, 8) })}</p>}
      <div className="mt-2 flex gap-3">
        {canExecute && (
          <button type="button" className="text-xs text-sky-700 underline" onClick={() => setAssigning((v) => !v)}>
            {t("assign.open")}
          </button>
        )}
        <button type="button" className="text-xs text-sky-700 underline" onClick={() => setDiscussing((v) => !v)}>
          {t("comments.open")}
        </button>
      </div>
      {assigning && <AssignForm client={client} task={task} onDone={() => setAssigning(false)} />}
      {discussing && (
        <div className="mt-2">
          <RecordCollaboration entityType="dpo_task" entityId={task.id} canWrite={canUpdate} currentUserId={currentUserId} />
        </div>
      )}
    </div>
  );
}

/** DPO-06's own task board (admin: /dpo/tasks): every dpo.tasks job every other feature auto-creates
 *  (RRA-07/DPO-09/DPA-12/PNG-07/ROPA-19/DPIA-08/RRA-08/RTG-10), in one kanban board grouped by status —
 *  the module description's own "kanban + รายการงาน" (a flat, filterable list is the same data, toggled). */
export function DpoTasksContent({ currentUserId }: { currentUserId: string }) {
  const t = useTranslations("dpoTasks");
  const canRead = usePermission("dpo.task.read");
  const canExecute = usePermission("dpo.task.execute");
  const canUpdate = usePermission("dpo.task.update");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const [view, setView] = useState<"kanban" | "list">("kanban");
  const [sourceType, setSourceType] = useState("");
  const [priority, setPriority] = useState("");
  const tasks = useTasks(client, { source_type: sourceType || undefined, priority: priority || undefined });

  if (!canRead) return <main className="mx-auto max-w-6xl p-8 text-slate-600">{t("forbidden")}</main>;

  const rows = (tasks.data?.pages ?? []).flatMap((p) => p.data);

  return (
    <main className="mx-auto max-w-6xl space-y-6 p-8 text-sm">
      <header className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">{t("title")}</h1>
          <p className="text-slate-600">{t("intro")}</p>
        </div>
        <div className="flex gap-2">
          <button type="button" className={`rounded-md border px-3 py-1 ${view === "kanban" ? "bg-slate-900 text-white" : "border-slate-300"}`} onClick={() => setView("kanban")}>
            {t("view.kanban")}
          </button>
          <button type="button" className={`rounded-md border px-3 py-1 ${view === "list" ? "bg-slate-900 text-white" : "border-slate-300"}`} onClick={() => setView("list")}>
            {t("view.list")}
          </button>
        </div>
      </header>

      <div className="flex flex-wrap items-center gap-2">
        <select className={INPUT} value={sourceType} onChange={(e) => setSourceType(e.target.value)}>
          <option value="">{t("filters.allSources")}</option>
          {SOURCE_TYPES.map((s) => (
            <option key={s} value={s}>{t(`sourceType.${s}`)}</option>
          ))}
        </select>
        <select className={INPUT} value={priority} onChange={(e) => setPriority(e.target.value)}>
          <option value="">{t("filters.allPriorities")}</option>
          {PRIORITIES.map((p) => (
            <option key={p} value={p}>{t(`priority.${p}`)}</option>
          ))}
        </select>
      </div>

      {tasks.isPending ? (
        <p className="text-slate-500">{t("loading")}</p>
      ) : tasks.isError ? (
        <p className="text-red-700">{t("loadError")}</p>
      ) : rows.length === 0 ? (
        <p className="text-slate-500">{t("empty")}</p>
      ) : view === "kanban" ? (
        <div className="grid gap-4 md:grid-cols-5" data-testid="task-kanban">
          {STATUSES.map((status) => (
            <div key={status} className="space-y-2">
              <h2 className="text-xs font-semibold uppercase text-slate-500">{t(`status.${status}`)}</h2>
              <div className="space-y-2">
                {rows.filter((r) => r.status === status).map((r) => (
                  <TaskCard key={r.id} client={client} task={r} canExecute={canExecute} canUpdate={canUpdate} currentUserId={currentUserId} />
                ))}
              </div>
            </div>
          ))}
        </div>
      ) : (
        <table className="w-full border-collapse text-left" data-testid="task-table">
          <thead>
            <tr className="border-b border-slate-200 text-slate-500">
              <th className="py-2">{t("table.taskNo")}</th>
              <th className="py-2">{t("table.title")}</th>
              <th className="py-2">{t("table.source")}</th>
              <th className="py-2">{t("table.status")}</th>
              <th className="py-2">{t("table.priority")}</th>
              <th className="py-2">{t("table.dueAt")}</th>
              <th className="py-2" />
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => {
              const link = sourceLink(r);
              return (
                <tr key={r.id} className="border-b border-slate-100">
                  <td className="py-2 font-mono text-xs">{r.task_no}</td>
                  <td className="py-2">{r.title}</td>
                  <td className="py-2">
                    {t(`sourceType.${r.source_type ?? "manual"}`)}
                    {link && (
                      <>
                        {" · "}
                        <a className="text-sky-700 underline" href={link}>{t("viewSource")}</a>
                      </>
                    )}
                  </td>
                  <td className="py-2">{t(`status.${r.status}`)}</td>
                  <td className="py-2">
                    <span className={`rounded px-2 py-0.5 text-xs font-semibold ${PRIORITY_STYLE[r.priority] ?? "bg-slate-100 text-slate-700"}`}>
                      {t(`priority.${r.priority}`)}
                    </span>
                  </td>
                  <td className="py-2">{r.due_at ?? "—"}</td>
                  <td className="py-2" />
                </tr>
              );
            })}
          </tbody>
        </table>
      )}

      {tasks.hasNextPage && (
        <Button onClick={() => tasks.fetchNextPage()} disabled={tasks.isFetchingNextPage}>
          {t("loadMore")}
        </Button>
      )}
    </main>
  );
}
