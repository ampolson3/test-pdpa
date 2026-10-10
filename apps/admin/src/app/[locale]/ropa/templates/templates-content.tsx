"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useTemplateSets,
  useActivityTemplates,
  useActivityTemplate,
  useActivityMutations,
  useLegalEntities,
  useOrgUnits,
  type RopaActivityTemplate,
} from "@pdpa/api-client";
import { Link, useRouter } from "@/i18n/routing";

type Purpose = { purpose_text_th?: string; purpose_text_en?: string; lawful_basis_code?: string };
type DataItem = { data_category_code?: string; subject_type_code?: string; source?: string };
type Recipient = { recipient_role?: string; note_th?: string };
type Retention = { retention_months?: number; retention_basis_th?: string; trigger_event_th?: string; disposal_method?: string };

function arr<T>(v: unknown): T[] {
  return Array.isArray(v) ? (v as T[]) : [];
}

function str(v: unknown): string {
  return typeof v === "string" ? v : "";
}

/** RTG-01: pick a job category, browse its standard activities, and view each one's ม.39 defaults. */
export function TemplatesContent() {
  const t = useTranslations("ropaTemplates");
  const allowed = usePermission("ropa.template.read");
  const canCreateBatch = usePermission("ropa.activity.create");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const [jobCategory, setJobCategory] = useState("");
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const sets = useTemplateSets(client);
  const standardSet = sets.data?.find((s) => s.set_type === "standard");
  const all = useActivityTemplates(client, standardSet?.id);
  const filtered = useActivityTemplates(client, standardSet?.id, jobCategory || undefined);
  const detail = useActivityTemplate(client, selectedId ?? undefined);

  if (!allowed) {
    return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;
  }

  const categories = Array.from(new Set((all.data ?? []).map((a) => a.job_category))).sort();
  const list = jobCategory ? filtered.data : all.data;

  return (
    <main className="mx-auto max-w-5xl space-y-4 p-8">
      <header className="flex items-start justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{t("title")}</h1>
          <p className="text-sm text-slate-600">{t("description")}</p>
        </div>
        {canCreateBatch && (
          <Link className="rounded-md border border-slate-300 bg-white px-3 py-2 text-sm hover:bg-slate-50" href="/ropa/templates/batch">
            {t("batchTitle")}
          </Link>
        )}
      </header>

      <div className="grid gap-4 sm:grid-cols-[260px_1fr]">
        <div className="space-y-3">
          <label className="text-sm">
            <span className="block text-slate-600">{t("jobCategory")}</span>
            <select
              className="mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1"
              value={jobCategory}
              onChange={(e) => { setJobCategory(e.target.value); setSelectedId(null); }}
            >
              <option value="">{t("allCategories")}</option>
              {categories.map((c) => (
                <option key={c} value={c}>
                  {t.has(`categories.${c}`) ? t(`categories.${c}`) : c}
                </option>
              ))}
            </select>
          </label>

          {all.isPending ? (
            <p className="text-slate-500">{t("loading")}</p>
          ) : all.isError ? (
            <p className="text-red-700">{t("loadError")}</p>
          ) : (list ?? []).length === 0 ? (
            <p className="text-slate-500">{t("empty")}</p>
          ) : (
            <ul className="divide-y divide-slate-100 rounded-md border border-slate-200 bg-white" data-testid="template-list">
              {list!.map((a) => (
                <li key={a.id}>
                  <button
                    type="button"
                    className={`block w-full px-3 py-2 text-left text-sm hover:bg-slate-50 ${selectedId === a.id ? "bg-sky-50 font-medium" : ""}`}
                    onClick={() => setSelectedId(a.id)}
                  >
                    {a.name_th}
                    <span className="block text-xs text-slate-500">{a.role === "processor" ? t("roleProcessor") : t("roleController")}</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div>
          {!selectedId ? (
            <p className="text-slate-500">{t("selectPrompt")}</p>
          ) : detail.isPending ? (
            <p className="text-slate-500">{t("loading")}</p>
          ) : detail.isError || !detail.data ? (
            <p className="text-red-700">{t("loadError")}</p>
          ) : (
            <ActivityDetail activity={detail.data} />
          )}
        </div>
      </div>
    </main>
  );
}

function ActivityDetail({ activity }: { activity: RopaActivityTemplate }) {
  const t = useTranslations("ropaTemplates");
  const d = activity.defaults as Record<string, unknown>;
  const r = activity.rationale as Record<string, unknown>;
  const canCreate = usePermission("ropa.activity.create");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const router = useRouter();
  const { createFromTemplate } = useActivityMutations(client);
  const entities = useLegalEntities(client);
  const [legalEntityId, setLegalEntityId] = useState("");
  const [code, setCode] = useState("");
  const units = useOrgUnits(client, legalEntityId || undefined);
  const [orgUnitId, setOrgUnitId] = useState("");
  const [creating, setCreating] = useState(false);

  const submit = () => {
    if (!legalEntityId || !orgUnitId || !code) return;
    createFromTemplate.mutate(
      { activity_template_id: activity.id, legal_entity_id: legalEntityId, org_unit_id: orgUnitId, code },
      { onSuccess: (a) => router.push(`/ropa/activities/${a!.id}`) },
    );
  };

  return (
    <section className="space-y-3 rounded-md border border-slate-200 bg-white p-4 text-sm" data-testid="template-detail">
      <div className="flex items-start justify-between gap-2">
        <div>
          <h2 className="font-semibold">{activity.name_th}</h2>
          {activity.name_en && <p className="text-slate-500">{activity.name_en}</p>}
          {str(d.description_th) && <p className="mt-1 text-slate-600">{str(d.description_th)}</p>}
        </div>
        {canCreate && !creating && (
          <Button onClick={() => { createFromTemplate.reset(); setCreating(true); }}>{t("createFromTemplate")}</Button>
        )}
      </div>

      {creating && (
        <fieldset className="grid gap-2 rounded-md border border-sky-200 bg-sky-50 p-3 sm:grid-cols-2" data-testid="create-from-template-form">
          <legend className="px-1 text-xs font-medium text-sky-800">{t("createFromTemplate")}</legend>
          <label className="text-xs">
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
          <label className="text-xs">
            <span className="block text-slate-600">{t("form.orgUnit")}</span>
            <select className="mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1" value={orgUnitId} onChange={(e) => setOrgUnitId(e.target.value)}>
              <option value="">{t("form.choose")}</option>
              {units.data?.map((u) => <option key={u.id} value={u.id}>{u.name_th}</option>)}
            </select>
          </label>
          <label className="text-xs sm:col-span-2">
            <span className="block text-slate-600">{t("form.code")}</span>
            <input className="mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1" value={code} onChange={(e) => setCode(e.target.value)} maxLength={40} />
          </label>
          {createFromTemplate.isError && (
            <p className="text-red-700 sm:col-span-2" role="alert">
              {(createFromTemplate.error as { detail?: string } | undefined)?.detail ?? t("createError")}
            </p>
          )}
          <div className="flex gap-2 sm:col-span-2">
            <Button onClick={submit} disabled={createFromTemplate.isPending || !legalEntityId || !orgUnitId || !code}>{t("form.save")}</Button>
            <Button variant="secondary" onClick={() => setCreating(false)}>{t("form.cancel")}</Button>
          </div>
        </fieldset>
      )}

      {str(r.note_th) && <p className="rounded bg-amber-50 px-2 py-1 text-xs text-amber-800">{str(r.note_th)}</p>}

      <div>
        <h3 className="font-medium">{t("section.purposes")}</h3>
        <ul className="list-inside list-disc">
          {arr<Purpose>(d.purposes).map((p, i) => (
            <li key={i}>
              {p.purpose_text_th} ({p.lawful_basis_code})
            </li>
          ))}
        </ul>
      </div>

      <div>
        <h3 className="font-medium">{t("section.data")}</h3>
        <ul className="list-inside list-disc">
          {arr<DataItem>(d.data).map((x, i) => (
            <li key={i}>
              {x.data_category_code} — {x.subject_type_code} ({x.source})
            </li>
          ))}
        </ul>
      </div>

      <div>
        <h3 className="font-medium">{t("section.recipients")}</h3>
        {arr<Recipient>(d.recipients).length === 0 ? (
          <p className="text-slate-500">{t("none")}</p>
        ) : (
          <ul className="list-inside list-disc">
            {arr<Recipient>(d.recipients).map((x, i) => (
              <li key={i}>
                {x.recipient_role}: {x.note_th}
              </li>
            ))}
          </ul>
        )}
      </div>

      <div>
        <h3 className="font-medium">{t("section.retention")}</h3>
        <ul className="list-inside list-disc">
          {arr<Retention>(d.retention).map((x, i) => (
            <li key={i}>
              {x.retention_months} {t("months")} — {x.trigger_event_th} ({x.disposal_method})
            </li>
          ))}
        </ul>
      </div>

      <div>
        <h3 className="font-medium">{t("section.securityControls")}</h3>
        <p>{arr<string>(d.security_controls).join(", ")}</p>
      </div>

      {activity.legal_refs.length > 0 && <p className="text-xs text-slate-500">{activity.legal_refs.join(" · ")}</p>}
    </section>
  );
}
