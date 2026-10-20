"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { Link } from "@/i18n/routing";
import {
  createApiClient,
  useRiskRegister,
  useSaveRiskRegisterEntry,
  type RiskRegisterEntry,
} from "@pdpa/api-client";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";
const STATUSES = ["open", "in_treatment", "accepted", "closed"] as const;
const SOURCE_TYPES = ["dpia", "activity", "vendor", "breach", "audit", "manual"] as const;
const TREATMENTS = ["mitigate", "accept", "transfer", "avoid"] as const;

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

type Draft = {
  id?: string;
  rowVersion?: number;
  title: string;
  description: string;
  likelihood: string;
  impact: string;
  status: (typeof STATUSES)[number];
  treatment: "" | (typeof TREATMENTS)[number];
};

const blank: Draft = { title: "", description: "", likelihood: "1", impact: "1", status: "open", treatment: "" };

function fromRisk(r: RiskRegisterEntry): Draft {
  return {
    id: r.id, rowVersion: r.row_version, title: r.title, description: r.description ?? "",
    likelihood: String(r.likelihood), impact: String(r.impact), status: r.status as Draft["status"],
    treatment: (r.treatment ?? "") as Draft["treatment"],
  };
}

/** RRA-09's own screen: risks from DPIA, RoPA activities and vendors — plus anything recorded directly
 *  here (source "manual") — in one shared register, filterable by status/source. Links to an activity,
 *  asset or vendor are made by that module's own feature (DPIA-06/07, ROPA, vendor assessment) when it
 *  first identifies the risk — this page's own create form only records a plain manual risk; editing
 *  here never touches another module's link (the backend's own UpdateRisk never does). */
