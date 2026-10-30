"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useTemplateSets,
  useActivityTemplates,
  useTemplateSuggestions,
  useActivityMutations,
} from "@pdpa/api-client";

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

const SELECT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";

/** RTG-06: lets the OWNER pick an RTG-01 standard activity close to this one and review its purposes/
 *  data/retention one item at a time, each with a rationale citing a real legal article — only the
 *  items checked and confirmed via "apply" are ever written (the acceptance criterion). This sits
 *  alongside ROPA-05's own "create from template" (which copies everything at once when an activity is
 *  first created) as the per-item, already-created-activity counterpart. */
export function Rtg06SuggestionsSection({ client, activityId }: { client: ReturnType<typeof createApiClient>; activityId: string }) {
  const t = useTranslations("activities");
  const canRead = usePermission("ropa.template.read");
  const canUpdate = usePermission("ropa.activity.update");
  const sets = useTemplateSets(client);
  const setId = sets.data?.[0]?.id;
  const [jobCategory, setJobCategory] = useState("");
  const templates = useActivityTemplates(client, setId, jobCategory || undefined);
  const [templateId, setTemplateId] = useState("");
  const suggestions = useTemplateSuggestions(client, templateId || undefined);
  const { applySuggestedItems } = useActivityMutations(client, activityId);

  const [purposes, setPurposes] = useState<Set<number>>(new Set());
  const [data, setData] = useState<Set<number>>(new Set());
  const [retention, setRetention] = useState<Set<number>>(new Set());

  if (!canRead) return null;

  const toggle = (set: Set<number>, setFn: (s: Set<number>) => void, idx: number) => {
    const next = new Set(set);
    if (next.has(idx)) next.delete(idx);
    else next.add(idx);
    setFn(next);
  };

  const pickTemplate = (id: string) => {
    setTemplateId(id);
    setPurposes(new Set());
    setData(new Set());
    setRetention(new Set());
    applySuggestedItems.reset();
  };

  const apply = () => {
    if (!templateId) return;
    applySuggestedItems.mutate({
      activityId,
      input: {
        activity_template_id: templateId,
        purposes: Array.from(purposes),
        data: Array.from(data),
        retention: Array.from(retention),
      },
    });
  };

  const hasSelection = purposes.size > 0 || data.size > 0 || retention.size > 0;

  return (
    <section className="space-y-3 rounded-md border border-slate-200 bg-white p-4" data-testid="rtg06-suggestions-section">
      <h2 className="font-semibold">{t("sections.suggestions")}</h2>
      <p className="text-slate-600">{t("suggestions.chooseTemplate")}</p>

      <div className="flex flex-wrap gap-3">
        <label className="block">
          <span className="block text-slate-600">{t("suggestions.jobCategory")}</span>
          <input className={SELECT} value={jobCategory} onChange={(e) => setJobCategory(e.target.value)} placeholder={t("suggestions.allCategories")} />
        </label>
        <label className="block min-w-64 flex-1">
          <span className="block text-slate-600">{t("suggestions.template")}</span>
          <select className={SELECT} value={templateId} onChange={(e) => pickTemplate(e.target.value)}>
            <option value="">{t("suggestions.choose")}</option>
            {(templates.data ?? []).map((tpl) => (
              <option key={tpl.id} value={tpl.id}>{tpl.name_th}</option>
            ))}
          </select>
        </label>
      </div>

      {!templateId && <p className="text-slate-500">{t("suggestions.noneYet")}</p>}
      {templateId && suggestions.isPending && <p className="text-slate-500">{t("loading")}</p>}
      {templateId && suggestions.isError && <p className="text-red-700" role="alert">{t("suggestions.loadError")}</p>}

      {suggestions.data && (
        <div className="space-y-4" data-testid="rtg06-suggestions-result">
          <div>
            <h3 className="text-sm font-medium text-slate-700">{t("suggestions.purposesHeading")}</h3>
            {suggestions.data.purposes.length === 0 ? (
              <p className="text-slate-400">{t("suggestions.noItems")}</p>
            ) : (
              <ul className="divide-y divide-slate-100 rounded-md border border-slate-200">
                {suggestions.data.purposes.map((p) => (
                  <li key={p.index} className="flex items-start gap-2 p-2">
                    <input type="checkbox" className="mt-1" checked={purposes.has(p.index)} onChange={() => toggle(purposes, setPurposes, p.index)} />
                    <div>
                      <p className="font-medium">{p.purpose_text} — {p.lawful_basis_name_th}</p>
                      <p className="text-xs text-slate-500">{p.rationale}</p>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </div>

          <div>
            <h3 className="text-sm font-medium text-slate-700">{t("suggestions.dataHeading")}</h3>
            {suggestions.data.data.length === 0 ? (
              <p className="text-slate-400">{t("suggestions.noItems")}</p>
            ) : (
              <ul className="divide-y divide-slate-100 rounded-md border border-slate-200">
                {suggestions.data.data.map((d) => (
                  <li key={d.index} className="flex items-start gap-2 p-2">
                    <input type="checkbox" className="mt-1" checked={data.has(d.index)} onChange={() => toggle(data, setData, d.index)} />
                    <div>
                      <p className="font-medium">
                        {d.data_category_name} / {d.subject_type_name}
                        {d.is_sensitive && <span className="ml-2 rounded bg-red-100 px-1.5 py-0.5 text-xs text-red-800">{t("suggestions.sensitiveTag")}</span>}
                      </p>
                      <p className="text-xs text-slate-500">{d.rationale}</p>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </div>

          <div>
            <h3 className="text-sm font-medium text-slate-700">{t("suggestions.retentionHeading")}</h3>
            {suggestions.data.retention.length === 0 ? (
              <p className="text-slate-400">{t("suggestions.noItems")}</p>
            ) : (
              <ul className="divide-y divide-slate-100 rounded-md border border-slate-200">
                {suggestions.data.retention.map((r) => (
                  <li key={r.index} className="flex items-start gap-2 p-2">
                    <input type="checkbox" className="mt-1" checked={retention.has(r.index)} onChange={() => toggle(retention, setRetention, r.index)} />
                    <div>
                      <p className="font-medium">
                        {r.retention_months != null ? t("suggestions.monthsLabel", { months: r.retention_months }) : "—"} — {r.disposal_method}
                      </p>
                      <p className="text-xs text-slate-500">{r.rationale}</p>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </div>

          {canUpdate && (
            <div className="flex items-center gap-2">
              <Button onClick={apply} disabled={!hasSelection || applySuggestedItems.isPending}>{t("suggestions.apply")}</Button>
              {applySuggestedItems.isError && <p className="text-red-700" role="alert">{t("suggestions.applyError", { detail: detail(applySuggestedItems.error) })}</p>}
              {applySuggestedItems.isSuccess && <p className="text-emerald-700" role="status">{t("suggestions.applySuccess")}</p>}
            </div>
          )}
        </div>
      )}
    </section>
  );
}
