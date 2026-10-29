"use client";

import { Fragment, useMemo, useState } from "react";
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
  usePolicies,
  usePublishPolicy,
  useAttestPolicy,
  usePolicyAttestations,
  useMissingAttestations,
  useRegulatorLetters,
  useCreateRegulatorLetter,
  useSetRegulatorLetterStatus,
  useDeleteRegulatorLetter,
  type RetentionScheduleStatus,
  type DisposalJobStatus,
  type StoredFile,
  type GovPolicy,
  type GovPolicyAttestation,
  type GovMissingAttestation,
  type GovRegulatorLetter,
  type GovRegulatorLetterStatus,
  type GovRegulatorLetterDirection,
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
  const canReadPolicy = usePermission("dpx.policy.read");
  const canPublishPolicy = usePermission("dpx.policy.publish");
  const canAttest = usePermission("dpx.policy.execute");
  const canReadLetters = usePermission("dpx.regulator.read");
  const canCreateLetter = usePermission("dpx.regulator.create");
  const canUpdateLetter = usePermission("dpx.regulator.update");
  const canDeleteLetter = usePermission("dpx.regulator.delete");
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

  const policies = usePolicies(client);
  const publishPolicy = usePublishPolicy(client);
  const attestPolicy = useAttestPolicy(client);
  const [newPolicyDocId, setNewPolicyDocId] = useState("");
  const [expandedPolicy, setExpandedPolicy] = useState<string | null>(null);
  const attestations = usePolicyAttestations(client, expandedPolicy ?? "");
  const missing = useMissingAttestations(client, expandedPolicy ?? "");

  const [letterStatusFilter, setLetterStatusFilter] = useState<GovRegulatorLetterStatus | "">("");
  const [letterDirectionFilter, setLetterDirectionFilter] = useState<GovRegulatorLetterDirection | "">("");
  const letters = useRegulatorLetters(client, { status: letterStatusFilter || undefined, direction: letterDirectionFilter || undefined });
  const saveLetter = useCreateRegulatorLetter(client);
  const setLetterStatus = useSetRegulatorLetterStatus(client);
  const deleteLetter = useDeleteRegulatorLetter(client);
  const [newLetter, setNewLetter] = useState<{ direction: GovRegulatorLetterDirection; letter_no: string; subject: string; received_at: string; due_at: string; incident_id: string } | null>(null);

  if (!canRead && !canReadPolicy && !canReadLetters) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  const scheduleRows = schedules.data?.pages.flatMap((p) => p.data) ?? [];
  const jobRows = jobs.data?.pages.flatMap((p) => p.data) ?? [];

  return (
    <main className="mx-auto max-w-6xl space-y-8 p-8 text-sm">
      <header>
        <h1 className="text-xl font-semibold">{t("title")}</h1>
        <p className="text-slate-600">{t("intro")}</p>
      </header>

      {canRead && <section className="space-y-3">
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
      </section>}

      {canRead && <section className="space-y-3">
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
      </section>}

      {canReadPolicy && <section className="space-y-3" data-testid="policies-section">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-lg font-semibold">{t("policies.title")}</h2>
          <p className="text-slate-600">{t("policies.intro")}</p>
        </div>

        {canPublishPolicy && (
          <fieldset className="flex flex-wrap items-end gap-3 rounded-md border border-slate-200 bg-white p-4">
            <label className="flex-1"><span className="block text-slate-600">{t("policies.form.documentId")}</span>
              <input className={INPUT} value={newPolicyDocId} onChange={(e) => setNewPolicyDocId(e.target.value)}
                placeholder={t("policies.form.documentIdHint")} /></label>
            {publishPolicy.isError && <p className="text-red-700" role="alert">{t("policies.form.saveError", { detail: detail(publishPolicy.error) })}</p>}
            <Button
              disabled={publishPolicy.isPending || !newPolicyDocId}
              onClick={() => publishPolicy.mutate({ document_id: newPolicyDocId, requires_attestation: true }, { onSuccess: () => setNewPolicyDocId("") })}
            >
              {t("policies.form.save")}
            </Button>
          </fieldset>
        )}

        <table className="w-full border-collapse text-left">
          <thead><tr className="border-b border-slate-200 text-slate-500">
            <th className="py-1">{t("policies.table.title")}</th>
            <th>{t("policies.table.versionNo")}</th>
            <th></th>
          </tr></thead>
          <tbody>
            {(policies.data ?? []).map((p: GovPolicy) => (
              <Fragment key={p.id}>
                <tr className="border-b border-slate-100">
                  <td className="py-1">{p.title}</td>
                  <td>{p.version_no}</td>
                  <td className="space-x-2 text-right">
                    {canAttest && (
                      <button className="text-sky-700 underline" onClick={() => attestPolicy.mutate(p.id)}>{t("policies.attest")}</button>
                    )}
                    <button className="text-slate-700 underline" onClick={() => setExpandedPolicy(expandedPolicy === p.id ? null : p.id)}>
                      {t("policies.toggleDetail")}
                    </button>
                  </td>
                </tr>
                {expandedPolicy === p.id && (
                  <tr className="border-b border-slate-100 bg-slate-50">
                    <td colSpan={3} className="space-y-3 p-3">
                      <div>
                        <p className="font-medium">{t("policies.missing.title")}</p>
                        <ul className="list-inside list-disc">
                          {(missing.data ?? []).map((m: GovMissingAttestation) => <li key={m.user_id}>{m.user_name}</li>)}
                          {missing.data?.length === 0 && <li className="list-none text-emerald-700">{t("policies.missing.none")}</li>}
                        </ul>
                      </div>
                      <div>
                        <p className="font-medium">{t("policies.attestations.title")}</p>
                        <ul className="list-inside list-disc">
                          {(attestations.data ?? []).map((a: GovPolicyAttestation) => (
                            <li key={a.id}>{a.user_name ?? a.user_id} — {new Date(a.attested_at).toLocaleString()}</li>
                          ))}
                        </ul>
                      </div>
                    </td>
                  </tr>
                )}
              </Fragment>
            ))}
          </tbody>
        </table>
      </section>}

      {canReadLetters && <section className="space-y-3" data-testid="regulator-letters-section">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-lg font-semibold">{t("regulatorLetters.title")}</h2>
          {canCreateLetter && (
            <Button
              onClick={() => setNewLetter({ direction: "inbound", letter_no: "", subject: "", received_at: "", due_at: "", incident_id: "" })}
              data-testid="new-regulator-letter"
            >
              {t("regulatorLetters.new")}
            </Button>
          )}
        </div>
        <p className="text-slate-600">{t("regulatorLetters.intro")}</p>

        {newLetter && (
          <fieldset className="grid gap-3 rounded-md border border-slate-200 bg-white p-4 sm:grid-cols-2">
            <legend className="px-1 font-semibold">{t("regulatorLetters.form.title")}</legend>
            <label><span className="block text-slate-600">{t("regulatorLetters.form.direction")}</span>
              <select className={INPUT} value={newLetter.direction}
                onChange={(e) => setNewLetter({ ...newLetter, direction: e.target.value as GovRegulatorLetterDirection })}>
                <option value="inbound">{t("regulatorLetters.directions.inbound")}</option>
                <option value="outbound">{t("regulatorLetters.directions.outbound")}</option>
              </select></label>
            <label><span className="block text-slate-600">{t("regulatorLetters.form.letterNo")}</span>
              <input className={INPUT} value={newLetter.letter_no} onChange={(e) => setNewLetter({ ...newLetter, letter_no: e.target.value })} /></label>
            <label className="sm:col-span-2"><span className="block text-slate-600">{t("regulatorLetters.form.subject")}</span>
              <input className={INPUT} value={newLetter.subject} onChange={(e) => setNewLetter({ ...newLetter, subject: e.target.value })} /></label>
            <label><span className="block text-slate-600">{t("regulatorLetters.form.receivedAt")}</span>
              <input type="date" className={INPUT} value={newLetter.received_at} onChange={(e) => setNewLetter({ ...newLetter, received_at: e.target.value })} /></label>
            <label><span className="block text-slate-600">{t("regulatorLetters.form.dueAt")}</span>
              <input type="date" className={INPUT} value={newLetter.due_at} onChange={(e) => setNewLetter({ ...newLetter, due_at: e.target.value })} /></label>
            <label className="sm:col-span-2"><span className="block text-slate-600">{t("regulatorLetters.form.incidentId")}</span>
              <input className={INPUT} value={newLetter.incident_id} onChange={(e) => setNewLetter({ ...newLetter, incident_id: e.target.value })} /></label>
            {saveLetter.isError && <p className="text-red-700 sm:col-span-2" role="alert">{t("regulatorLetters.form.saveError", { detail: detail(saveLetter.error) })}</p>}
            <div className="flex gap-2 sm:col-span-2">
              <Button
                disabled={saveLetter.isPending || !newLetter.subject}
                onClick={() =>
                  saveLetter.mutate(
                    {
                      direction: newLetter.direction, letter_no: newLetter.letter_no || undefined, subject: newLetter.subject,
                      received_at: newLetter.received_at || undefined, due_at: newLetter.due_at || undefined,
                      incident_id: newLetter.incident_id || undefined,
                    },
                    { onSuccess: () => setNewLetter(null) },
                  )
                }
              >
                {t("regulatorLetters.form.save")}
              </Button>
              <Button variant="secondary" onClick={() => { saveLetter.reset(); setNewLetter(null); }}>{t("regulatorLetters.form.cancel")}</Button>
            </div>
          </fieldset>
        )}

        <div className="flex flex-wrap items-center gap-4">
          <span className="flex items-center gap-2"><span className="text-slate-600">{t("regulatorLetters.filterStatus")}</span>
            <select className={INPUT + " w-auto"} value={letterStatusFilter} onChange={(e) => setLetterStatusFilter(e.target.value as GovRegulatorLetterStatus | "")}>
              <option value="">{t("regulatorLetters.allStatuses")}</option>
              {(["open", "responded", "closed"] as const).map((s) => <option key={s} value={s}>{t(`regulatorLetters.statuses.${s}`)}</option>)}
            </select></span>
          <span className="flex items-center gap-2"><span className="text-slate-600">{t("regulatorLetters.filterDirection")}</span>
            <select className={INPUT + " w-auto"} value={letterDirectionFilter} onChange={(e) => setLetterDirectionFilter(e.target.value as GovRegulatorLetterDirection | "")}>
              <option value="">{t("regulatorLetters.allDirections")}</option>
              {(["inbound", "outbound"] as const).map((d) => <option key={d} value={d}>{t(`regulatorLetters.directions.${d}`)}</option>)}
            </select></span>
        </div>

        <table className="w-full border-collapse text-left">
          <thead><tr className="border-b border-slate-200 text-slate-500">
            <th className="py-1">{t("regulatorLetters.table.subject")}</th>
            <th>{t("regulatorLetters.table.direction")}</th>
            <th>{t("regulatorLetters.table.dueAt")}</th>
            <th>{t("regulatorLetters.table.status")}</th>
            <th></th>
          </tr></thead>
          <tbody>
            {(letters.data ?? []).map((l: GovRegulatorLetter) => (
              <tr key={l.id} className="border-b border-slate-100">
                <td className="py-1">{l.subject}</td>
                <td>{t(`regulatorLetters.directions.${l.direction}`)}</td>
                <td>{l.due_at ?? "—"}</td>
                <td>{t(`regulatorLetters.statuses.${l.status}`)}</td>
                <td className="space-x-2 text-right">
                  {canUpdateLetter && l.status === "open" && (
                    <button className="text-sky-700 underline" onClick={() => setLetterStatus.mutate({ id: l.id, rowVersion: l.row_version, status: "responded" })}>
                      {t("regulatorLetters.respond")}
                    </button>
                  )}
                  {canUpdateLetter && l.status !== "closed" && (
                    <button className="text-sky-700 underline" onClick={() => setLetterStatus.mutate({ id: l.id, rowVersion: l.row_version, status: "closed" })}>
                      {t("regulatorLetters.close")}
                    </button>
                  )}
                  {canDeleteLetter && (
                    <button className="text-red-700 underline" onClick={() => deleteLetter.mutate(l.id)}>{t("regulatorLetters.delete")}</button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>}
    </main>
  );
}