export function RiskRegisterContent() {
  const t = useTranslations("riskRegister");
  const canRead = usePermission("dpo.risk.read");
  const canCreate = usePermission("dpo.risk.create");
  const canUpdate = usePermission("dpo.risk.update");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const [status, setStatus] = useState<string>("");
  const [sourceType, setSourceType] = useState<string>("");
  const list = useRiskRegister(client, { status: status || undefined, source_type: sourceType || undefined });
  const save = useSaveRiskRegisterEntry(client);
  const [draft, setDraft] = useState<Draft | null>(null);

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  const rows = list.data?.pages.flatMap((p) => p.data) ?? [];

  const submit = () => {
    if (!draft || !draft.title.trim()) return;
    save.mutate(
      {
        id: draft.id,
        rowVersion: draft.rowVersion,
        input: {
          source_type: "manual",
          title: draft.title,
          description: draft.description || undefined,
          likelihood: Number(draft.likelihood),
          impact: Number(draft.impact),
          status: draft.status,
          treatment: draft.treatment || undefined,
        },
      },
      { onSuccess: () => setDraft(null) },
    );
  };

  return (
    <main className="mx-auto max-w-5xl space-y-6 p-8 text-sm">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{t("title")}</h1>
          <p className="text-slate-600">{t("intro")}</p>
        </div>
        {canCreate && <Button onClick={() => { save.reset(); setDraft({ ...blank }); }} data-testid="new-risk">{t("newRisk")}</Button>}
      </header>

      <div className="flex flex-wrap gap-4" data-testid="risk-filters">
        <label><span className="block text-slate-600">{t("filters.status")}</span>
          <select className={INPUT} value={status} onChange={(e) => setStatus(e.target.value)}>
            <option value="">{t("filters.all")}</option>
            {STATUSES.map((s) => <option key={s} value={s}>{t(`statuses.${s}`)}</option>)}
          </select>
        </label>
        <label><span className="block text-slate-600">{t("filters.sourceType")}</span>
          <select className={INPUT} value={sourceType} onChange={(e) => setSourceType(e.target.value)}>
            <option value="">{t("filters.all")}</option>
            {SOURCE_TYPES.map((s) => <option key={s} value={s}>{t(`sourceTypes.${s}`)}</option>)}
          </select>
        </label>
      </div>

      {draft && (
        <fieldset className="grid gap-3 rounded-md border border-slate-200 bg-white p-4 sm:grid-cols-2" data-testid="risk-form">
          <legend className="px-1 font-semibold">{draft.id ? t("form.editTitle") : t("form.newTitle")}</legend>
          <label className="sm:col-span-2"><span className="block text-slate-600">{t("form.title")}</span>
            <input className={INPUT} value={draft.title} onChange={(e) => setDraft({ ...draft, title: e.target.value })} /></label>
          <label className="sm:col-span-2"><span className="block text-slate-600">{t("form.description")}</span>
            <textarea className={INPUT} value={draft.description} onChange={(e) => setDraft({ ...draft, description: e.target.value })} /></label>
          <label><span className="block text-slate-600">{t("form.likelihood")}</span>
            <input className={INPUT} type="number" min={1} max={5} value={draft.likelihood} onChange={(e) => setDraft({ ...draft, likelihood: e.target.value })} /></label>
          <label><span className="block text-slate-600">{t("form.impact")}</span>
            <input className={INPUT} type="number" min={1} max={5} value={draft.impact} onChange={(e) => setDraft({ ...draft, impact: e.target.value })} /></label>
          <label><span className="block text-slate-600">{t("form.status")}</span>
            <select className={INPUT} value={draft.status} onChange={(e) => setDraft({ ...draft, status: e.target.value as Draft["status"] })}>
              {STATUSES.map((s) => <option key={s} value={s}>{t(`statuses.${s}`)}</option>)}
            </select></label>
          <label><span className="block text-slate-600">{t("form.treatment")}</span>
            <select className={INPUT} value={draft.treatment} onChange={(e) => setDraft({ ...draft, treatment: e.target.value as Draft["treatment"] })}>
              <option value="">{t("filters.all")}</option>
              {TREATMENTS.map((tr) => <option key={tr} value={tr}>{t(`treatments.${tr}`)}</option>)}
            </select></label>
          {draft.id && <p className="text-xs text-slate-500 sm:col-span-2">{t("form.linksNote")}</p>}

          {save.isError && <p className="text-red-700 sm:col-span-2" role="alert">{t("form.saveError", { detail: detail(save.error) })}</p>}
          <div className="flex gap-2 sm:col-span-2">
            <Button onClick={submit} disabled={save.isPending || !draft.title.trim()}>{t("form.save")}</Button>
            <Button variant="secondary" onClick={() => { save.reset(); setDraft(null); }}>{t("form.cancel")}</Button>
          </div>
        </fieldset>
      )}

      {list.isPending ? (
        <p className="text-slate-500">{t("loading")}</p>
      ) : list.isError ? (
        <p className="text-red-700">{t("loadError")}</p>
      ) : rows.length === 0 ? (
        <p className="text-slate-500">{t("empty")}</p>
      ) : (
        <table className="w-full border-collapse text-left" data-testid="risks-table">
          <thead>
            <tr className="border-b border-slate-200 text-slate-500">
              <th className="py-2">{t("table.title")}</th>
              <th className="py-2">{t("table.source")}</th>
              <th className="py-2">{t("table.level")}</th>
              <th className="py-2">{t("table.score")}</th>
              <th className="py-2">{t("table.status")}</th>
              <th className="py-2">{t("table.linkedTo")}</th>
              <th className="py-2" />
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.id} className="border-b border-slate-100">
                <td className="py-2">{r.title}</td>
                <td className="py-2">{t(`sourceTypes.${r.source_type}`)}</td>
                <td className="py-2">{t(`levels.${r.level}`)}</td>
                <td className="py-2">{r.inherent_score}</td>
                <td className="py-2">{t(`statuses.${r.status}`)}</td>
                <td className="py-2">
                  {r.activity_id ? (
                    <Link href={`/ropa/activities/${r.activity_id}`} className="text-sky-700 underline">
                      {t("table.viewActivity")}
                    </Link>
                  ) : null}
                </td>
                <td className="py-2">
                  {canUpdate && r.source_type === "manual" && (
                    <button className="text-sky-700 underline" onClick={() => { save.reset(); setDraft(fromRisk(r)); }}>{t("edit")}</button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {list.hasNextPage && (
        <Button variant="secondary" onClick={() => list.fetchNextPage()} disabled={list.isFetchingNextPage}>
          {t("loading")}
        </Button>
      )}
    </main>
  );
}
