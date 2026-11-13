"use client";

import { Fragment, useMemo, useState } from "react";
import Link from "next/link";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useGapRules,
  useOpenGapFindings,
  useRemediateGapFinding,
  useMentionSearch,
  useTask,
  useUpdateTaskStatus,
  type DpoTaskStatus,
} from "@pdpa/api-client";

const SEVERITY_STYLE: Record<string, string> = {
  low: "bg-slate-100 text-slate-700",
  medium: "bg-amber-100 text-amber-800",
  high: "bg-red-100 text-red-800",
};

const INPUT = "rounded-md border border-slate-300 bg-white px-2 py-1";
const NEXT_STATUS: Partial<Record<DpoTaskStatus, DpoTaskStatus>> = {
  created: "assigned",
  assigned: "in_review",
  in_review: "done",
  done: "closed",
};

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

/** RRA-07's own assignee picker, the exact pattern VEN-01's own OwnerPicker already uses. */
function AssigneePicker({ client, onPick }: { client: ReturnType<typeof createApiClient>; onPick: (u: { id: string; display_name: string }) => void }) {
  const t = useTranslations("gapRegister");
  const [q, setQ] = useState("");
  const matches = useMentionSearch(client, q.length >= 2 ? q : null);
  return (
    <div className="relative inline-block">
      <input className={INPUT} value={q} onChange={(e) => setQ(e.target.value)} placeholder={t("remediate.assigneeSearch")} />
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

/** RRA-07's own remediate form: pick the assignee/due date/priority, then open the task. */
function RemediateForm({ client, findingId, activityId, onDone }: { client: ReturnType<typeof createApiClient>; findingId: string; activityId: string; onDone: () => void }) {
  const t = useTranslations("gapRegister");
  const [assignee, setAssignee] = useState<{ id: string; display_name: string } | null>(null);
  const [dueAt, setDueAt] = useState("");
  const [priority, setPriority] = useState<"low" | "medium" | "high" | "urgent">("medium");
  const remediate = useRemediateGapFinding(client);

  const submit = () => {
    remediate.mutate(
      { id: findingId, activityId, input: { assignee_user_id: assignee?.id, due_at: dueAt || undefined, priority } },
      { onSuccess: onDone },
    );
  };

  return (
    <div className="flex flex-wrap items-center gap-2 py-2" data-testid="remediate-form">
      <AssigneePicker client={client} onPick={setAssignee} />
      {assignee && <span className="text-xs text-slate-600">{assignee.display_name}</span>}
      <input className={INPUT} type="date" value={dueAt} onChange={(e) => setDueAt(e.target.value)} />
      <select className={INPUT} value={priority} onChange={(e) => setPriority(e.target.value as typeof priority)}>
        {(["low", "medium", "high", "urgent"] as const).map((p) => (
          <option key={p} value={p}>{t(`remediate.priority.${p}`)}</option>
        ))}
      </select>
      <Button onClick={submit} disabled={remediate.isPending}>{t("remediate.submit")}</Button>
      {remediate.isError && <span className="text-red-700" role="alert">{t("remediate.error", { detail: detail(remediate.error) })}</span>}
    </div>
  );
}

/** RRA-07's own "ติดตามจนปิด": shows the linked task's live status and a button to advance it one step,
 *  ending in "ปิดงาน" — the backend re-checks the rule right then, and the register refreshes to show
 *  whether the gap actually cleared. */
function TaskStatusControl({ client, taskId }: { client: ReturnType<typeof createApiClient>; taskId: string }) {
  const t = useTranslations("gapRegister");
  const canUpdate = usePermission("dpo.task.update");
  const task = useTask(client, taskId);
  const update = useUpdateTaskStatus(client);

  if (task.isPending) return <span className="text-slate-500">{t("task.loading")}</span>;
  if (task.isError || !task.data) return <span className="text-red-700">{t("task.loadError")}</span>;

  const next = NEXT_STATUS[task.data.status];
  return (
    <span className="flex items-center gap-2">
      <span className="rounded bg-slate-100 px-2 py-0.5 text-xs font-semibold text-slate-700">{t(`task.status.${task.data.status}`)}</span>
      {canUpdate && next && (
        <button
          type="button"
          className="text-sky-700 underline"
          disabled={update.isPending}
          onClick={() => update.mutate({ id: taskId, status: next, rowVersion: task.data!.row_version })}
        >
          {next === "closed" ? t("task.close") : t("task.advance")}
        </button>
      )}
      {update.isError && <span className="text-red-700" role="alert">{t("task.updateError", { detail: detail(update.error) })}</span>}
    </span>
  );
}

/** RRA-07's own register screen: every open legal-gap finding across every processing activity,
 *  from the last analysis run on each one, each linking straight to the activity that has it —
 *  the backlog's own "รายการช่องว่าง + ลิงก์ไปแก้" — plus a button to open a tracked remediation task
 *  against a finding (assignee, due date, priority) and track it through to close. Re-running the
 *  analysis itself happens on the activity page (GapAnalysisSection); this page is read-only on that. */
export function GapRegisterContent() {
  const t = useTranslations("gapRegister");
  const canRead = usePermission("ropa.risk.read");
  const canCreate = usePermission("ropa.risk.create");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const findings = useOpenGapFindings(client);
  const rules = useGapRules(client);
  const [remediating, setRemediating] = useState<string | null>(null);

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  const ruleByCode = new Map((rules.data ?? []).map((r) => [r.code, r]));
  const rows = (findings.data ?? []).filter((f) => f.status === "open");

  return (
    <main className="mx-auto max-w-5xl space-y-6 p-8 text-sm">
      <header>
        <h1 className="text-xl font-semibold">{t("title")}</h1>
        <p className="text-slate-600">{t("intro")}</p>
      </header>

      {findings.isPending ? (
        <p className="text-slate-500">{t("loading")}</p>
      ) : findings.isError ? (
        <p className="text-red-700">{t("loadError")}</p>
      ) : rows.length === 0 ? (
        <p className="text-slate-500">{t("empty")}</p>
      ) : (
        <table className="w-full border-collapse text-left" data-testid="gap-register-table">
          <thead>
            <tr className="border-b border-slate-200 text-slate-500">
              <th className="py-2">{t("table.rule")}</th>
              <th className="py-2">{t("table.severity")}</th>
              <th className="py-2">{t("table.detectedAt")}</th>
              <th className="py-2">{t("table.task")}</th>
              <th className="py-2" />
            </tr>
          </thead>
          <tbody>
            {rows.map((f) => {
              const rule = ruleByCode.get(f.rule_code);
              return (
                <Fragment key={f.id}>
                  <tr className="border-b border-slate-100">
                    <td className="py-2">{rule?.name ?? f.rule_code}</td>
                    <td className="py-2">
                      <span className={`rounded px-2 py-0.5 text-xs font-semibold ${SEVERITY_STYLE[rule?.severity ?? ""] ?? "bg-slate-100 text-slate-700"}`}>
                        {rule ? t(`severity.${rule.severity}`) : f.status}
                      </span>
                    </td>
                    <td className="py-2">{new Date(f.detected_at).toLocaleString()}</td>
                    <td className="py-2">
                      {f.task_id ? (
                        <TaskStatusControl client={client} taskId={f.task_id} />
                      ) : canCreate ? (
                        <button type="button" className="text-sky-700 underline" onClick={() => setRemediating(remediating === f.id ? null : f.id)}>
                          {t("remediate.open")}
                        </button>
                      ) : null}
                    </td>
                    <td className="py-2">
                      <Link className="text-sky-700 underline" href={`/ropa/activities/${f.activity_id}`}>
                        {t("viewActivity")}
                      </Link>
                    </td>
                  </tr>
                  {remediating === f.id && (
                    <tr className="border-b border-slate-100 bg-slate-50">
                      <td colSpan={5} className="px-2">
                        <RemediateForm client={client} findingId={f.id} activityId={f.activity_id} onDone={() => setRemediating(null)} />
                      </td>
                    </tr>
                  )}
                </Fragment>
              );
            })}
          </tbody>
        </table>
      )}
    </main>
  );
}
