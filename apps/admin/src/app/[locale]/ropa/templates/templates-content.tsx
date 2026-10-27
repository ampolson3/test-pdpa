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
  useOwnTemplateSets,
  useTemplateSetManagement,
  type RopaActivityTemplate,
  type RopaTemplateSet,
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
  const [industry, setIndustry] = useState(""); // "" = the standard, job-category-based set (RTG-01)
  const [jobCategory, setJobCategory] = useState("");
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const sets = useTemplateSets(client);
  const standardSet = sets.data?.find((s) => s.set_type === "standard");
  const industrySets = useMemo(() => (sets.data ?? []).filter((s) => s.set_type === "industry"), [sets.data]);
  // RTG-03: the official DGA/สพร. government-controller template is its own set_type ("government"),
  // a different axis from RTG-02's own generic industry='government' starter set — picked via a
  // sentinel value so it shares the same <select> as the industry picker rather than needing a
  // second control.
  const governmentSet = useMemo(() => (sets.data ?? []).find((s) => s.set_type === "government"), [sets.data]);
  const GOVERNMENT_SENTINEL = "__government__";
  // RTG-07: the processor RoPA template (ประกาศ RoPA ผู้ประมวลผล พ.ศ. 2565) is its own set_type
  // ("processor") — a third axis alongside the industry and government ones, picked the same way via
  // a sentinel value in the same <select>.
  const processorSet = useMemo(() => (sets.data ?? []).find((s) => s.set_type === "processor"), [sets.data]);
  const PROCESSOR_SENTINEL = "__processor__";
  // RTG-02: picking an industry switches the browsed set from RTG-01's standard (job-category) one to
  // that industry's own starter set — the job-category filter below still applies within either set.
  const activeSet =
    industry === GOVERNMENT_SENTINEL ? governmentSet :
    industry === PROCESSOR_SENTINEL ? processorSet :
    industry ? industrySets.find((s) => s.industry === industry) : standardSet;
  const all = useActivityTemplates(client, activeSet?.id);
  const filtered = useActivityTemplates(client, activeSet?.id, jobCategory || undefined);
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
            <span className="block text-slate-600">{t("industryFilter")}</span>
            <select
              className="mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1"
              data-testid="industry-picker"
              value={industry}
              onChange={(e) => { setIndustry(e.target.value); setJobCategory(""); setSelectedId(null); }}
            >
              <option value="">{t("standardSetLabel")}</option>
              {industrySets.map((s) => (
                <option key={s.id} value={s.industry}>
                  {t.has(`industries.${s.industry}`) ? t(`industries.${s.industry}`) : s.industry}
                </option>
              ))}
              {governmentSet && (
                <option value={GOVERNMENT_SENTINEL} data-testid="government-set-option">
                  {t("governmentSetLabel")}
                </option>
              )}
              {processorSet && (
                <option value={PROCESSOR_SENTINEL} data-testid="processor-set-option">
                  {t("processorSetLabel")}
                </option>
              )}
            </select>
          </label>

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

      <CustomTemplatesSection client={client} cloneSourceSetId={activeSet?.id} />
    </main>
  );
}

/** RTG-08: "คัดลอก template กลางมาปรับ และกำหนด template มาตรฐานของกลุ่มบริษัท" — a tenant's own custom
 * template sets, cloned from whichever set is currently browsed or authored from scratch, with full
 * create/publish/retire + add/remove-activity CRUD. Once published, the set shows up on the same
 * published browse list above (ListTemplateSets is set-type-agnostic), so every legal entity under this
 * tenant can immediately create activities from it — no further wiring needed.
 */
