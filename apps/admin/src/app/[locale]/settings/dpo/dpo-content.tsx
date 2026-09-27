"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useAppointmentMutations,
  useAppointments,
  useLegalEntities,
  type DpoAppointment,
  type DpoType,
} from "@pdpa/api-client";
import { FileUploader } from "@/components/file-uploader";

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
    </main>
  );
}
