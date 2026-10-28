"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useQuestionnaires,
  useSendQuestionnaire,
  useApproveQuestionnaire,
  useRejectQuestionnaire,
  useLegalEntities,
  useOrgUnits,
  useMentionSearch,
  type RopaQuestionnaire,
} from "@pdpa/api-client";
import { formatDate, type Locale } from "@pdpa/i18n";
import { useLocale } from "next-intl";
import { Link } from "@/i18n/routing";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";

const STATUS_STYLE: Record<string, string> = {
  sent: "bg-sky-100 text-sky-800",
  answered: "bg-amber-100 text-amber-800",
  approved: "bg-emerald-100 text-emerald-800",
  rejected: "bg-red-100 text-red-800",
};

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

type Client = ReturnType<typeof createApiClient>;

function RespondentPicker({ client, onPick }: { client: Client; onPick: (u: { id: string; display_name: string }) => void }) {
  const t = useTranslations("ropaQuestionnaires");
  const [q, setQ] = useState("");
  const matches = useMentionSearch(client, q.length >= 2 ? q : null);
  return (
    <div className="relative inline-block w-full">
      <input className={INPUT} value={q} onChange={(e) => setQ(e.target.value)} placeholder={t("form.respondentSearch")} />
      {q.length >= 2 && !!matches.data?.length && (
        <ul className="absolute z-10 w-full rounded-md border border-slate-200 bg-white text-sm shadow">
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

/** ROPA-11: send the data mapping questionnaire to a department, then review what comes back — the
 * acceptance criterion itself ("คำตอบที่อนุมัติแล้วเข้า RoPA อัตโนมัติ") happens on approve, which routes
 * straight to the newly created activity. The respondent answers through the generic
 * /admin/v1/ropa/questionnaires/{id}/answer endpoint (not built as its own screen here — that's a
 * department-facing flow with no consumer yet, the same "no consumer yet" deferral this codebase uses
 * throughout; this page is the DPO's own send/review side). */
export function QuestionnairesContent() {
  const t = useTranslations("ropaQuestionnaires");
  const locale = useLocale() as Locale;
  const canRead = usePermission("ropa.activity.read");
  const canSend = usePermission("ropa.activity.create");
  const canApprove = usePermission("ropa.activity.approve");
  const client = useMemo(() => createApiClient("/api/bff"), []);

  const [status, setStatus] = useState<string>("");
  const list = useQuestionnaires(client, { status: status || undefined });
  const send = useSendQuestionnaire(client);
  const approve = useApproveQuestionnaire(client);
  const reject = useRejectQuestionnaire(client);

  const [sending, setSending] = useState(false);
  const [legalEntityId, setLegalEntityId] = useState("");
  const [orgUnitId, setOrgUnitId] = useState("");
  const [respondent, setRespondent] = useState<{ id: string; display_name: string } | null>(null);
  const entities = useLegalEntities(client);
  const units = useOrgUnits(client, legalEntityId || undefined);

  const [approving, setApproving] = useState<RopaQuestionnaire | null>(null);
  const [code, setCode] = useState("");

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  const rows = list.data ?? [];

  const submitSend = () => {
    if (!orgUnitId || !respondent) return;
    send.mutate(
      { org_unit_id: orgUnitId, respondent_user_id: respondent.id },
      { onSuccess: () => { setSending(false); setOrgUnitId(""); setLegalEntityId(""); setRespondent(null); } },
    );
  };

  const submitApprove = () => {
    if (!approving || !legalEntityId || !code.trim()) return;
    approve.mutate(
      { questionnaire: approving, input: { legal_entity_id: legalEntityId, code: code.trim() } },
      { onSuccess: () => { setApproving(null); setCode(""); setLegalEntityId(""); } },
    );
  };

  return (
    <main className="mx-auto max-w-5xl space-y-6 p-8 text-sm">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{t("title")}</h1>
          <p className="text-slate-600">{t("intro")}</p>
        </div>
        {canSend && <Button onClick={() => setSending(true)} data-testid="new-questionnaire">{t("send")}</Button>}
      </header>

      <label className="block w-48"><span className="block text-slate-600">{t("filters.status")}</span>
        <select className={INPUT} value={status} onChange={(e) => setStatus(e.target.value)}>
          <option value="">{t("filters.all")}</option>
          {["sent", "answered", "approved", "rejected"].map((s) => <option key={s} value={s}>{t(`statuses.${s}`)}</option>)}
        </select>
      </label>

      {sending && (
        <fieldset className="grid gap-3 rounded-md border border-slate-200 bg-white p-4" data-testid="send-form">
          <legend className="px-1 font-semibold">{t("form.sendTitle")}</legend>
          <label><span className="block text-slate-600">{t("form.legalEntity")}</span>
            <select className={INPUT} value={legalEntityId} onChange={(e) => { setLegalEntityId(e.target.value); setOrgUnitId(""); }}>
              <option value="">{t("form.pick")}</option>
              {(entities.data ?? []).map((le) => <option key={le.id} value={le.id}>{le.name_th}</option>)}
            </select>
          </label>
          {legalEntityId && (
            <label><span className="block text-slate-600">{t("form.orgUnit")}</span>
              <select className={INPUT} value={orgUnitId} onChange={(e) => setOrgUnitId(e.target.value)}>
                <option value="">{t("form.pick")}</option>
                {(units.data ?? []).map((u) => <option key={u.id} value={u.id}>{u.name_th}</option>)}
              </select>
            </label>
          )}
          <label><span className="block text-slate-600">{t("form.respondent")}</span>
            {respondent ? (
              <span className="mt-1 flex items-center gap-2">
                <span>{respondent.display_name}</span>
                <button type="button" className="text-sky-700 underline" onClick={() => setRespondent(null)}>{t("form.change")}</button>
              </span>
            ) : <RespondentPicker client={client} onPick={setRespondent} />}
          </label>
          {send.isError && <p className="text-red-700">{detail(send.error)}</p>}
          <div className="flex gap-2">
            <Button onClick={submitSend} disabled={!orgUnitId || !respondent || send.isPending}>{t("form.submit")}</Button>
            <Button onClick={() => setSending(false)} className="bg-slate-200 text-slate-700 hover:bg-slate-300">{t("form.cancel")}</Button>
          </div>
        </fieldset>
      )}

      {approving && (
        <fieldset className="grid gap-3 rounded-md border border-slate-200 bg-white p-4" data-testid="approve-form">
          <legend className="px-1 font-semibold">{t("form.approveTitle")}</legend>
          <label><span className="block text-slate-600">{t("form.legalEntity")}</span>
            <select className={INPUT} value={legalEntityId} onChange={(e) => setLegalEntityId(e.target.value)}>
              <option value="">{t("form.pick")}</option>
              {(entities.data ?? []).map((le) => <option key={le.id} value={le.id}>{le.name_th}</option>)}
            </select>
          </label>
          <label><span className="block text-slate-600">{t("form.code")}</span>
            <input className={INPUT} value={code} onChange={(e) => setCode(e.target.value)} /></label>
          {approve.isError && <p className="text-red-700">{detail(approve.error)}</p>}
          <div className="flex gap-2">
            <Button onClick={submitApprove} disabled={!legalEntityId || !code.trim() || approve.isPending}>{t("form.submit")}</Button>
            <Button onClick={() => setApproving(null)} className="bg-slate-200 text-slate-700 hover:bg-slate-300">{t("form.cancel")}</Button>
          </div>
        </fieldset>
      )}

      {list.isPending ? (
        <p className="text-slate-500">{t("loading")}</p>
      ) : rows.length === 0 ? (
        <p className="text-slate-500">{t("empty")}</p>
      ) : (
        <table className="w-full border-collapse text-left">
          <thead>
            <tr className="border-b border-slate-200 text-slate-600">
              <th className="py-2">{t("table.respondent")}</th>
              <th className="py-2">{t("table.status")}</th>
              <th className="py-2">{t("table.sentAt")}</th>
              <th className="py-2">{t("table.activity")}</th>
              <th className="py-2"></th>
            </tr>
          </thead>
          <tbody>
            {rows.map((q) => (
              <tr key={q.id} className="border-b border-slate-100">
                <td className="py-2">{q.respondent_name}</td>
                <td className="py-2"><span className={`rounded px-2 py-0.5 text-xs ${STATUS_STYLE[q.status] ?? ""}`}>{t(`statuses.${q.status}`)}</span></td>
                <td className="py-2 text-slate-500">{formatDate(q.created_at, locale, { day: "numeric", month: "short", year: "numeric" })}</td>
                <td className="py-2">
                  {q.activity_id ? <Link className="text-sky-700 underline" href={`/ropa/activities/${q.activity_id}`}>{t("table.viewActivity")}</Link> : "—"}
                </td>
                <td className="py-2 text-right">
                  {canApprove && q.status === "answered" && (
                    <span className="flex justify-end gap-2">
                      <Button onClick={() => setApproving(q)} data-testid={`approve-${q.id}`}>{t("approve")}</Button>
                      <Button onClick={() => reject.mutate(q)} className="bg-slate-200 text-slate-700 hover:bg-slate-300" data-testid={`reject-${q.id}`}>{t("reject")}</Button>
                    </span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </main>
  );
}
