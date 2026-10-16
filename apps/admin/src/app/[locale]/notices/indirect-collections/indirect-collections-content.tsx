"use client";

import { useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { formatDate, type Locale } from "@pdpa/i18n";
import {
  createApiClient,
  useIndirectCollections,
  useIndirectCollectionMutations,
  useExternalParties,
  type IndirectCollectionMethod,
  type IndirectCollectionStatus,
} from "@pdpa/api-client";
import { FileUploader } from "@/components/file-uploader";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";
const METHODS: IndirectCollectionMethod[] = ["email", "sms", "letter", "website", "other"];
const STATUSES: IndirectCollectionStatus[] = ["pending", "notified", "overdue", "exempted"];

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

const STATUS_STYLE: Record<IndirectCollectionStatus, string> = {
  pending: "bg-slate-100 text-slate-700",
  notified: "bg-emerald-100 text-emerald-800",
  overdue: "bg-red-100 text-red-800",
  exempted: "bg-slate-100 text-slate-500",
};

export function IndirectCollectionsContent() {
  const t = useTranslations("indirectCollections");
  const locale = useLocale() as Locale;
  const canRead = usePermission("notice.indirect.read");
  const canCreate = usePermission("notice.indirect.create");
  const canUpdate = usePermission("notice.indirect.update");
  const client = useMemo(() => createApiClient("/api/bff"), []);

  const [statusFilter, setStatusFilter] = useState<IndirectCollectionStatus | "">("");
  const list = useIndirectCollections(client, { status: statusFilter || undefined });
  const parties = useExternalParties(client);
  const m = useIndirectCollectionMutations(client);

  const [draft, setDraft] = useState<{ source_party_id: string; obtained_at: string; subject_count: string } | null>(null);
  const [notifyId, setNotifyId] = useState<string | null>(null);
  const [notifyDraft, setNotifyDraft] = useState<{ method: IndirectCollectionMethod | ""; evidence_file_id: string }>({ method: "", evidence_file_id: "" });

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  const rows = list.data?.pages.flatMap((p) => p.data) ?? [];
  const partyRows = parties.data?.pages.flatMap((p) => p.data) ?? [];

  const register = () => {
    if (!draft || !draft.source_party_id || !draft.obtained_at) return;
    m.register.mutate(
      { source_party_id: draft.source_party_id, obtained_at: draft.obtained_at, subject_count: draft.subject_count ? Number(draft.subject_count) : undefined },
      { onSuccess: () => setDraft(null) },
    );
  };

  const submitNotify = (row: (typeof rows)[number]) => {
    if (!notifyDraft.method || !notifyDraft.evidence_file_id) return;
    m.recordNotice.mutate(
      { collection: row, input: { method: notifyDraft.method, evidence_file_id: notifyDraft.evidence_file_id } },
      { onSuccess: () => { setNotifyId(null); setNotifyDraft({ method: "", evidence_file_id: "" }); } },
    );
  };

  return (
    <main className="mx-auto max-w-6xl space-y-6 p-8 text-sm">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{t("title")}</h1>
          <p className="text-slate-600">{t("intro")}</p>
        </div>
        {canCreate && <Button onClick={() => setDraft({ source_party_id: "", obtained_at: "", subject_count: "" })} data-testid="new-collection">{t("newCollection")}</Button>}
      </header>

      {draft && (
        <fieldset className="grid gap-3 rounded-md border border-slate-200 bg-white p-4 sm:grid-cols-3" disabled={!canCreate}>
          <legend className="px-1 font-semibold">{t("form.title")}</legend>
          <label className="sm:col-span-2"><span className="block text-slate-600">{t("form.sourceParty")}</span>
            <select className={INPUT} value={draft.source_party_id} onChange={(e) => setDraft({ ...draft, source_party_id: e.target.value })}>
              <option value="">{t("form.choose")}</option>
              {partyRows.map((p) => <option key={p.id} value={p.id}>{p.name_th}</option>)}
            </select>
          </label>
          <label><span className="block text-slate-600">{t("form.subjectCount")}</span>
            <input type="number" min={0} className={INPUT} value={draft.subject_count} onChange={(e) => setDraft({ ...draft, subject_count: e.target.value })} /></label>
          <label><span className="block text-slate-600">{t("form.obtainedAt")}</span>
            <input type="date" className={INPUT} value={draft.obtained_at} onChange={(e) => setDraft({ ...draft, obtained_at: e.target.value })} /></label>
          {m.register.isError && <p className="text-red-700 sm:col-span-3" role="alert">{t("form.saveError", { detail: detail(m.register.error) })}</p>}
          <div className="flex gap-2 sm:col-span-3">
            <Button onClick={register} disabled={m.register.isPending || !draft.source_party_id || !draft.obtained_at}>{t("form.save")}</Button>
            <Button variant="secondary" onClick={() => setDraft(null)}>{t("form.cancel")}</Button>
          </div>
        </fieldset>
      )}

      <div className="flex items-center gap-2">
        <span className="text-slate-600">{t("filterStatus")}</span>
        <select className={INPUT + " w-auto"} value={statusFilter} onChange={(e) => setStatusFilter(e.target.value as IndirectCollectionStatus | "")}>
          <option value="">{t("allStatuses")}</option>
          {STATUSES.map((s) => <option key={s} value={s}>{t(`statuses.${s}`)}</option>)}
        </select>
      </div>

      {list.isPending ? <p className="text-slate-500">{t("loading")}</p> : list.isError ? <p className="text-red-700">{t("loadError")}</p> : rows.length === 0 ? (
        <p className="rounded-md bg-amber-50 p-3 text-amber-800">{t("empty")}</p>
      ) : (
        <table className="w-full rounded-md border border-slate-200 bg-white" data-testid="collections-list">
          <thead className="bg-slate-50 text-left text-slate-600">
            <tr><th className="px-3 py-2">{t("form.sourceParty")}</th><th className="px-3 py-2">{t("form.obtainedAt")}</th>
              <th className="px-3 py-2">{t("dueBy")}</th><th className="px-3 py-2">{t("statusLabel")}</th><th className="px-3 py-2" /></tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.map((c) => {
              const party = partyRows.find((p) => p.id === c.source_party_id);
              return (
                <>
                  <tr key={c.id}>
                    <td className="px-3 py-2">{party?.name_th ?? c.source_party_id}</td>
                    <td className="px-3 py-2">{formatDate(c.obtained_at, locale, { day: "numeric", month: "short", year: "numeric" })}</td>
                    <td className="px-3 py-2">{formatDate(c.notify_due_at, locale, { day: "numeric", month: "short", year: "numeric" })}</td>
                    <td className="px-3 py-2"><span className={`rounded px-2 py-0.5 ${STATUS_STYLE[c.status]}`}>{t(`statuses.${c.status}`)}</span></td>
                    <td className="px-3 py-2">
                      {canUpdate && (c.status === "pending" || c.status === "overdue") && (
                        <button type="button" className="text-sky-700 underline" data-testid={`notify-toggle-${c.id}`}
                          onClick={() => { setNotifyId(notifyId === c.id ? null : c.id); setNotifyDraft({ method: "", evidence_file_id: "" }); }}>
                          {notifyId === c.id ? t("form.hide") : t("recordNotice")}
                        </button>
                      )}
                    </td>
                  </tr>
                  {notifyId === c.id && (
                    <tr>
                      <td colSpan={5} className="bg-slate-50 px-3 py-3">
                        <div className="grid gap-2 sm:grid-cols-3">
                          <select className={INPUT} value={notifyDraft.method} onChange={(e) => setNotifyDraft({ ...notifyDraft, method: e.target.value as IndirectCollectionMethod })}>
                            <option value="">{t("form.method")}</option>
                            {METHODS.map((mth) => <option key={mth} value={mth}>{t(`methods.${mth}`)}</option>)}
                          </select>
                          <div className="sm:col-span-2">
                            <FileUploader onUploaded={(f) => setNotifyDraft((d) => ({ ...d, evidence_file_id: f.id }))} />
                          </div>
                          {m.recordNotice.isError && <p className="text-red-700 sm:col-span-3" role="alert">{t("form.saveError", { detail: detail(m.recordNotice.error) })}</p>}
                          <Button onClick={() => submitNotify(c)} disabled={m.recordNotice.isPending || !notifyDraft.method || !notifyDraft.evidence_file_id}>
                            {t("form.save")}
                          </Button>
                        </div>
                      </td>
                    </tr>
                  )}
                </>
              );
            })}
          </tbody>
        </table>
      )}
      {list.hasNextPage && <Button variant="secondary" onClick={() => list.fetchNextPage()}>{t("more")}</Button>}
    </main>
  );
}