function CustomTemplatesSection({ client, cloneSourceSetId }: { client: ReturnType<typeof createApiClient>; cloneSourceSetId?: string }) {
  const t = useTranslations("ropaTemplates.custom");
  const canManage = usePermission("ropa.template.create");
  const canPublish = usePermission("ropa.template.publish");
  const canDelete = usePermission("ropa.template.delete");
  const [open, setOpen] = useState(false);
  const [newSetName, setNewSetName] = useState("");
  const [cloneName, setCloneName] = useState("");
  const [selectedOwnSetId, setSelectedOwnSetId] = useState<string | null>(null);

  const ownSets = useOwnTemplateSets(client);
  const mgmt = useTemplateSetManagement(client);
  const ownActivities = useActivityTemplates(client, selectedOwnSetId ?? undefined);

  if (!canManage) return null;

  const selectedOwnSet = ownSets.data?.find((s) => s.id === selectedOwnSetId);

  return (
    <section className="space-y-3 rounded-md border border-slate-200 bg-white p-4" data-testid="custom-templates-section">
      <button type="button" className="text-left text-sm font-medium text-sky-700" onClick={() => setOpen((v) => !v)} data-testid="custom-templates-toggle">
        {open ? "▾ " : "▸ "}{t("manageToggle")}
      </button>
      {open && (
        <div className="space-y-4 text-sm">
          <div className="grid gap-3 sm:grid-cols-2">
            <fieldset className="space-y-2 rounded-md border border-slate-200 p-3">
              <legend className="px-1 text-xs font-medium text-slate-600">{t("createSet")}</legend>
              <input
                className="w-full rounded-md border border-slate-300 px-2 py-1"
                placeholder={t("setNamePlaceholder")}
                value={newSetName}
                onChange={(e) => setNewSetName(e.target.value)}
              />
              <Button
                disabled={!newSetName || mgmt.create.isPending}
                onClick={() => mgmt.create.mutate({ name: newSetName }, { onSuccess: () => setNewSetName("") })}
              >
                {t("create")}
              </Button>
            </fieldset>

            <fieldset className="space-y-2 rounded-md border border-slate-200 p-3">
              <legend className="px-1 text-xs font-medium text-slate-600">{t("cloneSet")}</legend>
              <input
                className="w-full rounded-md border border-slate-300 px-2 py-1"
                placeholder={t("setNamePlaceholder")}
                value={cloneName}
                onChange={(e) => setCloneName(e.target.value)}
              />
              <Button
                disabled={!cloneName || !cloneSourceSetId || mgmt.clone.isPending}
                onClick={() =>
                  cloneSourceSetId &&
                  mgmt.clone.mutate({ sourceSetId: cloneSourceSetId, body: { name: cloneName } }, { onSuccess: () => setCloneName("") })
                }
              >
                {t("clone")}
              </Button>
            </fieldset>
          </div>

          {(mgmt.create.isError || mgmt.clone.isError) && <p className="text-red-700">{t("saveError")}</p>}

          <div>
            <h3 className="font-medium">{t("ownSets")}</h3>
            {(ownSets.data ?? []).length === 0 ? (
              <p className="text-slate-500">{t("noOwnSets")}</p>
            ) : (
              <ul className="divide-y divide-slate-100 rounded-md border border-slate-200" data-testid="own-template-sets">
                {ownSets.data!.map((s) => (
                  <li key={s.id} className="flex items-center justify-between gap-2 px-3 py-2">
                    <button type="button" className="text-left hover:underline" onClick={() => setSelectedOwnSetId(s.id)}>
                      {s.name} <span className="text-xs text-slate-500">({t(`status.${s.status}`)})</span>
                    </button>
                    <div className="flex gap-2">
                      {canPublish && s.status === "draft" && (
                        <Button variant="secondary" onClick={() => mgmt.publish.mutate(s)}>{t("publish")}</Button>
                      )}
                      {canDelete && s.status !== "retired" && (
                        <Button variant="secondary" onClick={() => mgmt.retire.mutate(s)}>{t("retire")}</Button>
                      )}
                    </div>
                  </li>
                ))}
              </ul>
            )}
            {ownSets.data?.find((s) => s.id === selectedOwnSetId)?.status === "published" && (
              <p className="mt-2 rounded bg-emerald-50 px-2 py-1 text-xs text-emerald-800">{t("publishedHint")}</p>
            )}
          </div>

          {selectedOwnSet && (
            <CustomSetActivities
              client={client}
              set={selectedOwnSet}
              activities={ownActivities.data ?? []}
              addActivity={mgmt.addActivity}
              deleteActivity={mgmt.deleteActivity}
              canDelete={!!canDelete}
            />
          )}
        </div>
      )}
    </section>
  );
}

