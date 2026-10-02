"use client";

import { useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { formatDate, type Locale } from "@pdpa/i18n";
import {
  createApiClient,
  useActivities,
  useDsarRequests,
  useDsarRequestTypes,
  useDsarRequestMutations,
  useDsarVerifications,
  useDsarVerificationMutations,
  useLegalEntities,
  useMentionSearch,
  type DsarRequest,
  type DsarRequestStatus,
  type DsarRequestChannel,
  type DsarContactKind,
  type DsarOutcome,
  type DsarSlaStatus,
  type DsarVerificationMethod,
} from "@pdpa/api-client";
import { Link } from "@/i18n/routing";
import { RecordCollaboration } from "@/components/record-collaboration";
import { FileUploader } from "@/components/file-uploader";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";
const CHANNELS: DsarRequestChannel[] = ["web", "email", "phone", "branch", "letter", "line", "api"];
const CONTACT_KINDS: DsarContactKind[] = ["email", "phone", "national_id", "customer_id", "passport", "line_uid", "other"];
const STATUSES: DsarRequestStatus[] = ["received", "verifying", "in_review", "in_progress", "awaiting_info", "completed", "rejected", "withdrawn"];
const NEXT_STEPS: Record<DsarRequestStatus, DsarRequestStatus[]> = {
  received: ["verifying", "withdrawn"],
  verifying: ["in_review", "awaiting_info", "withdrawn"],
  in_review: ["in_progress", "awaiting_info", "rejected", "withdrawn"],
  in_progress: ["completed", "awaiting_info", "withdrawn"],
  awaiting_info: ["in_review", "rejected", "withdrawn"],
  completed: [],
  rejected: [],
  withdrawn: [],
};

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

const STATUS_STYLE: Record<DsarRequestStatus, string> = {
  received: "bg-slate-100 text-slate-700",
  verifying: "bg-sky-100 text-sky-800",
  in_review: "bg-sky-100 text-sky-800",
  in_progress: "bg-amber-100 text-amber-800",
  awaiting_info: "bg-amber-100 text-amber-800",
  completed: "bg-emerald-100 text-emerald-800",
  rejected: "bg-red-100 text-red-800",
  withdrawn: "bg-slate-100 text-slate-500",
};

const SLA_STYLE: Record<DsarSlaStatus, string> = {
  on_track: "bg-emerald-100 text-emerald-800",
  at_risk: "bg-amber-100 text-amber-800",
  overdue: "bg-red-100 text-red-800",
};

function AssigneePicker({ client, onPick }: { client: ReturnType<typeof createApiClient>; onPick: (u: { id: string; display_name: string }) => void }) {
  const t = useTranslations("dsarRequests");
  const [q, setQ] = useState("");
  const matches = useMentionSearch(client, q.length >= 2 ? q : null);
  return (
    <div className="relative inline-block">
      <input className={INPUT + " w-56"} value={q} onChange={(e) => setQ(e.target.value)} placeholder={t("form.assigneeSearch")} />
      {q.length >= 2 && !!matches.data?.length && (
        <ul className="absolute z-10 w-56 rounded-md border border-slate-200 bg-white text-sm shadow">
          {matches.data.map((u) => (
            <li key={u.id}>
              <button type="button" className="block w-full px-3 py-1.5 text-left hover:bg-slate-50" onClick={() => { onPick(u); setQ(""); }}>{u.display_name}</button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

/** DSAR-06: OTP or a redacted ID-card copy, then staff's own pass/fail on the latter. */
function VerificationPanel({ client, request, canUpdate }: { client: ReturnType<typeof createApiClient>; request: DsarRequest; canUpdate: boolean }) {
  const t = useTranslations("dsarRequests");
  const list = useDsarVerifications(client, request.id);
  const m = useDsarVerificationMutations(client);
  const [method, setMethod] = useState<DsarVerificationMethod>("otp_email");
  const [codeByVerification, setCodeByVerification] = useState<Record<string, string>>({});
  const [rawFileId, setRawFileId] = useState<string | null>(null);
  const [rect, setRect] = useState({ x: 0, y: 0, width: 100, height: 40 });

  const rows = list.data ?? [];
  const pendingId = rows.find((v) => v.status === "pending")?.id ?? null;

  return (
    <div className="space-y-3" data-testid="verification-panel">
      {rows.length > 0 && (
        <table className="w-full text-xs">
          <thead className="text-left text-slate-500">
            <tr><th className="py-1">{t("verify.method")}</th><th className="py-1">{t("verify.status")}</th><th className="py-1">{t("verify.verifiedAt")}</th></tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.map((v) => (
              <tr key={v.id}>
                <td className="py-1">{t(`verify.methods.${v.method}`)}</td>
                <td className="py-1">{t(`verify.statuses.${v.status}`)}</td>
                <td className="py-1">{v.verified_at ? new Date(v.verified_at).toLocaleString() : "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {canUpdate && (
        <div className="grid gap-3 sm:grid-cols-2">
          <fieldset className="space-y-2 rounded-md border border-slate-200 p-3">
            <legend className="px-1 text-slate-600">{t("verify.otpTitle")}</legend>
            <select className={INPUT} value={method} onChange={(e) => setMethod(e.target.value as DsarVerificationMethod)}>
              <option value="otp_email">{t("verify.methods.otp_email")}</option>
              <option value="otp_sms">{t("verify.methods.otp_sms")}</option>
            </select>
            <Button onClick={() => m.startOtp.mutate({ requestId: request.id, method: method as "otp_sms" | "otp_email" })} disabled={m.startOtp.isPending} data-testid="start-otp">
              {t("verify.sendOtp")}
            </Button>
            {pendingId && (
              <div className="flex items-center gap-2">
                <input className={INPUT} placeholder={t("verify.codePlaceholder")} value={codeByVerification[pendingId] ?? ""}
                  onChange={(e) => setCodeByVerification({ ...codeByVerification, [pendingId]: e.target.value })} data-testid="otp-code-input" />
                <Button onClick={() => m.confirmOtp.mutate({ requestId: request.id, verificationId: pendingId, code: codeByVerification[pendingId] ?? "" })}
                  disabled={m.confirmOtp.isPending} data-testid="confirm-otp">
                  {t("verify.confirm")}
                </Button>
              </div>
            )}
            {m.startOtp.isError && <p className="text-red-700" role="alert">{t("form.saveError", { detail: detail(m.startOtp.error) })}</p>}
            {m.confirmOtp.isError && <p className="text-red-700" role="alert">{t("form.saveError", { detail: detail(m.confirmOtp.error) })}</p>}
          </fieldset>

          <fieldset className="space-y-2 rounded-md border border-slate-200 p-3">
            <legend className="px-1 text-slate-600">{t("verify.idDocumentTitle")}</legend>
            <p className="text-xs text-slate-500">{t("verify.idDocumentHint")}</p>
            <FileUploader accept=".png,.jpg,.jpeg" onUploaded={(f) => setRawFileId(f.id)} />
            <div className="grid grid-cols-4 gap-1 text-xs">
              {(["x", "y", "width", "height"] as const).map((k) => (
                <label key={k}><span className="block text-slate-500">{t(`verify.rect.${k}`)}</span>
                  <input type="number" className={INPUT} value={rect[k]} min={0}
                    onChange={(e) => setRect({ ...rect, [k]: Number(e.target.value) })} /></label>
              ))}
            </div>
            <Button
              onClick={() => rawFileId && m.submitIdDocument.mutate({ requestId: request.id, rawFileId, redactions: [rect] }, { onSuccess: () => setRawFileId(null) })}
              disabled={!rawFileId || m.submitIdDocument.isPending} data-testid="submit-id-document">
              {t("verify.submitIdDocument")}
            </Button>
            {m.submitIdDocument.isError && <p className="text-red-700" role="alert">{t("form.saveError", { detail: detail(m.submitIdDocument.error) })}</p>}
            {pendingId && rows.find((v) => v.id === pendingId)?.method === "id_document" && (
              <div className="flex gap-2">
                <Button onClick={() => m.decide.mutate({ requestId: request.id, verificationId: pendingId, pass: true })} disabled={m.decide.isPending} data-testid="decide-pass">
                  {t("verify.pass")}
                </Button>
                <Button variant="secondary" onClick={() => m.decide.mutate({ requestId: request.id, verificationId: pendingId, pass: false })} disabled={m.decide.isPending} data-testid="decide-fail">
                  {t("verify.fail")}
                </Button>
              </div>
            )}
          </fieldset>
        </div>
      )}
    </div>
  );
}

export function RequestsContent({ currentUserId }: { currentUserId: string }) {
  const t = useTranslations("dsarRequests");
  const locale = useLocale() as Locale;
  const canRead = usePermission("dsar.request.read");
  const canCreate = usePermission("dsar.request.create");
  const canExecute = usePermission("dsar.request.execute");
  const canUpdate = usePermission("dsar.request.update");
  const client = useMemo(() => createApiClient("/api/bff"), []);

  const [statusFilter, setStatusFilter] = useState<DsarRequestStatus | "">("");
  const [search, setSearch] = useState("");
  const [historyId, setHistoryId] = useState<string | null>(null);
  const [verifyId, setVerifyId] = useState<string | null>(null);
  const list = useDsarRequests(client, { status: statusFilter || undefined, search: search || undefined });
  const types = useDsarRequestTypes(client);
  const entities = useLegalEntities(client);
  const activities = useActivities(client, {});
  const m = useDsarRequestMutations(client);

  const [draft, setDraft] = useState<{ request_type_id: string; legal_entity_id: string; channel: DsarRequestChannel | ""; requester_name: string; requester_contact: string; contact_kind: DsarContactKind } | null>(null);
  const [actionId, setActionId] = useState<string | null>(null);
  const [assignId, setAssignId] = useState<string | null>(null);
  const [actionDraft, setActionDraft] = useState<{ to: DsarRequestStatus | ""; outcome: DsarOutcome | ""; rejection_reason_code: string; activity_ids: string[] }>({ to: "", outcome: "", rejection_reason_code: "", activity_ids: [] });

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  const rows = list.data?.pages.flatMap((p) => p.data) ?? [];
  const typeRows = types.data ?? [];
  const entityRows = entities.data ?? [];
  const activityRows = activities.data?.pages.flatMap((p) => p.data) ?? [];
  const typeName = (id: string) => typeRows.find((rt) => rt.id === id)?.name_th ?? id;

  const create = () => {
    if (!draft || !draft.request_type_id || !draft.legal_entity_id || !draft.channel || !draft.requester_name || !draft.requester_contact) return;
    m.create.mutate(
      {
        request_type_id: draft.request_type_id, legal_entity_id: draft.legal_entity_id, channel: draft.channel, on_behalf: false,
        requester_name: draft.requester_name, requester_contact: draft.requester_contact, contact_kind: draft.contact_kind,
      },
      { onSuccess: () => setDraft(null) },
    );
  };

  const submitAction = (row: DsarRequest) => {
    if (!actionDraft.to) return;
    m.transition.mutate(
      {
        request: row,
        input: {
          to: actionDraft.to, outcome: actionDraft.outcome || undefined, rejection_reason_code: actionDraft.rejection_reason_code || undefined,
          activity_ids: actionDraft.to === "rejected" ? actionDraft.activity_ids : [],
        },
      },
      { onSuccess: () => { setActionId(null); setActionDraft({ to: "", outcome: "", rejection_reason_code: "", activity_ids: [] }); } },
    );
  };

  const toggleActionActivity = (id: string) => {
    const has = actionDraft.activity_ids.includes(id);
    setActionDraft({ ...actionDraft, activity_ids: has ? actionDraft.activity_ids.filter((x) => x !== id) : [...actionDraft.activity_ids, id] });
  };

  return (
    <main className="mx-auto max-w-6xl space-y-6 p-8 text-sm">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{t("title")}</h1>
          <p className="text-slate-600">{t("intro")}</p>
        </div>
        {canCreate && <Button onClick={() => setDraft({ request_type_id: "", legal_entity_id: "", channel: "", requester_name: "", requester_contact: "", contact_kind: "email" })} data-testid="new-request">{t("newRequest")}</Button>}
      </header>

      {draft && (
        <fieldset className="grid gap-3 rounded-md border border-slate-200 bg-white p-4 sm:grid-cols-3" disabled={!canCreate}>
          <legend className="px-1 font-semibold">{t("form.title")}</legend>
          <label><span className="block text-slate-600">{t("form.requestType")}</span>
            <select className={INPUT} value={draft.request_type_id} onChange={(e) => setDraft({ ...draft, request_type_id: e.target.value })}>
              <option value="">{t("form.choose")}</option>
              {typeRows.map((rt) => <option key={rt.id} value={rt.id}>{rt.name_th}</option>)}
            </select>
          </label>
          <label><span className="block text-slate-600">{t("form.legalEntity")}</span>
            <select className={INPUT} value={draft.legal_entity_id} onChange={(e) => setDraft({ ...draft, legal_entity_id: e.target.value })}>
              <option value="">{t("form.choose")}</option>
              {entityRows.map((le) => <option key={le.id} value={le.id}>{le.name_th}</option>)}
            </select>
          </label>
          <label><span className="block text-slate-600">{t("form.channel")}</span>
            <select className={INPUT} value={draft.channel} onChange={(e) => setDraft({ ...draft, channel: e.target.value as DsarRequestChannel })}>
              <option value="">{t("form.choose")}</option>
              {CHANNELS.map((c) => <option key={c} value={c}>{t(`channels.${c}`)}</option>)}
            </select>
          </label>
          <label><span className="block text-slate-600">{t("form.requesterName")}</span>
            <input className={INPUT} value={draft.requester_name} onChange={(e) => setDraft({ ...draft, requester_name: e.target.value })} maxLength={300} /></label>
          <label><span className="block text-slate-600">{t("form.requesterContact")}</span>
            <input className={INPUT} value={draft.requester_contact} onChange={(e) => setDraft({ ...draft, requester_contact: e.target.value })} maxLength={300} /></label>
          <label><span className="block text-slate-600">{t("form.contactKind")}</span>
            <select className={INPUT} value={draft.contact_kind} onChange={(e) => setDraft({ ...draft, contact_kind: e.target.value as DsarContactKind })}>
              {CONTACT_KINDS.map((k) => <option key={k} value={k}>{t(`contactKinds.${k}`)}</option>)}
            </select>
          </label>
          {m.create.isError && <p className="text-red-700 sm:col-span-3" role="alert">{t("form.saveError", { detail: detail(m.create.error) })}</p>}
          <div className="flex gap-2 sm:col-span-3">
            <Button onClick={create} disabled={m.create.isPending}>{t("form.save")}</Button>
            <Button variant="secondary" onClick={() => setDraft(null)}>{t("form.cancel")}</Button>
          </div>
        </fieldset>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <span className="text-slate-600">{t("filterStatus")}</span>
        <select className={INPUT + " w-auto"} value={statusFilter} onChange={(e) => setStatusFilter(e.target.value as DsarRequestStatus | "")}>
          <option value="">{t("allStatuses")}</option>
          {STATUSES.map((s) => <option key={s} value={s}>{t(`statuses.${s}`)}</option>)}
        </select>
        <label className="flex items-center gap-2">
          <span className="text-slate-600">{t("search.label")}</span>
          <input className={INPUT + " w-64"} value={search} onChange={(e) => setSearch(e.target.value)}
            placeholder={t("search.placeholder")} data-testid="request-search" />
        </label>
      </div>

      {list.isPending ? <p className="text-slate-500">{t("loading")}</p> : list.isError ? <p className="text-red-700">{t("loadError")}</p> : rows.length === 0 ? (
        <p className="rounded-md bg-amber-50 p-3 text-amber-800">{t("empty")}</p>
      ) : (
        <table className="w-full rounded-md border border-slate-200 bg-white" data-testid="requests-list">
          <thead className="bg-slate-50 text-left text-slate-600">
            <tr><th className="px-3 py-2">{t("form.requestNo")}</th><th className="px-3 py-2">{t("form.requestType")}</th>
              <th className="px-3 py-2">{t("dueBy")}</th><th className="px-3 py-2">{t("sla.label")}</th>
              <th className="px-3 py-2">{t("statusLabel")}</th><th className="px-3 py-2" /></tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.map((r) => (
              <>
                <tr key={r.id}>
                  <td className="px-3 py-2 font-mono text-xs">{r.request_no}</td>
                  <td className="px-3 py-2">{typeName(r.request_type_id)}</td>
                  <td className="px-3 py-2">{formatDate(r.due_at, locale, { day: "numeric", month: "short", year: "numeric" })}</td>
                  <td className="px-3 py-2"><span className={`rounded px-2 py-0.5 ${SLA_STYLE[r.sla_status]}`} data-testid={`sla-${r.id}`}>{t(`sla.${r.sla_status}`)}</span></td>
                  <td className="px-3 py-2"><span className={`rounded px-2 py-0.5 ${STATUS_STYLE[r.status]}`}>{t(`statuses.${r.status}`)}</span></td>
                  <td className="px-3 py-2 space-x-3">
                    {canExecute && NEXT_STEPS[r.status].length > 0 && (
                      <button type="button" className="text-sky-700 underline" data-testid={`action-toggle-${r.id}`}
                        onClick={() => { setActionId(actionId === r.id ? null : r.id); setActionDraft({ to: "", outcome: "", rejection_reason_code: "", activity_ids: [] }); }}>
                        {actionId === r.id ? t("form.hide") : t("takeAction")}
                      </button>
                    )}
                    {canUpdate && (
                      <button type="button" className="text-sky-700 underline" data-testid={`assign-toggle-${r.id}`}
                        onClick={() => setAssignId(assignId === r.id ? null : r.id)}>
                        {assignId === r.id ? t("form.hide") : t("form.assign")}
                      </button>
                    )}
                    <button type="button" className="text-sky-700 underline" data-testid={`history-toggle-${r.id}`}
                      onClick={() => setHistoryId(historyId === r.id ? null : r.id)}>
                      {historyId === r.id ? t("form.hide") : t("history")}
                    </button>
                    <button type="button" className="text-sky-700 underline" data-testid={`verify-toggle-${r.id}`}
                      onClick={() => setVerifyId(verifyId === r.id ? null : r.id)}>
                      {verifyId === r.id ? t("form.hide") : t("verify.title")}
                    </button>
                  </td>
                </tr>
                {verifyId === r.id && (
                  <tr>
                    <td colSpan={6} className="bg-slate-50 px-3 py-3">
                      <VerificationPanel client={client} request={r} canUpdate={canUpdate} />
                    </td>
                  </tr>
                )}
                {assignId === r.id && (
                  <tr>
                    <td colSpan={6} className="bg-slate-50 px-3 py-3">
                      <div className="flex flex-wrap items-center gap-3">
                        <span className="text-slate-600">
                          {r.assignee_user_id ? t("form.currentAssignee", { id: r.assignee_user_id.slice(0, 8) }) : t("form.unassigned")}
                        </span>
                        <AssigneePicker client={client} onPick={(u) => m.assign.mutate({ request: r, userId: u.id })} />
                        {r.assignee_user_id && (
                          <Button variant="secondary" onClick={() => m.assign.mutate({ request: r, userId: null })} disabled={m.assign.isPending}>{t("form.unassign")}</Button>
                        )}
                      </div>
                    </td>
                  </tr>
                )}
                {historyId === r.id && (
                  <tr>
                    <td colSpan={6} className="bg-slate-50 px-3 py-3">
                      <RecordCollaboration entityType="dsar_request" entityId={r.id} canWrite={canUpdate} currentUserId={currentUserId} />
                    </td>
                  </tr>
                )}
                {actionId === r.id && (
                  <tr>
                    <td colSpan={6} className="bg-slate-50 px-3 py-3">
                      <div className="grid gap-2 sm:grid-cols-3">
                        <select className={INPUT} value={actionDraft.to} onChange={(e) => setActionDraft({ ...actionDraft, to: e.target.value as DsarRequestStatus })}>
                          <option value="">{t("form.nextStatus")}</option>
                          {NEXT_STEPS[r.status].map((s) => <option key={s} value={s}>{t(`statuses.${s}`)}</option>)}
                        </select>
                        {actionDraft.to === "completed" && (
                          <select className={INPUT} value={actionDraft.outcome} onChange={(e) => setActionDraft({ ...actionDraft, outcome: e.target.value as DsarOutcome })}>
                            <option value="">{t("form.outcome")}</option>
                            <option value="fulfilled">{t("outcomes.fulfilled")}</option>
                            <option value="partially_fulfilled">{t("outcomes.partially_fulfilled")}</option>
                          </select>
                        )}
                        {actionDraft.to === "rejected" && (
                          <input className={INPUT} placeholder={t("form.rejectionReason")} value={actionDraft.rejection_reason_code}
                            onChange={(e) => setActionDraft({ ...actionDraft, rejection_reason_code: e.target.value })} />
                        )}
                        {m.transition.isError && <p className="text-red-700 sm:col-span-3" role="alert">{t("form.saveError", { detail: detail(m.transition.error) })}</p>}
                        <Button onClick={() => submitAction(r)} disabled={m.transition.isPending || !actionDraft.to}>{t("form.save")}</Button>
                      </div>
                      {actionDraft.to === "rejected" && (
                        <div className="mt-2">
                          <span className="block text-slate-600">{t("form.relatedActivities")}</span>
                          <p className="mb-1 text-xs text-slate-500">{t("form.relatedActivitiesHint")}</p>
                          <div className="max-h-32 space-y-1 overflow-auto rounded-md border border-slate-200 p-2">
                            {activityRows.map((a) => (
                              <label key={a.id} className="flex items-center gap-2">
                                <input type="checkbox" checked={actionDraft.activity_ids.includes(a.id)} onChange={() => toggleActionActivity(a.id)} />
                                <span>{a.code} — {a.name}</span>
                              </label>
                            ))}
                          </div>
                        </div>
                      )}
                      {m.transition.data?.document_id && (
                        <p className="mt-2">
                          <Link className="text-sky-700 underline" href={`/documents/${m.transition.data.document_id}`}>{t("openLetter")}</Link>
                        </p>
                      )}
                    </td>
                  </tr>
                )}
              </>
            ))}
          </tbody>
        </table>
      )}
      {list.hasNextPage && <Button variant="secondary" onClick={() => list.fetchNextPage()}>{t("more")}</Button>}
    </main>
  );
}
