"use client";

import { useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { FormRenderer, type FormSchema, type Language, type Scoring } from "@pdpa/form-renderer";
import {
  createApiClient,
  fileDownloadHref,
  useDocuments,
  useFileStatus,
  useForm,
  useForms,
  useIncident,
  useIncidentAssessments,
  useIncidentEvidence,
  useIncidentMutations,
  useIncidentNotices,
  useIncidentPDPCNotifications,
  useIncidentTimeline,
  useNoticeMutations,
  useNoticeRecipients,
  usePDPCNotificationMutations,
  usePublishedDocumentVersions,
  type BreachIncident,
  type BreachNotice,
  type BreachNoticeVars,
  type BreachPDPCNotification,
  type BreachTimelineItem,
} from "@pdpa/api-client";
import { FileUploader } from "@/components/file-uploader";
import { useRendererMessages } from "@/components/form-messages";
import type { ErrorCode, FieldError } from "@pdpa/form-renderer";
import { Link } from "@/i18n/routing";
import { Clock, RiskBadge, StatusBadge, field, input, problemText, useWhen } from "../shared";

type Tab = "assessment" | "timeline" | "evidence" | "notices" | "pdpc";

/** Answer errors of a 422 breach.invalid problem (fields "answers.<question>"), as the renderer takes them. */
function answerErrors(e: unknown): FieldError[] | undefined {
  const p = e as { code?: string; errors?: { field: string; code: string }[] } | null;
  if (p?.code !== "breach.invalid") return undefined;
  const list = (p.errors ?? []).filter((x) => x.field.startsWith("answers.")).map((x) => ({ question: x.field.slice(8), code: x.code as ErrorCode }));
  return list.length ? list : undefined;
}

export function IncidentContent({ id, meId }: { id: string; meId: string }) {
  const t = useTranslations("breach");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const q = useIncident(client, id);
  const [tab, setTab] = useState<Tab>("timeline");
  const when = useWhen();
  if (q.isPending) return <main className="p-8 text-slate-500">{t("loading")}</main>;
  if (q.isError) return <main className="p-8 text-red-700">{problemText(q.error)}</main>;
  const inc = q.data;

  return (
    <main className="mx-auto max-w-6xl space-y-4 p-8 text-sm">
      <p><Link className="text-sky-700 underline" href="/incidents">← {t("title")}</Link></p>
      <header className="space-y-2 rounded-md border border-slate-200 bg-white p-4" data-testid="incident-header">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-mono text-xs text-slate-500">{inc.incident_no}</span>
          <h1 className="text-lg font-semibold">{inc.title}</h1>
          <StatusBadge status={inc.status} />
          <RiskBadge risk={inc.risk_level} />
          <Clock incident={inc} />
        </div>
        <p className="whitespace-pre-line text-slate-700">{inc.description}</p>
        <dl className="grid gap-x-6 gap-y-1 text-xs md:grid-cols-4">
          <div><dt className="text-slate-500">{t("awareAt")}</dt><dd>{when(inc.aware_at)}</dd></div>
          <div><dt className="text-slate-500">{t("deadline")}</dt><dd data-testid="due-at">{when(inc.clock.due_at)}</dd></div>
          <div><dt className="text-slate-500">{t("owner")}</dt><dd>{inc.owner_name}</dd></div>
          <div><dt className="text-slate-500">{t("affectedSubjects")}</dt><dd>{inc.affected_subjects ?? "—"}</dd></div>
          <div><dt className="text-slate-500">{t("breachTypes")}</dt><dd>{inc.breach_types.map((x) => t(`type.${x}`)).join(", ")}</dd></div>
          <div><dt className="text-slate-500">{t("reportedVia")}</dt><dd>{t(`via.${inc.reported_via}`)}</dd></div>
          {inc.decision && <div className="md:col-span-2"><dt className="text-slate-500">{t("decisionLabel")}</dt><dd data-testid="decision">{t(`decision.${inc.decision}`)} — {inc.decision_reason}</dd></div>}
        </dl>
        {inc.clock.state === "overdue" && <p className="rounded-md bg-rose-50 p-2 text-rose-900" role="alert">{t("overdueNotice")}</p>}
      </header>
      <Actions incident={inc} meId={meId} />
      <nav className="flex flex-wrap gap-2" role="tablist">
        {(["timeline", "assessment", "evidence", "notices", "pdpc"] as Tab[]).map((k) => (
          <button key={k} role="tab" aria-selected={tab === k} className={`rounded-md px-3 py-1.5 ${tab === k ? "bg-slate-900 text-white" : "bg-white ring-1 ring-slate-200"}`} onClick={() => setTab(k)} data-testid={`tab-${k}`}>
            {t(`tabs.${k}`)}
          </button>
        ))}
      </nav>
      {tab === "timeline" && <Timeline incident={inc} />}
      {tab === "assessment" && <Assessment incident={inc} />}
      {tab === "evidence" && <Evidence incident={inc} />}
      {tab === "notices" && <Notices incident={inc} />}
      {tab === "pdpc" && <PDPCPanel incident={inc} />}
    </main>
  );
}

/** What the caller can do next on ST-03. */
function Actions({ incident, meId }: { incident: BreachIncident; meId: string }) {
  const t = useTranslations("breach");
  const canUpdate = usePermission("breach.incident.update");
  const canApprove = usePermission("breach.incident.approve");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const m = useIncidentMutations(client, incident.id);
  const [reason, setReason] = useState("");
  const [asking, setAsking] = useState<null | "closed" | "assessing">(null);
  if (!canUpdate || incident.status === "closed") return null;
  const go = (to: "triage" | "assessing" | "closed", extra: { reason?: string; owner_user_id?: string } = {}) =>
    m.transition.mutate({ incident, to, ...extra }, { onSuccess: () => { setAsking(null); setReason(""); } });
  const s = incident.status;
  return (
    <section className="space-y-2 rounded-md border border-slate-200 bg-white p-4" data-testid="actions">
      <h2 className="font-semibold">{t("next")}</h2>
      <div className="flex flex-wrap gap-2">
        {s === "reported" && <Button onClick={() => go("triage", { owner_user_id: meId })} data-testid="take-up">{t("actions.takeUp")}</Button>}
        {s === "triage" && <Button onClick={() => go("assessing")} data-testid="confirm">{t("actions.confirm")}</Button>}
        {s === "triage" && canApprove && <Button variant="secondary" onClick={() => setAsking("closed")}>{t("actions.notABreach")}</Button>}
        {s === "remediating" && canApprove && <Button onClick={() => setAsking("closed")} data-testid="close">{t("actions.close")}</Button>}
        {s === "remediating" && <Button variant="secondary" onClick={() => setAsking("assessing")}>{t("actions.reassess")}</Button>}
        {s === "assessing" && <p className="text-slate-600">{t("actions.assessHint")}</p>}
        {s === "notifying" && <p className="text-slate-600" data-testid="pdpc-pending">{t("actions.pdpcPending")}</p>}
      </div>
      {asking && (
        <div className="space-y-2">
          <textarea className={input} rows={2} placeholder={asking === "closed" ? t("actions.closeReason") : t("actions.reassessReason")} value={reason} onChange={(e) => setReason(e.target.value)} data-testid="transition-reason" />
          <Button onClick={() => go(asking, { reason })} disabled={!reason.trim() || m.transition.isPending} data-testid="transition-submit">{t("actions.confirmButton")}</Button>
        </div>
      )}
      {m.transition.isError && <p className="text-red-700" role="alert">{problemText(m.transition.error)}</p>}
    </section>
  );
}

function Timeline({ incident }: { incident: BreachIncident }) {
  const t = useTranslations("breach");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const q = useIncidentTimeline(client, incident.id);
  const m = useIncidentMutations(client, incident.id);
  const canUpdate = usePermission("breach.incident.update");
  const [note, setNote] = useState("");
  const when = useWhen();
  return (
    <section className="space-y-3 rounded-md border border-slate-200 bg-white p-4">
      <ol className="space-y-2" data-testid="timeline">
        {q.data?.map((it) => (
          <li key={it.id} className="border-l-2 border-slate-200 pl-3" data-type={it.type}>
            <div className="flex flex-wrap gap-2">
              <span className="font-medium">{it.auto ? describe(t, it) : t("timeline.note")}</span>
              {it.text && <span className="whitespace-pre-line text-slate-700">{it.text}</span>}
            </div>
            <div className="text-xs text-slate-500">{when(it.occurred_at)}{it.actor_name ? ` · ${it.actor_name}` : ` · ${t("timeline.system")}`}</div>
          </li>
        ))}
      </ol>
      {canUpdate && incident.status !== "closed" && (
        <div className="flex gap-2">
          <input className={input} placeholder={t("timeline.addNote")} value={note} onChange={(e) => setNote(e.target.value)} data-testid="note-text" />
          <Button onClick={() => m.note.mutate({ text: note }, { onSuccess: () => setNote("") })} disabled={!note.trim() || m.note.isPending} data-testid="note-add">{t("timeline.add")}</Button>
        </div>
      )}
    </section>
  );
}

type T = ReturnType<typeof useTranslations>;

/** An automatic timeline entry from its token (kind:args). */
function describe(t: T, it: BreachTimelineItem): string {
  const [kind, ...a] = (it.token ?? "").split(":");
  switch (kind) {
    case "reported":
      return t("timeline.reported", { via: t(`via.${a[0]}`) });
    case "status":
      return t("timeline.status", { from: t(`status.${a[0]}`), to: t(`status.${a[1]}`) });
    case "owner":
      return t("timeline.owner");
    case "contained":
      return t("timeline.contained");
    case "aware_at":
      return t("timeline.awareAt");
    case "assessment":
      return t("timeline.assessment", { risk: t(`risk.${a[0]}`), score: a[1] });
    case "decision":
      return t("timeline.decision", { decision: t(`decision.${a[0]}`), risk: t(`risk.${a[1]}`) });
    case "evidence":
      return t("timeline.evidence", { hash: (a[0] ?? "").slice(0, 12) });
    case "deadline":
      return a[0] === "72" ? t("timeline.overdue") : t("timeline.deadline", { hours: a[0] });
    case "notice_draft":
      return t("timeline.noticeDraft", { channel: t(`channel.${a[0]}`) });
    case "notice_approved":
      return t("timeline.noticeApproved", { channel: t(`channel.${a[0]}`), total: a[1] });
    case "notice_sent":
      return t("timeline.noticeSent", { channel: t(`channel.${a[0]}`), handed: a[1], failed: a[2] });
    default:
      return it.token ?? "";
  }
}

function Assessment({ incident }: { incident: BreachIncident }) {
  const t = useTranslations("breach");
  const locale = useLocale() as Language;
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const list = useIncidentAssessments(client, incident.id);
  const forms = useForms(client, "breach");
  const published = (forms.data ?? []).filter((f) => f.current_version_id);
  const [formId, setFormId] = useState("");
  const chosen = formId || published[0]?.id;
  const form = useForm(client, chosen);
  const m = useIncidentMutations(client, incident.id);
  const messages = useRendererMessages(locale);
  const canUpdate = usePermission("breach.incident.update");
  const when = useWhen();
  const version = form.data?.versions?.find((v) => v.id === form.data?.current_version_id);
  const text = (x?: Record<string, string>) => (x ? x[locale] ?? x.th : "");
  const answerText = (f: { answer?: unknown; answer_labels?: Record<string, string>[] }) =>
    f.answer_labels?.length ? f.answer_labels.map(text).join(", ") : f.answer === "yes" ? messages.yes : f.answer === "no" ? messages.no : String(f.answer ?? "");

  return (
    <section className="space-y-4">
      {incident.status === "assessing" && canUpdate && (
        <div className="space-y-3 rounded-md border border-slate-200 bg-white p-4" data-testid="assessment-form">
          <h2 className="font-semibold">{t("assessment.title")}</h2>
          {published.length === 0 ? <p className="text-amber-800">{t("assessment.noForm")}</p> : (
            <select className={`${field} w-80`} value={chosen} onChange={(e) => setFormId(e.target.value)}>
              {published.map((f) => <option key={f.id} value={f.id}>{f.name}</option>)}
            </select>
          )}
          {version && (
            <FormRenderer schema={version.schema as FormSchema} scoring={version.scoring as Scoring | null} language={locale} messages={messages} showScore
              serverErrors={answerErrors(m.assess.error)} idPrefix="breach"
              onSubmit={async (answers) => { await m.assess.mutateAsync({ form_id: chosen!, answers }); }}
              actions={({ submit, busy }) => <Button type="button" onClick={submit} disabled={busy || m.assess.isPending} data-testid="assess-submit">{t("assessment.submit")}</Button>} />
          )}
          {m.assess.isError && !answerErrors(m.assess.error) && <p className="text-red-700" role="alert">{problemText(m.assess.error)}</p>}
        </div>
      )}
      {incident.status === "assessing" && incident.risk_level && <Decision incident={incident} />}
      {list.data?.map((a) => (
        <div key={a.id} className="space-y-2 rounded-md border border-slate-200 bg-white p-4" data-testid="assessment-result">
          <div className="flex flex-wrap items-center gap-2">
            <RiskBadge risk={a.risk_level} />
            <span>{t("assessment.score", { score: a.score })}</span>
            <span className="text-xs text-slate-500">{when(a.assessed_at)} · {a.assessed_by_name}</span>
          </div>
          <table className="w-full text-left text-xs">
            <thead className="text-slate-500"><tr><th className="py-1">{t("assessment.factor")}</th><th>{t("assessment.answer")}</th><th>{t("assessment.points")}</th></tr></thead>
            <tbody className="divide-y divide-slate-100">
              {a.factors.map((f) => (
                <tr key={f.question}><td className="py-1">{text(f.label)}</td><td>{answerText(f)}</td><td>{f.points}</td></tr>
              ))}
            </tbody>
          </table>
        </div>
      ))}
    </section>
  );
}

const RANK: Record<string, number> = { no_notification: 0, notify_pdpc: 1, notify_pdpc_and_subjects: 2 };
const REQUIRED: Record<string, string> = { none: "no_notification", low: "notify_pdpc", high: "notify_pdpc_and_subjects" };

/** BRE-06: the DPO decides who is notified, never less than the risk requires. */
function Decision({ incident }: { incident: BreachIncident }) {
  const t = useTranslations("breach");
  const canApprove = usePermission("breach.incident.approve");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const m = useIncidentMutations(client, incident.id);
  const required = REQUIRED[incident.risk_level ?? "none"];
  const [decision, setDecision] = useState(required);
  const [reason, setReason] = useState("");
  if (!canApprove) return <p className="rounded-md bg-slate-100 p-3 text-slate-700">{t("decisionPanel.waiting")}</p>;
  return (
    <div className="space-y-2 rounded-md border border-sky-200 bg-sky-50 p-4" data-testid="decision-panel">
      <h2 className="font-semibold">{t("decisionPanel.title")}</h2>
      <p className="text-slate-700">{t("decisionPanel.required", { decision: t(`decision.${required}`) })}</p>
      <div className="flex flex-col gap-1">
        {Object.keys(RANK).map((d) => (
          <label key={d} className={`flex items-center gap-2 ${RANK[d] < RANK[required] ? "text-slate-400" : ""}`}>
            <input type="radio" name="decision" value={d} checked={decision === d} disabled={RANK[d] < RANK[required]} onChange={() => setDecision(d)} data-testid={`decision-${d}`} />
            {t(`decision.${d}`)}
          </label>
        ))}
      </div>
      <textarea className={input} rows={2} placeholder={t("decisionPanel.reason")} value={reason} onChange={(e) => setReason(e.target.value)} data-testid="decision-reason" />
      {m.decide.isError && <p className="text-red-700" role="alert">{problemText(m.decide.error)}</p>}
      <Button onClick={() => m.decide.mutate({ incident, decision: decision as "notify_pdpc", reason })} disabled={!reason.trim() || m.decide.isPending} data-testid="decide">{t("decisionPanel.submit")}</Button>
    </div>
  );
}

function Evidence({ incident }: { incident: BreachIncident }) {
  const t = useTranslations("breach");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const list = useIncidentEvidence(client, incident.id);
  const m = useIncidentMutations(client, incident.id);
  const canUpdate = usePermission("breach.incident.update");
  const [fileId, setFileId] = useState<string>();
  const [desc, setDesc] = useState("");
  const status = useFileStatus(client, fileId);
  const clean = status.data?.av_status === "clean";
  const when = useWhen();
  return (
    <section className="space-y-3 rounded-md border border-slate-200 bg-white p-4">
      {canUpdate && incident.status !== "closed" && (
        <div className="space-y-2" data-testid="evidence-upload">
          <FileUploader onUploaded={(f) => setFileId(f.id)} />
          {fileId && (
            <div className="flex flex-wrap gap-2">
              <input className={`${field} w-80`} placeholder={t("evidence.description")} value={desc} onChange={(e) => setDesc(e.target.value)} />
              <Button onClick={() => m.evidence.mutate({ file_id: fileId, description: desc || undefined }, { onSuccess: () => { setFileId(undefined); setDesc(""); } })}
                disabled={!clean || m.evidence.isPending} data-testid="evidence-add">{clean ? t("evidence.add") : t("evidence.scanning")}</Button>
            </div>
          )}
          {m.evidence.isError && <p className="text-red-700" role="alert">{problemText(m.evidence.error)}</p>}
        </div>
      )}
      <table className="w-full text-left text-xs" data-testid="evidence-list">
        <thead className="text-slate-500"><tr><th className="py-1">{t("evidence.file")}</th><th>SHA-256</th><th>{t("evidence.by")}</th><th>{t("evidence.when")}</th></tr></thead>
        <tbody className="divide-y divide-slate-100">
          {list.data?.map((e) => (
            <tr key={e.id}>
              <td className="py-1"><a className="text-sky-700 underline" href={`/api/bff/admin/v1/platform/files/${e.file_id}/download`}>{e.file_name}</a>{e.description && <span className="ml-1 text-slate-500">— {e.description}</span>}</td>
              <td className="font-mono">{e.sha256?.slice(0, 16)}…</td>
              <td>{e.collected_by_name}</td>
              <td>{when(e.collected_at)}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {list.data?.length === 0 && <p className="text-slate-500">{t("evidence.none")}</p>}
    </section>
  );
}

function Notices({ incident }: { incident: BreachIncident }) {
  const t = useTranslations("breach");
  const canRead = usePermission("breach.notification.read");
  const canCreate = usePermission("breach.notification.create");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const list = useIncidentNotices(client, incident.id, canRead || canCreate);
  const m = useNoticeMutations(client, incident.id);
  const [vars, setVars] = useState<BreachNoticeVars>({ organization: "", summary: "", remedy: "", contact: "" });
  const [channel, setChannel] = useState<"email" | "sms">("email");
  if (!canRead && !canCreate) return <p className="text-slate-600">{t("forbidden")}</p>;
  const needed = incident.decision === "notify_pdpc_and_subjects";
  return (
    <section className="space-y-3">
      {!needed && <p className="rounded-md bg-slate-100 p-3 text-slate-700">{t("notices.notNeeded")}</p>}
      {needed && canCreate && incident.status !== "closed" && (
        <div className="space-y-2 rounded-md border border-slate-200 bg-white p-4" data-testid="notice-form">
          <h2 className="font-semibold">{t("notices.new")}</h2>
          <p className="rounded-md bg-amber-50 p-2 text-amber-900">{t("notices.draftTemplate")}</p>
          <select className={`${field} w-40`} value={channel} onChange={(e) => setChannel(e.target.value as "email" | "sms")}>
            <option value="email">{t("channel.email")}</option>
            <option value="sms">{t("channel.sms")}</option>
          </select>
          {(["organization", "summary", "remedy", "contact"] as const).map((k) => (
            <label key={k} className="block space-y-0.5">
              <span className="font-medium">{t(`notices.vars.${k}`)}</span>
              <textarea className={input} rows={k === "summary" || k === "remedy" ? 2 : 1} value={vars[k]} onChange={(e) => setVars({ ...vars, [k]: e.target.value })} data-testid={`notice-${k}`} />
            </label>
          ))}
          {m.create.isError && <p className="text-red-700" role="alert">{problemText(m.create.error)}</p>}
          <Button onClick={() => m.create.mutate({ channel, variables: vars })} disabled={m.create.isPending || Object.values(vars).some((v) => !v.trim())} data-testid="notice-create">{t("notices.create")}</Button>
        </div>
      )}
      {list.data?.map((n) => <NoticeCard key={n.id} notice={n} incidentId={incident.id} />)}
    </section>
  );
}

function NoticeCard({ notice, incidentId }: { notice: BreachNotice; incidentId: string }) {
  const t = useTranslations("breach");
  const canUpdate = usePermission("breach.notification.update");
  const canSend = usePermission("breach.notification.approve");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const m = useNoticeMutations(client, incidentId);
  const [fileId, setFileId] = useState<string>();
  const status = useFileStatus(client, fileId);
  const clean = status.data?.av_status === "clean";
  const [show, setShow] = useState(false);
  const recipients = useNoticeRecipients(client, show ? notice.id : undefined);
  const failure = [m.recipients, m.send].find((x) => x.isError)?.error;
  return (
    <div className="space-y-2 rounded-md border border-slate-200 bg-white p-4" data-testid="notice-card">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-semibold">{t(`channel.${notice.channel}`)}</span>
        <span className="rounded-full bg-slate-100 px-2 py-0.5 text-xs" data-testid="notice-status">{t(`notices.status.${notice.status}`)}</span>
        <span className="text-xs text-slate-500">{t("notices.by", { name: notice.created_by_name ?? "" })}{notice.approved_by_name ? ` · ${t("notices.approvedBy", { name: notice.approved_by_name })}` : ""}</span>
      </div>
      <p className="text-slate-700">{notice.variables.summary}</p>
      <p data-testid="notice-progress">
        {t("notices.progress", { total: notice.total, handed: notice.handed, failed: notice.failed, sent: notice.delivery.sent ?? 0, queued: notice.delivery.queued ?? 0, undelivered: notice.delivery.failed ?? 0 })}
      </p>
      {notice.status === "draft" && canUpdate && (
        <div className="space-y-1">
          <p className="text-xs text-slate-500">{t("notices.csvHint")}</p>
          <FileUploader accept=".csv,.txt" onUploaded={(f) => setFileId(f.id)} />
          {fileId && <Button onClick={() => m.recipients.mutate({ notice, file_id: fileId }, { onSuccess: () => setFileId(undefined) })} disabled={!clean || m.recipients.isPending} data-testid="recipients-load">{clean ? t("notices.loadRecipients") : t("evidence.scanning")}</Button>}
        </div>
      )}
      {notice.status === "draft" && canSend && notice.total > 0 && (
        <Button onClick={() => m.send.mutate(notice)} disabled={m.send.isPending} data-testid="notice-send">{t("notices.send", { total: notice.total })}</Button>
      )}
      {failure && <p className="text-red-700" role="alert">{problemText(failure)}</p>}
      {notice.total > 0 && <Button variant="ghost" onClick={() => setShow(!show)}>{show ? t("notices.hideRecipients") : t("notices.showRecipients")}</Button>}
      {show && (
        <table className="w-full text-left text-xs" data-testid="recipients">
          <thead className="text-slate-500"><tr><th className="py-1">#</th><th>{t("notices.address")}</th><th>{t("notices.handover")}</th><th>{t("notices.delivery")}</th></tr></thead>
          <tbody className="divide-y divide-slate-100">
            {recipients.data?.map((r) => (
              <tr key={r.id}><td className="py-1">{r.line}</td><td className="font-mono">{r.masked}</td><td>{t(`notices.handoverStatus.${r.status}`)}</td><td>{r.delivery ? t(`notices.deliveryStatus.${r.delivery}`) : "—"}</td></tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

function PDPCPanel({ incident }: { incident: BreachIncident }) {
  const t = useTranslations("breach.pdpc");
  const tb = useTranslations("breach");
  const when = useWhen();
  const canRead = usePermission("breach.notification.read");
  const canCreate = usePermission("breach.notification.create");
  const canConfirm = usePermission("breach.notification.approve");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const list = useIncidentPDPCNotifications(client, incident.id, canRead || canCreate);
  const m = usePDPCNotificationMutations(client, incident.id);
  const docs = useDocuments(client, { doc_type: "pdpc_form" });
  const documents = docs.data?.pages.flatMap((p) => p.data) ?? [];
  const [docId, setDocId] = useState("");
  const versions = usePublishedDocumentVersions(client, docId);
  const published = versions.data ?? [];
  const [versionId, setVersionId] = useState("");
  const [type, setType] = useState<BreachPDPCNotification["notification_type"]>("initial");
  const [submittedAt, setSubmittedAt] = useState(() => new Date().toISOString().slice(0, 16));
  const [ref, setRef] = useState("");
  const [lateReason, setLateReason] = useState("");
  const [fileId, setFileId] = useState<string>();
  const fileStatus = useFileStatus(client, fileId);
  const evidenceClean = !fileId || fileStatus.data?.av_status === "clean";

  if (!canRead && !canCreate) return <p className="text-slate-600">{tb("forbidden")}</p>;

  const version = versionId || published[published.length - 1]?.id || "";

  return (
    <section className="space-y-3">
      <p className="text-slate-600">{t("intro")}</p>
      {canCreate && incident.status !== "closed" && (
        <form
          className="space-y-2 rounded-md border border-slate-200 bg-white p-4"
          data-testid="pdpc-form"
          onSubmit={(e) => {
            e.preventDefault();
            m.record.mutate(
              {
                notification_type: type,
                document_version_id: version,
                submitted_at: new Date(submittedAt).toISOString(),
                submission_ref: ref || undefined,
                evidence_file_id: fileId,
                late_reason: lateReason || undefined,
              },
              { onSuccess: () => { setRef(""); setLateReason(""); setFileId(undefined); } },
            );
          }}
        >
          <h2 className="font-semibold">{t("new")}</h2>
          <div className="grid gap-2 md:grid-cols-2">
            <label className="space-y-1">
              <span>{t("type")}</span>
              <select className={input} value={type} onChange={(e) => setType(e.target.value as typeof type)}>
                {(["initial", "supplementary", "final"] as const).map((x) => <option key={x} value={x}>{t(`type_${x}`)}</option>)}
              </select>
            </label>
            <label className="space-y-1">
              <span>{t("submittedAt")}</span>
              <input type="datetime-local" className={input} required value={submittedAt} onChange={(e) => setSubmittedAt(e.target.value)} />
            </label>
            <label className="space-y-1 md:col-span-2">
              <span>{t("document")}</span>
              {documents.length === 0 ? (
                <p className="text-amber-700">{t("documentEmpty")}</p>
              ) : (
                <select className={input} required value={docId} onChange={(e) => { setDocId(e.target.value); setVersionId(""); }}>
                  <option value="">{t("documentNone")}</option>
                  {documents.map((d) => <option key={d.id} value={d.id}>{d.title}</option>)}
                </select>
              )}
              <Link className="text-xs text-sky-700 underline" href="/documents">{t("createDocument")}</Link>
            </label>
            {docId && published.length > 1 && (
              <label className="space-y-1">
                <span>{t("version")}</span>
                <select className={input} value={version} onChange={(e) => setVersionId(e.target.value)}>
                  {published.map((v) => <option key={v.id} value={v.id}>{`v${v.version}`}</option>)}
                </select>
              </label>
            )}
            <label className="space-y-1">
              <span>{t("submissionRef")}</span>
              <input className={input} maxLength={60} value={ref} onChange={(e) => setRef(e.target.value)} />
            </label>
            <label className="space-y-1 md:col-span-2">
              <span>{t("lateReason")}</span>
              <textarea className={input} rows={2} maxLength={4000} value={lateReason} onChange={(e) => setLateReason(e.target.value)} placeholder={t("lateReasonHint")} />
            </label>
            <div className="space-y-1 md:col-span-2">
              <span>{t("evidence")}</span>
              <FileUploader onUploaded={(f) => setFileId(f.id)} />
            </div>
          </div>
          {m.record.isError && <p className="text-red-700" role="alert">{t("error")}: {problemText(m.record.error)}</p>}
          <Button type="submit" disabled={m.record.isPending || !docId || !version || !evidenceClean} data-testid="pdpc-record">{t("record")}</Button>
        </form>
      )}
      {list.data?.length === 0 && <p className="text-slate-500">{t("empty")}</p>}
      <ul className="space-y-2">
        {list.data?.map((n) => (
          <li key={n.id} className="space-y-1 rounded-md border border-slate-200 bg-white p-3" data-testid="pdpc-round">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-semibold">{t("sequence", { no: n.sequence_no })}</span>
              <span className="rounded-full bg-slate-100 px-2 py-0.5 text-xs">{t(`type_${n.notification_type}`)}</span>
              <span className={`rounded-full px-2 py-0.5 text-xs ${n.is_late ? "bg-amber-100 text-amber-900" : "bg-emerald-50 text-emerald-800"}`}>
                {n.is_late ? t("late") : t("onTime")}
              </span>
              {n.approved_by ? (
                <span className="text-xs text-slate-500">{t("confirmedBy", { name: n.approved_by_name ?? "" })}</span>
              ) : (
                <span className="rounded-full bg-rose-50 px-2 py-0.5 text-xs text-rose-800">{t("unconfirmed")}</span>
              )}
            </div>
            <p className="text-xs text-slate-500">{t("recordedBy", { name: n.created_by_name ?? "" })} · {when(n.submitted_at)}{n.submission_ref ? ` · ${n.submission_ref}` : ""}</p>
            {n.late_reason && <p className="text-slate-700">{n.late_reason}</p>}
            <div className="flex flex-wrap items-center gap-3 text-xs">
              {n.evidence_file_id && <a className="text-sky-700 underline" href={fileDownloadHref("/api/bff", n.evidence_file_id)}>{t("downloadEvidence")}</a>}
              {!n.approved_by && canConfirm && (
                <Button variant="secondary" onClick={() => m.confirm.mutate(n)} disabled={m.confirm.isPending} data-testid="pdpc-confirm">{t("confirm")}</Button>
              )}
            </div>
          </li>
        ))}
      </ul>
      {m.confirm.isError && <p className="text-red-700" role="alert">{t("error")}: {problemText(m.confirm.error)}</p>}
    </section>
  );
}
