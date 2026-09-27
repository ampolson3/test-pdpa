"use client";

import { useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { formatDate, type Locale } from "@pdpa/i18n";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { FormRenderer, type FormSchema, type Language, type Scoring } from "@pdpa/form-renderer";
import {
  createApiClient,
  useAppointmentMutations,
  useAppointments,
  useAssessment,
  useAssessments,
  useForm,
  useForms,
  useLegalEntities,
  useRecordAssessment,
  type DpoAppointment,
  type DpoSecurityAssessment,
  type DpoType,
} from "@pdpa/api-client";
import { FileUploader } from "@/components/file-uploader";
import { useRendererMessages } from "@/components/form-messages";

function problemText(e: unknown): string {
  if (typeof e !== "object" || e === null) return "";
  const p = e as { title?: string; detail?: string; errors?: { field: string; code: string }[] };
  const fields = (p.errors ?? []).map((f) => `${f.field}: ${f.code}`).join(", ");
  return [p.title, p.detail, fields].filter(Boolean).join(" — ");
}

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";
const TYPES: DpoType[] = ["internal", "external", "group"];

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

type Draft = {
  dpo_type: DpoType | "";
  user_id: string;
  external_name: string;
  external_company: string;
  contact_email: string;
  contact_phone: string;
  appointed_at: string;
  appointment_file_id?: string;
  ended_at: string;
};
const blank: Draft = { dpo_type: "", user_id: "", external_name: "", external_company: "", contact_email: "", contact_phone: "", appointed_at: "", ended_at: "" };

export function DpoContent() {
  const t = useTranslations("dpo");
  const canRead = usePermission("dpo.profile.read");
  const canCreate = usePermission("dpo.profile.create");
  const canUpdate = usePermission("dpo.profile.update");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const entities = useLegalEntities(client);
  const [legalEntityId, setLegalEntityId] = useState<string>();
  const [draft, setDraft] = useState<Draft | null>(null);
  const [editing, setEditing] = useState<DpoAppointment | null>(null);

  const list = useAppointments(client, { legal_entity_id: legalEntityId });
  const { save } = useAppointmentMutations(client);

  if (!canRead) return <main className="mx-auto max-w-4xl p-8 text-slate-600">{t("forbidden")}</main>;

  const rows = list.data?.pages.flatMap((p) => p.data) ?? [];
  const set = (p: Partial<Draft>) => setDraft({ ...(draft ?? blank), ...p });

  const startCreate = () => {
    save.reset();
    setEditing(null);
    setDraft({ ...blank });
  };
  const startEdit = (a: DpoAppointment) => {
    save.reset();
    setEditing(a);
    setDraft({
      dpo_type: a.dpo_type, user_id: a.user_id ?? "", external_name: a.external_name ?? "", external_company: a.external_company ?? "",
      contact_email: a.contact_email, contact_phone: a.contact_phone ?? "", appointed_at: a.appointed_at, appointment_file_id: a.appointment_file_id,
      ended_at: a.ended_at ?? "",
    });
  };

  const submit = () => {
    if (!draft || !draft.dpo_type || !legalEntityId) return;
    save.mutate(
      {
        appointment: editing ?? undefined,
        input: {
          legal_entity_id: legalEntityId, dpo_type: draft.dpo_type, contact_email: draft.contact_email, appointed_at: draft.appointed_at,
          user_id: draft.dpo_type === "internal" ? draft.user_id || undefined : undefined,
          external_name: draft.dpo_type !== "internal" ? draft.external_name || undefined : undefined,
          external_company: draft.dpo_type !== "internal" ? draft.external_company || undefined : undefined,
          contact_phone: draft.contact_phone || undefined, appointment_file_id: draft.appointment_file_id,
          ended_at: draft.ended_at || undefined,
        },
      },
      { onSuccess: () => { setDraft(null); setEditing(null); } },
    );
  };

  return (
    <main className="mx-auto max-w-4xl space-y-6 p-8 text-sm">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{t("title")}</h1>
          <p className="text-slate-600">{t("intro")}</p>
        </div>
        {canCreate && legalEntityId && <Button onClick={startCreate}>{t("newAppointment")}</Button>}
      </header>

      <label><span className="block text-slate-600">{t("form.legalEntity")}</span>
        <select className={INPUT} value={legalEntityId ?? ""} onChange={(e) => { setLegalEntityId(e.target.value || undefined); setDraft(null); }}>
          <option value="">{t("form.choose")}</option>
          {entities.data?.map((le) => <option key={le.id} value={le.id}>{le.name_th}</option>)}
        </select>
      </label>

      {!legalEntityId ? (
        <p className="rounded-md bg-amber-50 p-3 text-amber-800">{t("chooseEntity")}</p>
      ) : draft ? (
        <fieldset className="grid gap-3 rounded-md border border-slate-200 bg-white p-4 sm:grid-cols-2" disabled={!canCreate && !canUpdate}>
          <legend className="px-1 font-semibold">{editing ? t("form.editTitle") : t("form.newTitle")}</legend>
          <label><span className="block text-slate-600">{t("form.dpoType")}</span>
            <select className={INPUT} value={draft.dpo_type} onChange={(e) => set({ dpo_type: e.target.value as DpoType })}>
              <option value="">{t("form.choose")}</option>
              {TYPES.map((ty) => <option key={ty} value={ty}>{t(`types.${ty}`)}</option>)}
            </select>
          </label>
          {draft.dpo_type === "internal" ? (
            <label><span className="block text-slate-600">{t("form.userId")}</span>
              <input className={INPUT} value={draft.user_id} onChange={(e) => set({ user_id: e.target.value })} placeholder={t("form.userIdHint")} /></label>
          ) : (
            <label><span className="block text-slate-600">{t("form.externalName")}</span>
              <input className={INPUT} value={draft.external_name} onChange={(e) => set({ external_name: e.target.value })} /></label>
          )}
          {draft.dpo_type !== "internal" && (
            <label><span className="block text-slate-600">{t("form.externalCompany")}</span>
              <input className={INPUT} value={draft.external_company} onChange={(e) => set({ external_company: e.target.value })} /></label>
          )}
          <label><span className="block text-slate-600">{t("form.contactEmail")}</span>
            <input type="email" className={INPUT} value={draft.contact_email} onChange={(e) => set({ contact_email: e.target.value })} /></label>
          <label><span className="block text-slate-600">{t("form.contactPhone")}</span>
            <input className={INPUT} value={draft.contact_phone} onChange={(e) => set({ contact_phone: e.target.value })} /></label>
          <label><span className="block text-slate-600">{t("form.appointedAt")}</span>
            <input type="date" className={INPUT} value={draft.appointed_at} onChange={(e) => set({ appointed_at: e.target.value })} /></label>
          <label><span className="block text-slate-600">{t("form.endedAt")}</span>
            <input type="date" className={INPUT} value={draft.ended_at} onChange={(e) => set({ ended_at: e.target.value })} /></label>
          <div className="sm:col-span-2">
            <span className="block text-slate-600">{t("form.appointmentFile")}</span>
            <FileUploader onUploaded={(f) => set({ appointment_file_id: f.id })} accept=".pdf,.png,.jpg,.jpeg,.docx" />
          </div>
          {save.isError && <p className="text-red-700 sm:col-span-2" role="alert">{t("form.saveError", { detail: detail(save.error) })}</p>}
          <div className="flex gap-2 sm:col-span-2">
            <Button onClick={submit} disabled={save.isPending || !draft.dpo_type || !draft.contact_email || !draft.appointed_at}>{t("form.save")}</Button>
            <Button variant="secondary" onClick={() => { save.reset(); setDraft(null); setEditing(null); }}>{t("form.cancel")}</Button>
          </div>
        </fieldset>
      ) : null}

      {legalEntityId && (list.isPending ? <p className="text-slate-500">{t("loading")}</p> : list.isError ? <p className="text-red-700">{t("loadError")}</p> : rows.length === 0 ? (
        <p className="rounded-md bg-amber-50 p-3 text-amber-800">{t("empty")}</p>
      ) : (
        <table className="w-full rounded-md border border-slate-200 bg-white" data-testid="dpo-list">
          <thead className="bg-slate-50 text-left text-slate-600">
            <tr><th className="px-3 py-2">{t("form.dpoType")}</th><th className="px-3 py-2">{t("form.contactEmail")}</th>
              <th className="px-3 py-2">{t("form.appointedAt")}</th><th className="px-3 py-2">{t("statusLabel")}</th><th className="px-3 py-2" /></tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.map((a) => (
              <tr key={a.id}>
                <td className="px-3 py-2">{t(`types.${a.dpo_type}`)}</td>
                <td className="px-3 py-2">{a.contact_email}</td>
                <td className="px-3 py-2">{a.appointed_at}</td>
                <td className="px-3 py-2">
                  <span className={a.ended_at ? "rounded bg-slate-100 px-2 py-0.5 text-slate-600" : "rounded bg-emerald-100 px-2 py-0.5 text-emerald-800"}>
                    {a.ended_at ? t("ended") : t("current")}
                  </span>
                </td>
                <td className="px-3 py-2 text-right">
                  {canUpdate && <button className="text-sky-700 underline" onClick={() => startEdit(a)}>{t("form.edit")}</button>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ))}
      {legalEntityId && list.hasNextPage && <Button variant="secondary" onClick={() => list.fetchNextPage()}>{t("more")}</Button>}

      {legalEntityId && <SecurityAssessments legalEntityId={legalEntityId} />}
    </main>
  );
}

/** DPO-09: run a published security-measures checklist and see past runs, each with its remediation tasks. */
function SecurityAssessments({ legalEntityId }: { legalEntityId: string }) {
  const t = useTranslations("dpo");
  const locale = useLocale() as Language;
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const canRead = usePermission("dpo.risk.read");
  const canRecord = usePermission("dpo.risk.create");
  const list = useAssessments(client, { legal_entity_id: legalEntityId });
  const forms = useForms(client, "security");
  const published = (forms.data ?? []).filter((f) => f.current_version_id);
  const [formId, setFormId] = useState("");
  const chosen = formId || published[0]?.id;
  const form = useForm(client, chosen);
  const record = useRecordAssessment(client);
  const messages = useRendererMessages(locale);
  const [expanded, setExpanded] = useState<string | null>(null);
  const version = form.data?.versions?.find((v) => v.id === form.data?.current_version_id);

  if (!canRead) return null;
  const rows = list.data?.pages.flatMap((p) => p.data) ?? [];

  return (
    <section className="space-y-4 border-t border-slate-200 pt-6">
      <h2 className="text-lg font-semibold">{t("assessments.title")}</h2>
      <p className="text-slate-600">{t("assessments.intro")}</p>

      {canRecord && (
        <div className="space-y-3 rounded-md border border-slate-200 bg-white p-4" data-testid="assessment-form">
          {published.length === 0 ? <p className="text-amber-800">{t("assessments.noForm")}</p> : (
            <select className={INPUT} value={chosen ?? ""} onChange={(e) => setFormId(e.target.value)}>
              {published.map((f) => <option key={f.id} value={f.id}>{f.name}</option>)}
            </select>
          )}
          {version && (
            <FormRenderer schema={version.schema as FormSchema} scoring={version.scoring as Scoring | null} language={locale} messages={messages} showScore
              idPrefix="dpo-security"
              onSubmit={async (answers) => { await record.mutateAsync({ legal_entity_id: legalEntityId, form_id: chosen!, answers }); }}
              actions={({ submit, busy }) => <Button type="button" onClick={submit} disabled={busy || record.isPending} data-testid="assess-submit">{t("assessments.submit")}</Button>} />
          )}
          {record.isError && <p className="text-red-700" role="alert">{problemText(record.error)}</p>}
        </div>
      )}

      {list.isPending ? <p className="text-slate-500">{t("loading")}</p> : rows.length === 0 ? (
        <p className="rounded-md bg-amber-50 p-3 text-amber-800">{t("assessments.empty")}</p>
      ) : (
        <ul className="space-y-2">
          {rows.map((a) => (
            <li key={a.id} className="rounded-md border border-slate-200 bg-white p-3">
              <button className="flex w-full items-center justify-between text-left" onClick={() => setExpanded(expanded === a.id ? null : a.id)}>
                <span className="flex items-center gap-2">
                  <span className={a.result === "pass" ? "rounded bg-emerald-100 px-2 py-0.5 text-emerald-800" : "rounded bg-amber-100 px-2 py-0.5 text-amber-800"}>
                    {a.result}
                  </span>
                  <span>{t("assessments.score", { score: a.score })}</span>
                  <span className="text-xs text-slate-500">{formatDate(a.assessed_at, locale as Locale, { month: "short", hour: "2-digit", minute: "2-digit" })}</span>
                </span>
              </button>
              {expanded === a.id && <AssessmentDetail id={a.id} />}
            </li>
          ))}
        </ul>
      )}
      {list.hasNextPage && <Button variant="secondary" onClick={() => list.fetchNextPage()}>{t("more")}</Button>}
    </section>
  );
}

function AssessmentDetail({ id }: { id: string }) {
  const t = useTranslations("dpo");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const q = useAssessment(client, id);
  const a = q.data as DpoSecurityAssessment | undefined;
  if (!a) return null;
  return (
    <div className="mt-3 space-y-2 border-t border-slate-100 pt-3">
      <table className="w-full text-left text-xs">
        <thead className="text-slate-500"><tr><th className="py-1">{t("assessments.item")}</th><th>{t("assessments.answer")}</th></tr></thead>
        <tbody className="divide-y divide-slate-100">
          {a.factors.map((f) => (
            <tr key={f.question}><td className="py-1">{f.label.th ?? f.question}</td><td>{String(f.answer ?? "")}</td></tr>
          ))}
        </tbody>
      </table>
      {a.tasks && a.tasks.length > 0 && (
        <div>
          <p className="font-semibold text-slate-700">{t("assessments.tasks")}</p>
          <ul className="list-inside list-disc text-amber-800">
            {a.tasks.map((task) => <li key={task.id}>{task.task_no} — {task.title}</li>)}
          </ul>
        </div>
      )}
    </div>
  );
}
