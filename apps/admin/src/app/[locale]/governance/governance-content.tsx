"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useAssets,
  useRetentionSchedules,
  useCreateRetentionSchedule,
  useSetRetentionScheduleStatus,
  useDisposalJobs,
  useApproveDisposalJob,
  useStartDisposalJob,
  useCompleteDisposalJob,
  useReleaseDisposalJobHold,
  type RetentionScheduleStatus,
  type DisposalJobStatus,
  type StoredFile,
} from "@pdpa/api-client";
import { FileUploader } from "@/components/file-uploader";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

const JOB_STATUS_STYLE: Record<DisposalJobStatus, string> = {
  pending: "bg-slate-100 text-slate-700",
  approved: "bg-sky-100 text-sky-800",
  in_progress: "bg-amber-100 text-amber-800",
  done: "bg-emerald-100 text-emerald-800",
  on_hold: "bg-red-100 text-red-800",
};

export function GovernanceContent() {
  const t = useTranslations("governance");
  const canRead = usePermission("dpx.retention.read");
  const canCreate = usePermission("dpx.retention.create");
  const canApprove = usePermission("dpx.retention.approve");
  const canExecute = usePermission("dpx.retention.execute");
  const client = useMemo(() => createApiClient("/api/bff"), []);

  const assets = useAssets(client);
  const assetRows = assets.data?.pages.flatMap((p) => p.data) ?? [];
  const assetName = (id: string) => assetRows.find((a) => a.id === id)?.name ?? id;

  const [scheduleStatusFilter, setScheduleStatusFilter] = useState<RetentionScheduleStatus | "">("");
  const schedules = useRetentionSchedules(client, { status: scheduleStatusFilter || undefined });
  const saveSchedule = useCreateRetentionSchedule(client);
  const setScheduleStatus = useSetRetentionScheduleStatus(client);
  const [newSchedule, setNewSchedule] = useState<{ retention_rule_id: string; asset_id: string; next_due_at: string } | null>(null);

  const [jobStatusFilter, setJobStatusFilter] = useState<DisposalJobStatus | "">("");
  const jobs = useDisposalJobs(client, { status: jobStatusFilter || undefined });
  const approveJob = useApproveDisposalJob(client);
  const startJob = useStartDisposalJob(client);
  const completeJob = useCompleteDisposalJob(client);
  const releaseHold = useReleaseDisposalJobHold(client);
  const [completing, setCompleting] = useState<{ id: string; rowVersion: number; evidence?: StoredFile; recordCount: string } | null>(null);

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  const scheduleRows = schedules.data?.pages.flatMap((p) => p.data) ?? [];
  const jobRows = jobs.data?.pages.flatMap((p) => p.data) ?? [];

  return (
    <main className="mx-auto max-w-6xl space-y-8 p-8 text-sm">
      <header>
        <h1 className="text-xl font-semibold">{t("title")}</h1>
        <p className="text-slate-600">{t("intro")}</p>
      </header>

      <section className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-lg font-semibold">{t("schedules.title")}</h2>
          {canCreate && (
            <Button onClick={() => setNewSchedule({ retention_rule_id: "", asset_id: "", next_due_at: "" })} data-testid="new-schedule">
              {t("schedules.new")}
            </Button>
          )}
        </div>

        {newSchedule && (
          <fieldset className="grid gap-3 rounded-md border border-slate-200 bg-white p-4 sm:grid-cols-2">
            <legend className="px-1 font-semibold">{t("schedules.form.title")}</legend>
            <label><span className="block text-slate-600">{t("schedules.form.asset")}</span>
              <select className={INPUT} value={newSchedule.asset_id} onChange={(e) => setNewSchedule({ ...newSchedule, asset_id: e.target.value })}>
                <option value="">{t("schedules.form.none")}</option>
                {assetRows.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
              </select>
            </label>
            <label><span className="block text-slate-600">{t("schedules.form.retentionRuleId")}</span>
              <input className={INPUT} value={newSchedule.retention_rule_id}
                onChange={(e) => setNewSchedule({ ...newSchedule, retention_rule_id: e.target.value })}
                placeholder={t("schedules.form.retentionRuleIdHint")} /></label>
            <label><span className="block text-slate-600">{t("schedules.form.nextDueAt")}</span>
              <input type="date" className={INPUT} value={newSchedule.next_due_at}
                onChange={(e) => setNewSchedule({ ...newSchedule, next_due_at: e.target.value })} /></label>
            {saveSchedule.isError && <p className="text-red-700 sm:col-span-2" role="alert">{t("schedules.form.saveError", { detail: detail(saveSchedule.error) })}</p>}
            <div className="flex gap-2 sm:col-span-2">
              <Button
                disabled={saveSchedule.isPending || !newSchedule.asset_id || !newSchedule.retention_rule_id || !newSchedule.next_due_at}
                onClick={() =>
                  saveSchedule.mutate(
                    { asset_id: newSchedule.asset_id, retention_rule_id: newSchedule.retention_rule_id, next_due_at: newSchedule.next_due_at },
                    { onSuccess: () => setNewSchedule(null) },
                  )
                }
              >
                {t("schedules.form.save")}
              </Button>
              <Button variant="secondary" onClick={() => { saveSchedule.reset(); setNewSchedule(null); }}>{t("schedules.form.cancel")}</Button>
            </div>
          </fieldset>
        )}

        <div className="flex items-center gap-2">
          <span className="text-slate-600">{t("schedules.filterStatus")}</span>
          <select className={INPUT + " w-auto"} value={scheduleStatusFilter} onChange={(e) => setScheduleStatusFilter(e.target.value as RetentionScheduleStatus | "")}>
            <option value="">{t("schedules.allStatuses")}</option>
            {(["active", "paused", "retired"] as const).map((s) => <option key={s} value={s}>{t(`schedules.statuses.${s}`)}</option>)}
          </select>
        </div>

        <table className="w-full border-collapse text-left">
          <thead><tr className="border-b border-slate-200 text-slate-500">
            <th className="py-1">{t("schedules.table.asset")}</th>
            <th>{t("schedules.table.nextDueAt")}</th>
            <th>{t("schedules.table.status")}</th>
            <th></th>
          </tr></thead>
          <tbody>
            {scheduleRows.map((s) => (
              <tr key={s.id} className="border-b border-slate-100">
                <td className="py-1">{assetName(s.asset_id)}</td>
                <td>{s.next_due_at}</td>
                <td>{t(`schedules.statuses.${s.status}`)}</td>
                <td className="space-x-2 text-right">
                  {canCreate && s.status === "active" && (
                    <button className="text-sky-700 underline" onClick={() => setScheduleStatus.mutate({ id: s.id, rowVersion: s.row_version, status: "paused" })}>{t("schedules.pause")}</button>
                  )}
                  {canCreate && s.status === "paused" && (
                    <button className="text-sky-700 underline" onClick={() => setScheduleStatus.mutate({ id: s.id, rowVersion: s.row_version, status: "active" })}>{t("schedules.resume")}</button>
                  )}
                  {canCreate && s.status !== "retired" && (
                    <button className="text-red-700 underline" onClick={() => setScheduleStatus.mutate({ id: s.id, rowVersion: s.row_version, status: "retired" })}>{t("schedules.retire")}</button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {schedules.hasNextPage && <Button variant="secondary" onClick={() => schedules.fetchNextPage()}>{t("schedules.loadMore")}</Button>}
      </section>

      <section className="space-y-3">
        <h2 className="text-lg font-semibold">{t("jobs.title")}</h2>
        <p className="text-slate-600">{t("jobs.intro")}</p>

        <div className="flex items-center gap-2">
          <span className="text-slate-600">{t("jobs.filterStatus")}</span>
          <select className={INPUT + " w-auto"} value={jobStatusFilter} onChange={(e) => setJobStatusFilter(e.target.value as DisposalJobStatus | "")}>
            <option value="">{t("jobs.allStatuses")}</option>
            {(["pending", "approved", "in_progress", "done", "on_hold"] as const).map((s) => <option key={s} value={s}>{t(`jobs.statuses.${s}`)}</option>)}
          </select>
        </div>

        <table className="w-full border-collapse text-left">
          <thead><tr className="border-b border-slate-200 text-slate-500">
            <th className="py-1">{t("jobs.table.dueAt")}</th>
            <th>{t("jobs.table.method")}</th>
            <th>{t("jobs.table.status")}</th>
            <th></th>
          </tr></thead>
          <tbody>
            {jobRows.map((j) => (
              <>
                <tr key={j.id} className="border-b border-slate-100">
                  <td className="py-1">{j.due_at}</td>
                  <td>{j.method}</td>
                  <td><span className={`rounded px-1.5 py-0.5 ${JOB_STATUS_STYLE[j.status]}`}>{t(`jobs.statuses.${j.status}`)}</span></td>
                  <td className="space-x-2 text-right">
                    {canApprove && j.status === "pending" && (
                      <button className="text-sky-700 underline" onClick={() => approveJob.mutate({ id: j.id, rowVersion: j.row_version })}>{t("jobs.approve")}</button>
                    )}
                    {canExecute && j.status === "approved" && (
                      <button className="text-sky-700 underline" onClick={() => startJob.mutate({ id: j.id, rowVersion: j.row_version })}>{t("jobs.start")}</button>
                    )}
                    {canExecute && j.status === "in_progress" && (
                      <button className="text-sky-700 underline" onClick={() => setCompleting({ id: j.id, rowVersion: j.row_version, recordCount: "" })}>{t("jobs.complete")}</button>
                    )}
                    {canApprove && j.status === "on_hold" && (
                      <button className="text-amber-700 underline" onClick={() => releaseHold.mutate({ id: j.id, rowVersion: j.row_version })}>{t("jobs.releaseHold")}</button>
                    )}
                  </td>
                </tr>
                {completing?.id === j.id && (
                  <tr key={`${j.id}-complete`} className="border-b border-slate-100 bg-slate-50">
                    <td colSpan={4} className="p-3">
                      <div className="space-y-2">
                        <p className="font-medium">{t("jobs.completeForm.title")}</p>
                        <FileUploader onUploaded={(f) => setCompleting((c) => (c ? { ...c, evidence: f } : c))} />
                        {completing.evidence && <p className="text-emerald-700">{t("jobs.completeForm.evidenceAttached", { name: completing.evidence.file_name })}</p>}
                        <label className="block w-48"><span className="block text-slate-600">{t("jobs.completeForm.recordCount")}</span>
                          <input type="number" className={INPUT} value={completing.recordCount}
                            onChange={(e) => setCompleting({ ...completing, recordCount: e.target.value })} /></label>
                        {completeJob.isError && <p className="text-red-700" role="alert">{t("jobs.completeForm.saveError", { detail: detail(completeJob.error) })}</p>}
                        <div className="flex gap-2">
                          <Button
                            disabled={completeJob.isPending || !completing.evidence}
                            onClick={() =>
                              completeJob.mutate(
                                {
                                  id: completing.id, rowVersion: completing.rowVersion,
                                  input: { evidence_file_id: completing.evidence!.id, record_count: completing.recordCount ? Number(completing.recordCount) : undefined },
                                },
                                { onSuccess: () => setCompleting(null) },
                              )
                            }
                          >
                            {t("jobs.completeForm.save")}
                          </Button>
                          <Button variant="secondary" onClick={() => { completeJob.reset(); setCompleting(null); }}>{t("jobs.completeForm.cancel")}</Button>
                        </div>
                      </div>
                    </td>
                  </tr>
                )}
              </>
            ))}
          </tbody>
        </table>
        {jobs.hasNextPage && <Button variant="secondary" onClick={() => jobs.fetchNextPage()}>{t("jobs.loadMore")}</Button>}
      </section>
    </main>
  );
}
