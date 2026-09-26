"use client";

import { useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { formatDate, type Locale } from "@pdpa/i18n";
import {
  createApiClient,
  useCollectionPoints,
  useConsentSettings,
  useConsentSubject,
  useConsentSubjectMutations,
  useConsentSubjects,
  type ConsentDecisionInput,
  type SubjectIdentifier,
} from "@pdpa/api-client";
import { StatusBadge, field, input, problemText, useText } from "../shared";

const ID_TYPES = ["email", "phone", "customer_id", "national_id", "passport", "line_uid", "other"] as const;
const REASONS = ["no_longer_interested", "too_many_messages", "privacy_concern", "service_ended", "other"] as const;

export function SubjectsContent() {
  const t = useTranslations("consent");
  const canRead = usePermission("consent.record.read");
  const canRecord = usePermission("consent.onbehalf.create");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const [draft, setDraft] = useState<{ type: SubjectIdentifier["type"]; value: string }>({ type: "email", value: "" });
  const [search, setSearch] = useState<SubjectIdentifier | null>(null);
  const list = useConsentSubjects(client, search);
  const [selected, setSelected] = useState<string | "new" | null>(null);
  const locale = useLocale() as Locale;
  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  return (
    <main className="mx-auto max-w-6xl space-y-4 p-8 text-sm">
      <header className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{t("subjects.title")}</h1>
          <p className="text-slate-600">{t("subjects.intro")}</p>
        </div>
        {canRecord && <Button onClick={() => setSelected("new")} data-testid="new-record">{t("subjects.recordNew")}</Button>}
      </header>
      <StorageCard />
      <form className="flex flex-wrap items-end gap-2" onSubmit={(e) => { e.preventDefault(); setSearch(draft.value.trim() ? { type: draft.type, value: draft.value.trim() } : null); }}>
        <select className={`${field} w-40`} value={draft.type} onChange={(e) => setDraft({ ...draft, type: e.target.value as SubjectIdentifier["type"] })}>
          {ID_TYPES.map((x) => <option key={x} value={x}>{t(`idType.${x}`)}</option>)}
        </select>
        <input className={`${field} w-72`} value={draft.value} placeholder={t("subjects.searchHint")} onChange={(e) => setDraft({ ...draft, value: e.target.value })} data-testid="subject-search" />
        <Button type="submit">{t("subjects.search")}</Button>
        {search && <Button variant="ghost" type="button" onClick={() => { setSearch(null); setDraft({ ...draft, value: "" }); }}>{t("subjects.clear")}</Button>}
      </form>
      <div className="grid gap-4 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
        <section className="rounded-md border border-slate-200 bg-white">
          <h2 className="border-b border-slate-100 px-3 py-2 text-xs font-medium text-slate-500">{search ? t("subjects.results") : t("subjects.recent")}</h2>
          {list.isPending && <p className="p-3 text-slate-500">{t("loading")}</p>}
          {list.data?.length === 0 && <p className="p-3 text-slate-500">{t("subjects.none")}</p>}
          <ul className="divide-y divide-slate-100">
            {list.data?.map((s) => (
              <li key={s.id}>
                <button className={`w-full px-3 py-2 text-left hover:bg-slate-50 ${s.id === selected ? "bg-slate-50" : ""}`} onClick={() => setSelected(s.id)} data-testid={`subject-${s.key}`}>
                  <span className="font-mono text-xs text-slate-500">{s.key}</span>
                  <span className="ml-2">{s.identifiers.map((i) => i.masked).join(" · ")}</span>
                  {s.last_activity_at && <span className="block text-xs text-slate-500">{formatDate(s.last_activity_at, locale, { month: "short", hour: "2-digit", minute: "2-digit" })}</span>}
                </button>
              </li>
            ))}
          </ul>
        </section>
        <section className="space-y-4">
          {selected === "new" && <RecordForm onDone={(id) => setSelected(id)} />}
          {selected && selected !== "new" && <Profile key={selected} id={selected} />}
        </section>
      </div>
    </main>
  );
}

/** CON-17: where the consent data lives and how identifiers are protected. */
function StorageCard() {
  const t = useTranslations("consent.storage");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const s = useConsentSettings(client);
  if (!s.data) return null;
  return (
    <p className="rounded-md bg-slate-100 p-3 text-slate-700" data-testid="storage-card">
      {t("summary", { region: s.data.data_region, encryption: s.data.identifier_encryption.toUpperCase() })} · {t(`km.${s.data.key_management}`)}
    </p>
  );
}

function Profile({ id }: { id: string }) {
  const t = useTranslations("consent");
  const locale = useLocale() as Locale;
  const canRecord = usePermission("consent.onbehalf.create");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const p = useConsentSubject(client, id);
  const m = useConsentSubjectMutations(client);
  const text = useText();
  if (p.isPending) return <p className="text-slate-500">{t("loading")}</p>;
  if (p.isError) return <p className="text-red-700">{problemText(p.error)}</p>;
  const s = p.data;
  const when = (d: string) => formatDate(d, locale, { month: "short", hour: "2-digit", minute: "2-digit" });

  return (
    <>
      <div className="space-y-3 rounded-md border border-slate-200 bg-white p-4" data-testid="subject-profile">
        <header className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="font-mono font-semibold">{s.key}</h2>
          <span className="flex items-center gap-2">
            <Button variant="secondary" onClick={() => m.verify.mutate(id)} disabled={m.verify.isPending} data-testid="verify">{t("subjects.verify")}</Button>
          </span>
        </header>
        {m.verify.data && (
          <p className={m.verify.data.ok ? "text-emerald-700" : "text-red-700"} role="status" data-testid="verify-result">
            {m.verify.data.ok ? t("subjects.verifyOk", { n: m.verify.data.checked }) : t("subjects.verifyBroken", { receipt: m.verify.data.broken_at ?? "", reason: t(`subjects.reason.${m.verify.data.reason ?? "hash_mismatch"}`) })}
          </p>
        )}
        <ul className="flex flex-wrap gap-2">
          {s.identifiers.map((i, k) => (
            <li key={k} className="rounded-md bg-slate-100 px-2 py-1">
              <span className="text-xs text-slate-500">{t(`idType.${i.type}`)}</span> {i.masked}
              {i.verified && <span className="ml-1 text-xs text-emerald-700">✓</span>}
            </li>
          ))}
        </ul>
        <table className="w-full text-left">
          <thead className="text-xs text-slate-500">
            <tr><th className="py-1">{t("subjects.purpose")}</th><th>{t("subjects.status")}</th><th>{t("subjects.version")}</th><th>{t("subjects.updated")}</th></tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {s.statuses.map((st) => (
              <tr key={st.purpose_id} data-testid={`status-${st.purpose_code}`}>
                <td className="py-1.5">{text(st.purpose_name)}{st.is_sensitive && <span className="ml-1 text-xs text-rose-700">{t("sensitive")}</span>}</td>
                <td><StatusBadge status={st.status} />{st.needs_reconsent && <span className="ml-1 text-xs text-amber-800">{t("subjects.needsReconsent")}</span>}</td>
                <td>{st.version}</td>
                <td className="text-xs text-slate-500">{when(st.updated_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {canRecord && <RecordForm subjectId={id} active={s.statuses.filter((x) => x.status === "ACTIVE").map((x) => x.purpose_code)} onDone={() => undefined} />}
      <div className="rounded-md border border-slate-200 bg-white p-4">
        <h3 className="mb-2 font-semibold">{t("subjects.history")}</h3>
        <ol className="space-y-2" data-testid="history">
          {s.history.map((h) => (
            <li key={h.id} className="border-l-2 border-slate-200 pl-3">
              <div className="flex flex-wrap gap-2">
                <span className="font-medium">{t(`txType.${h.type}`)}</span>
                <span>{text(h.purpose_name)} · {t("purposes.versionNo", { no: h.version })}</span>
                {h.reason_code && <span className="text-slate-500">({t(`reason.${h.reason_code}`)})</span>}
              </div>
              <div className="text-xs text-slate-500">
                {when(h.occurred_at)} · {h.collection_point} · {t(`source.${h.source}`)}{h.captured_by && ` · ${t("subjects.by", { name: h.captured_by })}`} · <span className="font-mono">{h.receipt_no}</span>
              </div>
            </li>
          ))}
        </ol>
      </div>
    </>
  );
}

/** Staff record consent or a withdrawal on the subject's behalf (CON-13) — for an existing subject or a new one by identifier. */
function RecordForm({ subjectId, active = [], onDone }: { subjectId?: string; active?: string[]; onDone: (subjectId: string) => void }) {
  const t = useTranslations("consent");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const cps = useCollectionPoints(client);
  const m = useConsentSubjectMutations(client);
  const text = useText();
  const live = (cps.data ?? []).filter((c) => c.status === "active");
  const [cpId, setCpId] = useState("");
  const [ident, setIdent] = useState<{ type: SubjectIdentifier["type"]; value: string }>({ type: "email", value: "" });
  const [choice, setChoice] = useState<Record<string, { decision: ConsentDecisionInput["decision"] | ""; reason: string }>>({});
  const cp = live.find((c) => c.id === cpId) ?? live[0];
  const decisions: ConsentDecisionInput[] = (cp?.purposes ?? [])
    .filter((p) => choice[p.code]?.decision && p.current_version)
    .map((p) => ({ purpose_code: p.code, purpose_version_no: p.current_version!, decision: choice[p.code].decision as ConsentDecisionInput["decision"],
      ...(choice[p.code].decision === "WITHDRAWN" && choice[p.code].reason ? { reason_code: choice[p.code].reason as ConsentDecisionInput["reason_code"] } : {}) }));
  const submit = () => {
    if (!cp) return;
    m.record.mutate(
      { collection_point_id: cp.id, decisions, ...(subjectId ? { subject_id: subjectId } : { identifiers: [{ type: ident.type, value: ident.value.trim() }] }) },
      { onSuccess: (r) => { setChoice({}); onDone(r.subject_ref); } },
    );
  };

  return (
    <div className="space-y-3 rounded-md border border-slate-200 bg-white p-4" data-testid="record-form">
      <h3 className="font-semibold">{subjectId ? t("subjects.recordTitle") : t("subjects.recordNew")}</h3>
      <p className="text-xs text-slate-500">{t("subjects.recordHint")}</p>
      {live.length === 0 ? <p className="text-slate-500">{t("subjects.noLiveCp")}</p> : (
        <>
          {!subjectId && (
            <div className="flex flex-wrap gap-2">
              <select className={`${field} w-40`} value={ident.type} onChange={(e) => setIdent({ ...ident, type: e.target.value as SubjectIdentifier["type"] })}>
                {ID_TYPES.map((x) => <option key={x} value={x}>{t(`idType.${x}`)}</option>)}
              </select>
              <input className={`${field} w-72`} value={ident.value} onChange={(e) => setIdent({ ...ident, value: e.target.value })} data-testid="record-identifier" />
            </div>
          )}
          <label className="block space-y-0.5">
            <span className="font-medium">{t("subjects.collectionPoint")}</span>
            <select className={input} value={cp?.id} onChange={(e) => { setCpId(e.target.value); setChoice({}); }} data-testid="record-cp">
              {live.map((c) => <option key={c.id} value={c.id}>{c.name} ({c.code})</option>)}
            </select>
          </label>
          <ul className="divide-y divide-slate-100 rounded-md border border-slate-200">
            {cp?.purposes.filter((p) => p.current_version).map((p) => {
              const c = choice[p.code] ?? { decision: "", reason: "" };
              const set = (patch: Partial<typeof c>) => setChoice({ ...choice, [p.code]: { ...c, ...patch } });
              return (
                <li key={p.purpose_id} className="flex flex-wrap items-center justify-between gap-2 px-3 py-1.5">
                  <span>{text(p.name)}</span>
                  <span className="flex gap-2">
                    <select className={`${field} w-44`} value={c.decision} onChange={(e) => set({ decision: e.target.value as typeof c.decision })} data-testid={`decision-${p.code}`}>
                      <option value="">{t("subjects.noChange")}</option>
                      <option value="CONSENTED">{t("decision.CONSENTED")}</option>
                      <option value="NOT_CONSENTED">{t("decision.NOT_CONSENTED")}</option>
                      {active.includes(p.code) && <option value="WITHDRAWN">{t("decision.WITHDRAWN")}</option>}
                    </select>
                    {c.decision === "WITHDRAWN" && (
                      <select className={`${field} w-48`} value={c.reason} onChange={(e) => set({ reason: e.target.value })} data-testid={`reason-${p.code}`}>
                        <option value="">{t("subjects.reasonNone")}</option>
                        {REASONS.map((r) => <option key={r} value={r}>{t(`reason.${r}`)}</option>)}
                      </select>
                    )}
                  </span>
                </li>
              );
            })}
          </ul>
          {m.record.isError && <p className="text-red-700" role="alert">{problemText(m.record.error)}</p>}
          {m.record.isSuccess && <p className="text-emerald-700" role="status" data-testid="record-done">{t("subjects.recorded", { receipt: m.record.data.receipt_no })}</p>}
          <Button onClick={submit} disabled={decisions.length === 0 || m.record.isPending || (!subjectId && !ident.value.trim())} data-testid="record-submit">{t("subjects.record")}</Button>
        </>
      )}
    </div>
  );
}
