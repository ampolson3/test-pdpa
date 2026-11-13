"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useTemplateSets,
  useActivityTemplates,
  useActivityMutations,
  useLegalEntities,
  useOrgUnits,
  type RopaActivityTemplate,
} from "@pdpa/api-client";
import { Link, useRouter } from "@/i18n/routing";

type Step = "category" | "activities" | "department" | "summary";

/** RTG-04: pick a job category → pick several of its activities → pick a department, then create all
 * drafts in one call — the module doc's own "wizard 3 ขั้นตอน + สรุปก่อนสร้าง" (3 steps + a summary). */
export function BatchWizardContent() {
  const t = useTranslations("ropaTemplates");
  const canCreate = usePermission("ropa.activity.create");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const router = useRouter();

  const [step, setStep] = useState<Step>("category");
  const [jobCategory, setJobCategory] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [legalEntityId, setLegalEntityId] = useState("");
  const [orgUnitId, setOrgUnitId] = useState("");

  const sets = useTemplateSets(client);
  const standardSet = sets.data?.find((s) => s.set_type === "standard");
  const all = useActivityTemplates(client, standardSet?.id);
  const byCategory = useActivityTemplates(client, standardSet?.id, jobCategory || undefined);
  const { createBatchFromTemplates } = useActivityMutations(client);
  const entities = useLegalEntities(client);
  const units = useOrgUnits(client, legalEntityId || undefined);

  if (!canCreate) {
    return <main className="mx-auto max-w-3xl p-8 text-slate-600">{t("forbidden")}</main>;
  }

  const categories = Array.from(new Set((all.data ?? []).map((a) => a.job_category))).sort();
  const activities = byCategory.data ?? [];
  const chosen = (all.data ?? []).filter((a) => selected.has(a.id));

  const toggle = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const create = () => {
    if (!orgUnitId || chosen.length === 0) return;
    createBatchFromTemplates.mutate(
      { org_unit_id: orgUnitId, activity_template_ids: chosen.map((a) => a.id) },
      { onSuccess: () => router.push("/ropa/activities") },
    );
  };

  return (
    <main className="mx-auto max-w-3xl space-y-6 p-8">
      <header className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">{t("batchTitle")}</h1>
          <p className="text-sm text-slate-600">{t("batchDescription")}</p>
        </div>
        <Link className="text-sm text-sky-700 underline" href="/ropa/templates">
          {t("backToBrowse")}
        </Link>
      </header>

      <ol className="flex gap-4 text-sm text-slate-500" data-testid="batch-wizard-steps">
        {(["category", "activities", "department", "summary"] as Step[]).map((s, i) => (
          <li key={s} className={step === s ? "font-semibold text-sky-700" : undefined}>
            {i + 1}. {t(`batchStep.${s}`)}
          </li>
        ))}
      </ol>

      {step === "category" && (
        <section className="space-y-3">
          <label className="block text-sm">
            <span className="block text-slate-600">{t("jobCategory")}</span>
            <select
              className="mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1"
              value={jobCategory}
              onChange={(e) => setJobCategory(e.target.value)}
            >
              <option value="">{t("form.choose")}</option>
              {categories.map((c) => (
                <option key={c} value={c}>
                  {t.has(`categories.${c}`) ? t(`categories.${c}`) : c}
                </option>
              ))}
            </select>
          </label>
          <Button onClick={() => setStep("activities")} disabled={!jobCategory}>
            {t("batchNext")}
          </Button>
        </section>
      )}

      {step === "activities" && (
        <section className="space-y-3">
          {byCategory.isPending ? (
            <p className="text-slate-500">{t("loading")}</p>
          ) : activities.length === 0 ? (
            <p className="text-slate-500">{t("empty")}</p>
          ) : (
            <ul className="divide-y divide-slate-100 rounded-md border border-slate-200 bg-white" data-testid="batch-activity-list">
              {activities.map((a: RopaActivityTemplate) => (
                <li key={a.id} className="flex items-center gap-2 px-3 py-2 text-sm">
                  <input type="checkbox" checked={selected.has(a.id)} onChange={() => toggle(a.id)} id={`tpl-${a.id}`} />
                  <label htmlFor={`tpl-${a.id}`} className="flex-1">
                    {a.name_th}
                    <span className="ml-2 text-xs text-slate-500">{a.role === "processor" ? t("roleProcessor") : t("roleController")}</span>
                  </label>
                </li>
              ))}
            </ul>
          )}
          <p className="text-xs text-slate-500">{t("batchSelectedCount", { count: selected.size })}</p>
          <div className="flex gap-2">
            <Button variant="secondary" onClick={() => setStep("category")}>{t("batchBack")}</Button>
            <Button onClick={() => setStep("department")} disabled={selected.size === 0}>{t("batchNext")}</Button>
          </div>
        </section>
      )}

      {step === "department" && (
        <section className="space-y-3">
          <label className="block text-sm">
            <span className="block text-slate-600">{t("form.legalEntity")}</span>
            <select
              className="mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1"
              value={legalEntityId}
              onChange={(e) => { setLegalEntityId(e.target.value); setOrgUnitId(""); }}
            >
              <option value="">{t("form.choose")}</option>
              {entities.data?.map((le) => <option key={le.id} value={le.id}>{le.name_th}</option>)}
            </select>
          </label>
          <label className="block text-sm">
            <span className="block text-slate-600">{t("form.orgUnit")}</span>
            <select
              className="mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1"
              value={orgUnitId}
              onChange={(e) => setOrgUnitId(e.target.value)}
              disabled={!legalEntityId}
            >
              <option value="">{t("form.choose")}</option>
              {units.data?.map((u) => <option key={u.id} value={u.id}>{u.name_th}</option>)}
            </select>
          </label>
          <div className="flex gap-2">
            <Button variant="secondary" onClick={() => setStep("activities")}>{t("batchBack")}</Button>
            <Button onClick={() => setStep("summary")} disabled={!orgUnitId}>{t("batchNext")}</Button>
          </div>
        </section>
      )}

      {step === "summary" && (
        <section className="space-y-3" data-testid="batch-summary">
          <p className="text-sm text-slate-700">{t("batchSummaryIntro", { count: chosen.length })}</p>
          <ul className="list-inside list-disc rounded-md border border-slate-200 bg-white p-3 text-sm">
            {chosen.map((a) => <li key={a.id}>{a.name_th}</li>)}
          </ul>
          {createBatchFromTemplates.isError && (
            <p className="text-red-700" role="alert">
              {(createBatchFromTemplates.error as { detail?: string } | undefined)?.detail ?? t("createError")}
            </p>
          )}
          <div className="flex gap-2">
            <Button variant="secondary" onClick={() => setStep("department")}>{t("batchBack")}</Button>
            <Button onClick={create} disabled={createBatchFromTemplates.isPending}>{t("batchCreate")}</Button>
          </div>
        </section>
      )}
    </main>
  );
}
