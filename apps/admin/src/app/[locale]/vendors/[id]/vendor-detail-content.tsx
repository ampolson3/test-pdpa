"use client";

import { useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { FormRenderer, type FormSchema, type Language, type Scoring } from "@pdpa/form-renderer";
import {
  createApiClient,
  useVendor,
  useSaveVendor,
  useExternalParties,
  useAgreements,
  useVendorIntakes,
  useRecordVendorIntake,
  useVendorContractStatus,
  useVendorAssessments,
  useRecordVendorAssessment,
  useDpiaTemplates,
  useForm,
} from "@pdpa/api-client";
import { useRendererMessages } from "@/components/form-messages";
import { Link } from "@/i18n/routing";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

const TIER_STYLE: Record<string, string> = {
  low: "bg-emerald-100 text-emerald-800",
  medium: "bg-amber-100 text-amber-800",
  high: "bg-orange-100 text-orange-800",
  critical: "bg-red-100 text-red-800",
};

const STATUS_STYLE: Record<string, string> = {
  prospect: "bg-slate-100 text-slate-700",
  onboarding: "bg-sky-100 text-sky-800",
  approved: "bg-emerald-100 text-emerald-800",
  conditional: "bg-amber-100 text-amber-800",
  rejected: "bg-red-100 text-red-800",
  offboarding: "bg-amber-100 text-amber-800",
  terminated: "bg-slate-100 text-slate-500",
};

/** VEN-01's acceptance criterion: one page per vendor gathering its profile — sibling features (VEN-02
 *  tiering, VEN-04/05/07 assessments, VEN-08 approval, VEN-11 contracts/activities, ...) add their own
 *  sections to this same page once built, rather than scattering vendor data across separate screens. */
export function VendorDetailContent({ id }: { id: string }) {
  const t = useTranslations("vendors");
  const ta = useTranslations("agreements");
  const canRead = usePermission("vendor.vendor.read");
  const canUpdate = usePermission("vendor.vendor.update");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const vendor = useVendor(client, id);
  const parties = useExternalParties(client);
  const agreements = useAgreements(client, { vendorId: id });
  const contractStatus = useVendorContractStatus(client, id);
  const save = useSaveVendor(client);
  const [editing, setEditing] = useState(false);
  const [serviceDescription, setServiceDescription] = useState("");
  const [isProcessor, setIsProcessor] = useState(true);
  const [countries, setCountries] = useState("");

  if (!canRead) return <main className="mx-auto max-w-4xl p-8 text-slate-600">{t("forbidden")}</main>;
  if (vendor.isPending) return <main className="mx-auto max-w-4xl p-8 text-slate-500">{t("loading")}</main>;
  if (vendor.isError || !vendor.data) return <main className="mx-auto max-w-4xl p-8 text-red-700">{t("loadError")}</main>;

  const v = vendor.data;
  const partyRows = parties.data?.pages.flatMap((p) => p.data) ?? [];
  const party = partyRows.find((p) => p.id === v.party_id);

  const startEdit = () => {
    save.reset();
    setServiceDescription(v.service_description);
    setIsProcessor(v.is_processor);
    setCountries(v.processing_countries.join(", "));
    setEditing(true);
  };

  const submit = () => {
    save.mutate(
      {
        id: v.id, rowVersion: v.row_version,
        input: {
          party_id: v.party_id, service_description: serviceDescription, is_processor: isProcessor,
          relationship_owner_id: v.relationship_owner_id ?? undefined,
          processing_countries: countries.split(",").map((c) => c.trim()).filter(Boolean),
        },
      },
      { onSuccess: () => setEditing(false) },
    );
  };

  return (
    <main className="mx-auto max-w-4xl space-y-6 p-8 text-sm">
      <Link className="text-sky-700 underline" href="/vendors">{t("backToList")}</Link>

      <header className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{party?.name_th ?? v.party_id}</h1>
          <p className="text-slate-600">{v.service_description}</p>
        </div>
        <span className={`rounded px-2 py-1 ${STATUS_STYLE[v.status]}`} data-testid="vendor-status">{t(`statuses.${v.status}`)}</span>
      </header>

      <section className="grid gap-3 rounded-md border border-slate-200 bg-white p-4 sm:grid-cols-2">
        <h2 className="font-semibold sm:col-span-2">{t("form.details")}</h2>
        <div><span className="block text-slate-500">{t("form.party")}</span><span>{party?.name_th ?? "—"}</span></div>
        <div><span className="block text-slate-500">{t("form.tier")}</span><span>{v.tier ? t(`tiers.${v.tier}`) : t("form.tierNotSet")}</span></div>
        <div><span className="block text-slate-500">{t("form.isProcessor")}</span><span>{v.is_processor ? t("statuses.approved") : "—"}</span></div>
        <div><span className="block text-slate-500">{t("form.processingCountries")}</span><span>{v.processing_countries.join(", ") || "—"}</span></div>
        {v.next_assessment_at && <div><span className="block text-slate-500">{t("form.nextAssessmentAt")}</span><span>{v.next_assessment_at}</span></div>}

        {editing ? (
          <>
            <label className="sm:col-span-2"><span className="block text-slate-600">{t("form.serviceDescription")}</span>
              <textarea className={INPUT} rows={2} value={serviceDescription} maxLength={2000}
                onChange={(e) => setServiceDescription(e.target.value)} /></label>
            <label><span className="block text-slate-600">{t("form.processingCountries")}</span>
              <input className={INPUT} value={countries} onChange={(e) => setCountries(e.target.value)} /></label>
            <label className="flex items-center gap-2">
              <input type="checkbox" checked={isProcessor} onChange={(e) => setIsProcessor(e.target.checked)} />
              <span className="text-slate-600">{t("form.isProcessor")}</span>
            </label>
            {save.isError && <p className="text-red-700 sm:col-span-2" role="alert">{t("form.saveError", { detail: detail(save.error) })}</p>}
            <div className="flex gap-2 sm:col-span-2">
              <Button onClick={submit} disabled={save.isPending}>{t("form.save")}</Button>
              <Button variant="secondary" onClick={() => setEditing(false)}>{t("form.cancel")}</Button>
            </div>
          </>
        ) : (
          canUpdate && <div className="sm:col-span-2"><Button variant="secondary" onClick={startEdit}>{t("form.edit")}</Button></div>
        )}
      </section>

      {contractStatus.data?.is_processor && !contractStatus.data.has_dpa && (
        <div className="rounded-md border border-amber-300 bg-amber-50 p-3 text-amber-900" role="alert">
          {t("detail.agreements.missingDpaWarning")}
        </div>
      )}

      <section className="space-y-2 rounded-md border border-slate-200 bg-white p-4">
        <h2 className="font-semibold">{t("detail.agreements.title")}</h2>
        {(agreements.data?.pages.flatMap((p) => p.data) ?? []).length === 0 ? (
          <p className="text-slate-500">{t("detail.agreements.empty")}</p>
        ) : (
          <ul className="divide-y divide-slate-100">
            {agreements.data!.pages.flatMap((p) => p.data).map((ag) => (
              <li key={ag.id} className="space-y-1 py-2">
                <div className="flex items-center justify-between gap-2">
                  <Link className="font-medium text-sky-700 underline" href={`/agreements/${ag.id}`}>
                    {ag.title}
                  </Link>
                  <span className="text-xs text-slate-500">{ta(`statuses.${ag.status}`)}</span>
                </div>
                <p className="text-xs text-slate-500">{ag.agreement_no}</p>
                {ag.activity_ids.length > 0 && (
                  <p className="text-xs text-slate-500">
                    {t("detail.agreements.activities")}:{" "}
                    {ag.activity_ids.map((aid, i) => (
                      <span key={aid}>
                        {i > 0 && ", "}
                        <Link className="underline" href={`/ropa/activities/${aid}`}>
                          {aid}
                        </Link>
                      </span>
                    ))}
                  </p>
                )}
              </li>
            ))}
          </ul>
        )}
        <Link className="text-sky-700 underline" href="/agreements">{t("agreementsLink")}</Link>
      </section>

      <IntakePanel client={client} vendorId={v.id} canUpdate={canUpdate} />
      <AssessmentPanel client={client} vendorId={v.id} />
    </main>
  );
}

const RESIDUAL_STYLE: Record<string, string> = {
  low: "bg-emerald-100 text-emerald-800",
  medium: "bg-amber-100 text-amber-800",
  high: "bg-orange-100 text-orange-800",
  critical: "bg-red-100 text-red-800",
};

/** VEN-07: answer one of VEN-04's published vendor templates once and see the score + residual risk
 *  level (RRA-02) it classifies to, plus every past cycle. */
function AssessmentPanel({ client, vendorId }: { client: ReturnType<typeof createApiClient>; vendorId: string }) {
  const t = useTranslations("vendors");
  const locale = useLocale() as Language;
  const canRead = usePermission("vendor.vendor.read");
  const canRecord = usePermission("vendor.vendor.update");
  const canReadTemplates = usePermission("assessment.template.read");
  const list = useVendorAssessments(client, vendorId);
  const templates = useDpiaTemplates(client, "vendor");
  const published = (templates.data?.data ?? []).filter((tpl) => tpl.status === "published");
  const [templateId, setTemplateId] = useState("");
  const chosen = templateId || published[0]?.id;
  const chosenTemplate = published.find((tpl) => tpl.id === chosen);
  const form = useForm(client, chosenTemplate?.form_id);
  const record = useRecordVendorAssessment(client, vendorId);
  const messages = useRendererMessages(locale);
  const version = form.data?.versions?.find((ver) => ver.id === form.data?.current_version_id);

  if (!canRead) return null;
  const rows = list.data?.data ?? [];

  return (
    <section className="space-y-3 rounded-md border border-slate-200 bg-white p-4">
      <h2 className="font-semibold">{t("detail.assessments.title")}</h2>

      {canRecord && canReadTemplates && (
        <div className="space-y-3 border-b border-slate-100 pb-3" data-testid="assessment-form">
          {published.length === 0 ? <p className="text-amber-800">{t("detail.assessments.noTemplate")}</p> : (
            <select className={SELECT} value={chosen ?? ""} onChange={(e) => setTemplateId(e.target.value)}>
              {published.map((tpl) => <option key={tpl.id} value={tpl.id}>{tpl.name}</option>)}
            </select>
          )}
          {version && chosenTemplate && (
            <FormRenderer schema={version.schema as FormSchema} scoring={version.scoring as Scoring | null} language={locale} messages={messages} showScore
              idPrefix="vendor-assessment"
              onSubmit={async (answers) => { await record.mutateAsync({ template_code: chosenTemplate.code, answers }); }}
              actions={({ submit, busy }) => <Button type="button" onClick={submit} disabled={busy || record.isPending} data-testid="assess-submit">{t("detail.assessments.submit")}</Button>} />
          )}
          {record.isError && <p className="text-red-700" role="alert">{t("detail.assessments.submitError", { detail: detail(record.error) })}</p>}
        </div>
      )}

      {list.isPending ? <p className="text-slate-500">{t("loading")}</p> : rows.length === 0 ? (
        <p className="text-slate-500">{t("detail.assessments.empty")}</p>
      ) : (
        <ul className="divide-y divide-slate-100 text-sm">
          {rows.map((a) => (
            <li key={a.id} className="flex items-center justify-between gap-2 py-2">
              <span className="text-slate-500">{t("detail.assessments.cycle", { n: a.cycle_no })}</span>
              <span className={`rounded px-2 py-0.5 text-xs font-semibold ${RESIDUAL_STYLE[a.residual_level] ?? "bg-slate-100 text-slate-700"}`}>
                {t(`tiers.${a.residual_level}`)}
              </span>
              <span className="text-slate-500">{t("detail.assessments.score", { score: a.score })}</span>
              <span className="text-xs text-slate-500">{new Date(a.created_at).toLocaleDateString()}</span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

const SELECT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";

/** VEN-02: answer the intake questionnaire once; the system computes the tier (and which VEN-04
 *  assessment templates it now requires) rather than leaving that to a human's own reading. */
function IntakePanel({ client, vendorId, canUpdate }: { client: ReturnType<typeof createApiClient>; vendorId: string; canUpdate: boolean }) {
  const t = useTranslations("vendors");
  const intakes = useVendorIntakes(client, vendorId);
  const record = useRecordVendorIntake(client, vendorId);
  const [open, setOpen] = useState(false);
  const [dataVolume, setDataVolume] = useState("small");
  const [sensitiveData, setSensitiveData] = useState(false);
  const [systemAccessLevel, setSystemAccessLevel] = useState("none");
  const [crossBorderTransfer, setCrossBorderTransfer] = useState(false);

  const rows = intakes.data?.data ?? [];

  const submit = () => {
    record.mutate(
      {
        answers: {
          data_volume: dataVolume,
          sensitive_data: sensitiveData ? "yes" : "no",
          system_access_level: systemAccessLevel,
          cross_border_transfer: crossBorderTransfer ? "yes" : "no",
        },
      },
      { onSuccess: () => setOpen(false) },
    );
  };

  return (
    <section className="space-y-3 rounded-md border border-slate-200 bg-white p-4">
      <h2 className="font-semibold">{t("detail.intake.title")}</h2>

      {rows.length === 0 ? <p className="text-slate-500">{t("detail.intake.empty")}</p> : (
        <div className="space-y-1">
          <h3 className="text-xs font-medium text-slate-500">{t("detail.intake.history")}</h3>
          <ul className="divide-y divide-slate-100 text-sm">
            {rows.map((in_) => (
              <li key={in_.id} className="flex items-center justify-between gap-2 py-1">
                <span className={`rounded px-2 py-0.5 text-xs ${TIER_STYLE[in_.tier_result] ?? "bg-slate-100 text-slate-700"}`}>
                  {t(`tiers.${in_.tier_result}`)}
                </span>
                <span className="text-slate-500">{t("detail.intake.historyScore", { score: in_.inherent_score })}</span>
                <span className="text-slate-500">{t("detail.intake.historyDate", { date: new Date(in_.created_at).toLocaleDateString() })}</span>
              </li>
            ))}
          </ul>
        </div>
      )}

      {record.isSuccess && (
        <div className="rounded-md bg-emerald-50 p-2 text-sm text-emerald-800">
          <p>{t("detail.intake.result", { tier: t(`tiers.${record.data.intake.tier_result}`), score: record.data.intake.inherent_score })}</p>
          <p>
            {record.data.required_assessment_codes.length > 0
              ? t("detail.intake.requiredAssessments", { codes: record.data.required_assessment_codes.join(", ") })
              : t("detail.intake.requiredAssessmentsNone")}
          </p>
        </div>
      )}

      {canUpdate && (open ? (
        <div className="space-y-3 border-t border-slate-100 pt-3">
          <label className="block"><span className="block text-slate-600">{t("detail.intake.dataVolume")}</span>
            <select className={SELECT} value={dataVolume} onChange={(e) => setDataVolume(e.target.value)}>
              {(["small", "medium", "large", "very_large"] as const).map((v) => (
                <option key={v} value={v}>{t(`detail.intake.dataVolumeOptions.${v}`)}</option>
              ))}
            </select>
          </label>
          <label className="flex items-center gap-2">
            <input type="checkbox" checked={sensitiveData} onChange={(e) => setSensitiveData(e.target.checked)} />
            <span className="text-slate-600">{t("detail.intake.sensitiveData")}</span>
          </label>
          <label className="block"><span className="block text-slate-600">{t("detail.intake.systemAccessLevel")}</span>
            <select className={SELECT} value={systemAccessLevel} onChange={(e) => setSystemAccessLevel(e.target.value)}>
              {(["none", "read_only", "read_write", "admin"] as const).map((v) => (
                <option key={v} value={v}>{t(`detail.intake.systemAccessLevelOptions.${v}`)}</option>
              ))}
            </select>
          </label>
          <label className="flex items-center gap-2">
            <input type="checkbox" checked={crossBorderTransfer} onChange={(e) => setCrossBorderTransfer(e.target.checked)} />
            <span className="text-slate-600">{t("detail.intake.crossBorderTransfer")}</span>
          </label>
          {record.isError && <p className="text-red-700" role="alert">{t("detail.intake.submitError", { detail: detail(record.error) })}</p>}
          <div className="flex gap-2">
            <Button onClick={submit} disabled={record.isPending}>{t("detail.intake.submit")}</Button>
            <Button variant="secondary" onClick={() => setOpen(false)}>{t("form.cancel")}</Button>
          </div>
        </div>
      ) : (
        <Button variant="secondary" onClick={() => setOpen(true)}>{t("detail.intake.newRound")}</Button>
      ))}
    </section>
  );
}