function CustomSetActivities({
  set,
  activities,
  addActivity,
  deleteActivity,
  canDelete,
}: {
  client: ReturnType<typeof createApiClient>;
  set: RopaTemplateSet;
  activities: RopaActivityTemplate[];
  addActivity: ReturnType<typeof useTemplateSetManagement>["addActivity"];
  deleteActivity: ReturnType<typeof useTemplateSetManagement>["deleteActivity"];
  canDelete: boolean;
}) {
  const t = useTranslations("ropaTemplates.custom");
  const [code, setCode] = useState("");
  const [nameTh, setNameTh] = useState("");
  const [nameEn, setNameEn] = useState("");
  const [jobCategory, setJobCategory] = useState("");
  const [role, setRole] = useState<"controller" | "processor">("controller");
  const [defaultsJson, setDefaultsJson] = useState('{"purposes": [], "data": [], "recipients": [], "retention": [], "security_controls": []}');
  const [rationaleJson, setRationaleJson] = useState("{}");
  const [jsonError, setJsonError] = useState(false);

  const submit = () => {
    let defaults: Record<string, unknown>, rationale: Record<string, unknown>;
    try {
      defaults = JSON.parse(defaultsJson);
      rationale = JSON.parse(rationaleJson);
    } catch {
      setJsonError(true);
      return;
    }
    setJsonError(false);
    addActivity.mutate(
      { setId: set.id, body: { code, name_th: nameTh, name_en: nameEn || undefined, job_category: jobCategory, role, defaults, rationale } },
      { onSuccess: () => { setCode(""); setNameTh(""); setNameEn(""); setJobCategory(""); } },
    );
  };

  return (
    <div className="space-y-3 rounded-md border border-slate-200 p-3" data-testid="custom-set-activities">
      <h4 className="font-medium">{set.name}</h4>
      <ul className="divide-y divide-slate-100">
        {activities.map((a) => (
          <li key={a.id} className="flex items-center justify-between gap-2 py-1 text-xs">
            <span>{a.name_th} ({a.code})</span>
            {canDelete && <Button variant="secondary" onClick={() => deleteActivity.mutate(a)}>{t("remove")}</Button>}
          </li>
        ))}
      </ul>

      <fieldset className="grid gap-2 rounded-md border border-slate-200 p-3 sm:grid-cols-2">
        <legend className="px-1 text-xs font-medium text-slate-600">{t("addActivity")}</legend>
        <label className="text-xs">
          <span className="block text-slate-600">{t("activityCode")}</span>
          <input className="mt-1 w-full rounded-md border border-slate-300 px-2 py-1" value={code} onChange={(e) => setCode(e.target.value)} maxLength={60} />
        </label>
        <label className="text-xs">
          <span className="block text-slate-600">{t("activityNameTh")}</span>
          <input className="mt-1 w-full rounded-md border border-slate-300 px-2 py-1" value={nameTh} onChange={(e) => setNameTh(e.target.value)} />
        </label>
        <label className="text-xs">
          <span className="block text-slate-600">{t("activityNameEn")}</span>
          <input className="mt-1 w-full rounded-md border border-slate-300 px-2 py-1" value={nameEn} onChange={(e) => setNameEn(e.target.value)} />
        </label>
        <label className="text-xs">
          <span className="block text-slate-600">{t("activityRole")}</span>
          <select className="mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1" value={role} onChange={(e) => setRole(e.target.value as "controller" | "processor")}>
            <option value="controller">controller</option>
            <option value="processor">processor</option>
          </select>
        </label>
        <label className="text-xs sm:col-span-2">
          <span className="block text-slate-600">{t("jobCategory")}</span>
          <input className="mt-1 w-full rounded-md border border-slate-300 px-2 py-1" value={jobCategory} onChange={(e) => setJobCategory(e.target.value)} maxLength={40} />
        </label>
        <label className="text-xs sm:col-span-2">
          <span className="block text-slate-600">{t("defaultsJson")}</span>
          <textarea className="mt-1 w-full rounded-md border border-slate-300 px-2 py-1 font-mono" rows={3} value={defaultsJson} onChange={(e) => setDefaultsJson(e.target.value)} />
        </label>
        <label className="text-xs sm:col-span-2">
          <span className="block text-slate-600">{t("rationaleJson")}</span>
          <textarea className="mt-1 w-full rounded-md border border-slate-300 px-2 py-1 font-mono" rows={2} value={rationaleJson} onChange={(e) => setRationaleJson(e.target.value)} />
        </label>
        {(jsonError || addActivity.isError) && (
          <p className="text-red-700 sm:col-span-2" role="alert">{jsonError ? t("invalidJson") : t("saveError")}</p>
        )}
        <div className="sm:col-span-2">
          <Button disabled={!code || !nameTh || !jobCategory || addActivity.isPending} onClick={submit}>{t("save")}</Button>
        </div>
      </fieldset>
    </div>
  );
}

function ActivityDetail({ activity }: { activity: RopaActivityTemplate }) {
  const t = useTranslations("ropaTemplates");
  const d = activity.defaults as Record<string, unknown>;
  const r = activity.rationale as Record<string, unknown>;
  // RTG-09: every template's core content (name/description/purpose) is seeded in both th/en — this
  // toggle is the literal "สลับภาษาในหน้า template" the module doc asks for, switching which language
  // of that already-bilingual content is shown; data/recipient/retention items stay code/Thai-only
  // internal RoPA record text, the same scope ROPA-05's own rights_and_access text already drew.
  const [lang, setLang] = useState<"th" | "en">("th");
  const name = lang === "en" && activity.name_en ? activity.name_en : activity.name_th;
  const description = lang === "en" ? str(d.description_en) : str(d.description_th);
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
          <div className="flex items-center gap-2">
            <h2 className="font-semibold">{name}</h2>
            <div className="flex overflow-hidden rounded border border-slate-300 text-xs" data-testid="template-lang-switch">
              <button
                type="button"
                className={`px-2 py-0.5 ${lang === "th" ? "bg-sky-100 font-medium" : "bg-white"}`}
                onClick={() => setLang("th")}
              >
                TH
              </button>
              <button
                type="button"
                className={`px-2 py-0.5 ${lang === "en" ? "bg-sky-100 font-medium" : "bg-white"}`}
                onClick={() => setLang("en")}
              >
                EN
              </button>
            </div>
          </div>
          {description && <p className="mt-1 text-slate-600">{description}</p>}
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
              {(lang === "en" && p.purpose_text_en ? p.purpose_text_en : p.purpose_text_th)} ({p.lawful_basis_code})
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
