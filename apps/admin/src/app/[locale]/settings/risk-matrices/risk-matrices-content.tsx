"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useRiskMatrices,
  useSaveRiskMatrix,
  useDeleteRiskMatrix,
  type RiskMatrix,
  type RiskLevel,
} from "@pdpa/api-client";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";
const LEVELS: RiskLevel[] = ["low", "medium", "high", "very_high"];

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

type ThresholdDraft = { level: RiskLevel; min_score: string };
type Draft = {
  id?: string;
  rowVersion?: number;
  name: string;
  likelihood_levels: string;
  impact_levels: string;
  is_default: boolean;
  thresholds: ThresholdDraft[];
};

const blankThreshold: ThresholdDraft = { level: "low", min_score: "1" };
const blank: Draft = { name: "", likelihood_levels: "", impact_levels: "", is_default: false, thresholds: [{ ...blankThreshold }] };

function fromMatrix(m: RiskMatrix): Draft {
  return {
    id: m.id, rowVersion: m.row_version, name: m.name, is_default: m.is_default,
    likelihood_levels: m.likelihood_levels.join(", "), impact_levels: m.impact_levels.join(", "),
    thresholds: m.thresholds.map((t) => ({ level: t.level, min_score: String(t.min_score) })),
  };
}

/** RRA-02's own settings screen: configure a tenant's likelihood x impact risk matrix — the shared "risk
 *  engine" DPIA/vendor/breach will all read from once they compute a score against it (none do yet). */
export function RiskMatricesContent() {
  const t = useTranslations("riskMatrices");
  const canRead = usePermission("ropa.risk.read");
  const canCreate = usePermission("ropa.risk.create");
  const canUpdate = usePermission("ropa.risk.update");
  const canDelete = usePermission("ropa.risk.delete");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const list = useRiskMatrices(client);
  const save = useSaveRiskMatrix(client);
  const remove = useDeleteRiskMatrix(client);
  const [draft, setDraft] = useState<Draft | null>(null);

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  const rows = list.data ?? [];

  const addThreshold = () => draft && setDraft({ ...draft, thresholds: [...draft.thresholds, { ...blankThreshold }] });
  const removeThreshold = (i: number) => draft && setDraft({ ...draft, thresholds: draft.thresholds.filter((_, idx) => idx !== i) });
  const setThreshold = (i: number, v: Partial<ThresholdDraft>) =>
    draft && setDraft({ ...draft, thresholds: draft.thresholds.map((th, idx) => (idx === i ? { ...th, ...v } : th)) });

  const submit = () => {
    if (!draft || !draft.name.trim()) return;
    const likelihood = draft.likelihood_levels.split(",").map((s) => s.trim()).filter(Boolean);
    const impact = draft.impact_levels.split(",").map((s) => s.trim()).filter(Boolean);
    const thresholds = draft.thresholds.map((th) => ({ level: th.level, min_score: Number(th.min_score) }));
    save.mutate(
      {
        id: draft.id, rowVersion: draft.rowVersion,
        input: { name: draft.name, likelihood_levels: likelihood, impact_levels: impact, thresholds, is_default: draft.is_default },
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
        {canCreate && <Button onClick={() => { save.reset(); setDraft({ ...blank, thresholds: [{ ...blankThreshold }] }); }} data-testid="new-matrix">{t("newMatrix")}</Button>}
      </header>

      {draft && (
        <fieldset className="grid gap-3 rounded-md border border-slate-200 bg-white p-4 sm:grid-cols-2" data-testid="matrix-form">
          <legend className="px-1 font-semibold">{draft.id ? t("form.editTitle") : t("form.newTitle")}</legend>
          <label className="sm:col-span-2"><span className="block text-slate-600">{t("form.name")}</span>
            <input className={INPUT} value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} /></label>
          <label><span className="block text-slate-600">{t("form.likelihoodLevels")}</span>
            <input className={INPUT} value={draft.likelihood_levels} placeholder={t("form.levelsPlaceholder")}
              onChange={(e) => setDraft({ ...draft, likelihood_levels: e.target.value })} /></label>
          <label><span className="block text-slate-600">{t("form.impactLevels")}</span>
            <input className={INPUT} value={draft.impact_levels} placeholder={t("form.levelsPlaceholder")}
              onChange={(e) => setDraft({ ...draft, impact_levels: e.target.value })} /></label>
          <label className="flex items-center gap-2 sm:col-span-2">
            <input type="checkbox" checked={draft.is_default} onChange={(e) => setDraft({ ...draft, is_default: e.target.checked })} />
            <span className="text-slate-600">{t("form.isDefault")}</span>
          </label>

          <div className="sm:col-span-2">
            <span className="block text-slate-600">{t("form.thresholds")}</span>
            <ul className="mt-1 space-y-1" data-testid="threshold-list">
              {draft.thresholds.map((th, i) => (
                <li key={i} className="flex items-center gap-2">
                  <select className={INPUT + " w-auto"} value={th.level} onChange={(e) => setThreshold(i, { level: e.target.value as RiskLevel })}>
                    {LEVELS.map((lv) => <option key={lv} value={lv}>{t(`levels.${lv}`)}</option>)}
                  </select>
                  <span className="text-slate-500">{t("form.minScore")}</span>
                  <input className={INPUT + " w-24"} type="number" min={0} value={th.min_score} onChange={(e) => setThreshold(i, { min_score: e.target.value })} />
                  <button type="button" className="text-slate-500" onClick={() => removeThreshold(i)}>×</button>
                </li>
              ))}
            </ul>
            <Button variant="secondary" onClick={addThreshold}>{t("form.addThreshold")}</Button>
          </div>

          {save.isError && <p className="text-red-700 sm:col-span-2" role="alert">{t("form.saveError", { detail: detail(save.error) })}</p>}
          <div className="flex gap-2 sm:col-span-2">
            <Button onClick={submit} disabled={save.isPending || !draft.name.trim()}>{t("form.save")}</Button>
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
        <table className="w-full border-collapse text-left" data-testid="matrices-table">
          <thead>
            <tr className="border-b border-slate-200 text-slate-500">
              <th className="py-2">{t("table.name")}</th>
              <th className="py-2">{t("table.shape")}</th>
              <th className="py-2">{t("table.default")}</th>
              <th className="py-2" />
            </tr>
          </thead>
          <tbody>
            {rows.map((m) => (
              <tr key={m.id} className="border-b border-slate-100">
                <td className="py-2">{m.name}</td>
                <td className="py-2">{m.likelihood_levels.length}×{m.impact_levels.length}</td>
                <td className="py-2">{m.is_default ? t("table.yes") : ""}</td>
                <td className="py-2 space-x-2">
                  {canUpdate && <button className="text-sky-700 underline" onClick={() => { save.reset(); setDraft(fromMatrix(m)); }}>{t("edit")}</button>}
                  {canDelete && (
                    <button className="text-red-700 underline" onClick={() => remove.mutate({ id: m.id, rowVersion: m.row_version })}>{t("delete")}</button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {remove.isError && <p className="text-red-700" role="alert">{t("form.saveError", { detail: detail(remove.error) })}</p>}
    </main>
  );
}
