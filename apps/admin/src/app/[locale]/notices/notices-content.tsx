"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useActivities,
  useCreateNoticeWizard,
  useLegalEntities,
  useNotices,
  type NoticeType,
} from "@pdpa/api-client";
import { Link, useRouter } from "@/i18n/routing";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";
const NOTICE_TYPES: NoticeType[] = ["privacy_notice", "privacy_policy", "cookie_policy", "cctv", "layered_short", "employee"];
const SLUG_RE = /^[a-z0-9-]+$/;

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

type Draft = { legal_entity_id: string; notice_type: NoticeType | ""; title: string; slug: string; activity_ids: string[] };
const blank: Draft = { legal_entity_id: "", notice_type: "", title: "", slug: "", activity_ids: [] };

export function NoticesContent() {
  const t = useTranslations("notices");
  const canRead = usePermission("notice.document.read");
  const canCreate = usePermission("notice.document.create");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const router = useRouter();
  const [draft, setDraft] = useState<Draft | null>(null);

  const list = useNotices(client, {});
  const wizard = useCreateNoticeWizard(client);
  const entities = useLegalEntities(client);
  const activities = useActivities(client, {});

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  const rows = list.data?.pages.flatMap((p) => p.data) ?? [];
  const activityRows = activities.data?.pages.flatMap((p) => p.data) ?? [];
  const set = (p: Partial<Draft>) => setDraft({ ...(draft ?? blank), ...p });
  const slugValid = !draft || draft.slug === "" || SLUG_RE.test(draft.slug);

  const submit = () => {
    if (!draft || !draft.notice_type || !slugValid) return;
    wizard.mutate(
      { legal_entity_id: draft.legal_entity_id, notice_type: draft.notice_type, title: draft.title, slug: draft.slug, activity_ids: draft.activity_ids },
      { onSuccess: (n) => { setDraft(null); router.push(`/documents/${n!.document_id}`); } },
    );
  };

  const toggleActivity = (id: string) => {
    if (!draft) return;
    const has = draft.activity_ids.includes(id);
    set({ activity_ids: has ? draft.activity_ids.filter((x) => x !== id) : [...draft.activity_ids, id] });
  };

  return (
    <main className="mx-auto max-w-6xl space-y-6 p-8 text-sm">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{t("title")}</h1>
          <p className="text-slate-600">{t("intro")}</p>
        </div>
        {canCreate && <Button onClick={() => { wizard.reset(); setDraft({ ...blank }); }} data-testid="new-notice">{t("newNotice")}</Button>}
      </header>

      {draft && (
        <fieldset className="grid gap-3 rounded-md border border-slate-200 bg-white p-4 sm:grid-cols-2" disabled={!canCreate}>
          <legend className="px-1 font-semibold">{t("wizard.title")}</legend>
          <p className="text-slate-600 sm:col-span-2">{t("wizard.intro")}</p>
          <label><span className="block text-slate-600">{t("form.legalEntity")}</span>
            <select className={INPUT} value={draft.legal_entity_id} onChange={(e) => set({ legal_entity_id: e.target.value })}>
              <option value="">{t("form.choose")}</option>
              {entities.data?.map((le) => <option key={le.id} value={le.id}>{le.name_th}</option>)}
            </select>
          </label>
          <label><span className="block text-slate-600">{t("form.noticeType")}</span>
            <select className={INPUT} value={draft.notice_type} onChange={(e) => set({ notice_type: e.target.value as NoticeType })}>
              <option value="">{t("form.choose")}</option>
              {NOTICE_TYPES.map((n) => <option key={n} value={n}>{t(`types.${n}`)}</option>)}
            </select>
          </label>
          <label><span className="block text-slate-600">{t("form.noticeTitle")}</span>
            <input className={INPUT} value={draft.title} onChange={(e) => set({ title: e.target.value })} maxLength={300} /></label>
          <label><span className="block text-slate-600">{t("form.slug")}</span>
            <input className={INPUT} value={draft.slug} onChange={(e) => set({ slug: e.target.value })} maxLength={120}
              aria-invalid={!slugValid} placeholder="employee-privacy-notice" />
            {!slugValid && <span className="text-xs text-red-700">{t("form.slugInvalid")}</span>}
          </label>
          <div className="sm:col-span-2">
            <span className="block text-slate-600">{t("form.activities")}</span>
            <p className="mb-1 text-xs text-slate-500">{t("form.activitiesHint")}</p>
            <div className="max-h-48 space-y-1 overflow-auto rounded-md border border-slate-200 p-2" data-testid="activity-picker">
              {activityRows.length === 0 && <p className="text-slate-500">{t("form.noActivities")}</p>}
              {activityRows.map((a) => (
                <label key={a.id} className="flex items-center gap-2">
                  <input type="checkbox" checked={draft.activity_ids.includes(a.id)} onChange={() => toggleActivity(a.id)} />
                  <span>{a.code} — {a.name}</span>
                </label>
              ))}
            </div>
          </div>
          {wizard.isError && <p className="text-red-700 sm:col-span-2" role="alert">{t("form.saveError", { detail: detail(wizard.error) })}</p>}
          <div className="flex gap-2 sm:col-span-2">
            <Button onClick={submit} disabled={wizard.isPending || !draft.legal_entity_id || !draft.notice_type || !draft.title || !draft.slug || !slugValid}>
              {t("wizard.generate")}
            </Button>
            <Button variant="secondary" onClick={() => { wizard.reset(); setDraft(null); }}>{t("form.cancel")}</Button>
          </div>
        </fieldset>
      )}

      {list.isPending ? <p className="text-slate-500">{t("loading")}</p> : list.isError ? <p className="text-red-700">{t("loadError")}</p> : rows.length === 0 ? (
        <p className="rounded-md bg-amber-50 p-3 text-amber-800">{t("empty")}</p>
      ) : (
        <table className="w-full rounded-md border border-slate-200 bg-white" data-testid="notices-list">
          <thead className="bg-slate-50 text-left text-slate-600">
            <tr><th className="px-3 py-2">{t("form.noticeTitle")}</th><th className="px-3 py-2">{t("form.noticeType")}</th>
              <th className="px-3 py-2">{t("statusLabel")}</th><th className="px-3 py-2">{t("form.slug")}</th></tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.map((n) => (
              <tr key={n.id}>
                <td className="px-3 py-2"><Link className="text-sky-700 underline" href={`/documents/${n.document_id}`}>{n.title}</Link></td>
                <td className="px-3 py-2">{t(`types.${n.notice_type}`)}</td>
                <td className="px-3 py-2">{t(`statuses.${n.status}`)}</td>
                <td className="px-3 py-2 font-mono text-xs">{n.slug}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {list.hasNextPage && <Button variant="secondary" onClick={() => list.fetchNextPage()}>{t("more")}</Button>}
    </main>
  );
}
