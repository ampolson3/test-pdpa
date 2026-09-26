"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useProcessingActivity,
  useActivityPurposes,
  useActivityData,
  useRetentionRules,
  useActivityRecipients,
  useActivityTransfers,
  useActivityMutations,
  useLegalEntities,
  useOrgUnits,
  useMasterData,
  useExternalParties,
  useConsentPurposes,
  type ActivityRole,
  type ActivityDataSource,
  type ActivityVolumeBand,
  type ActivityDisposalMethod,
  type ActivityRecipientRole,
  type ActivityTransferBasis,
} from "@pdpa/api-client";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";
const ROLES: ActivityRole[] = ["controller", "processor"];
const SOURCES: ActivityDataSource[] = ["direct", "indirect"];
const VOLUMES: ActivityVolumeBand[] = ["lt_1k", "1k_10k", "10k_100k", "gt_100k"];
const DISPOSALS: ActivityDisposalMethod[] = ["delete", "destroy", "anonymize", "return"];
const RECIPIENT_ROLES: ActivityRecipientRole[] = ["processor", "controller", "joint_controller", "government"];
const TRANSFER_BASES: ActivityTransferBasis[] = ["adequacy", "bcr", "standard_clauses", "certification", "exemption", "consent"];

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

export function ActivityDetailContent({ id }: { id: string }) {
  const t = useTranslations("activities");
  const canRead = usePermission("ropa.activity.read");
  const canUpdate = usePermission("ropa.activity.update");
  const client = useMemo(() => createApiClient("/api/bff"), []);

  const activity = useProcessingActivity(client, id);
  const purposes = useActivityPurposes(client, id);
  const data = useActivityData(client, id);
  const retention = useRetentionRules(client, id);
  const recipients = useActivityRecipients(client, id);
  const transfers = useActivityTransfers(client, id);
  const m = useActivityMutations(client, id);

  const [legalEntityId, setLegalEntityId] = useState<string>();
  const entities = useLegalEntities(client);
  const units = useOrgUnits(client, legalEntityId ?? activity.data?.legal_entity_id);
  const categories = useMasterData(client, "data_categories");
  const subjectTypes = useMasterData(client, "data_subject_types");
  const lawfulBases = useMasterData(client, "lawful_bases");
  const countries = useMasterData(client, "countries");
  const parties = useExternalParties(client);
  const consentPurposes = useConsentPurposes(client);

  const [core, setCore] = useState<{ code: string; name: string; role: ActivityRole; org_unit_id: string; rights_and_access: string } | null>(null);
  const [purposeDraft, setPurposeDraft] = useState({ purpose_text: "", lawful_basis_code: "", consent_purpose_id: "" });
  const [dataDraft, setDataDraft] = useState({ data_category_id: "", subject_type_id: "", source: "" as ActivityDataSource | "", volume_band: "" as ActivityVolumeBand | "" });
  const [retentionDraft, setRetentionDraft] = useState({ data_category_id: "", retention_months: "", retention_basis: "", trigger_event: "", disposal_method: "" as ActivityDisposalMethod | "" });
  const [recipientDraft, setRecipientDraft] = useState({ party_id: "", recipient_role: "" as ActivityRecipientRole | "", disclosure_basis: "" });
  const [transferDraft, setTransferDraft] = useState({ recipient_id: "", country_code: "", transfer_basis: "" as ActivityTransferBasis | "", safeguards: "" });

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;
  if (activity.isPending) return <main className="mx-auto max-w-5xl p-8 text-slate-500">{t("loading")}</main>;
  if (activity.isError || !activity.data) return <main className="mx-auto max-w-5xl p-8 text-red-700">{t("loadError")}</main>;

  const a = activity.data;
  const editable = canUpdate && a.status === "draft";

  return (
    <main className="mx-auto max-w-5xl space-y-6 p-8 text-sm">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{a.name}</h1>
          <p className="font-mono text-xs text-slate-600">{a.code} · {t(`statuses.${a.status}`)}</p>
        </div>
        <span className={a.completeness < 100 ? "rounded bg-amber-100 px-2 py-1 text-amber-800" : "rounded bg-emerald-100 px-2 py-1 text-emerald-800"} data-testid="completeness-badge">
          {a.completeness < 100 ? t("incomplete", { pct: a.completeness }) : t("complete")}
        </span>
      </header>

      {a.missing_items && a.missing_items.length > 0 && (
        <div className="rounded-md bg-amber-50 p-3 text-amber-800" data-testid="missing-items" role="status">
          <p className="font-semibold">{t("missingItemsTitle")}</p>
          <ul className="list-disc pl-5">
            {a.missing_items.map((mi) => <li key={mi}>{t(`missingItems.${mi}`)}</li>)}
          </ul>
        </div>
      )}

      <section className="space-y-2 rounded-md border border-slate-200 bg-white p-4">
        <div className="flex items-center justify-between">
          <h2 className="font-semibold">{t("form.details")}</h2>
          {editable && !core && <button className="text-sky-700 underline" onClick={() => setCore({ code: a.code, name: a.name, role: a.role, org_unit_id: a.org_unit_id, rights_and_access: a.rights_and_access ?? "" })}>{t("edit")}</button>}
        </div>
        {core ? (
          <div className="grid gap-3 sm:grid-cols-2">
            <label><span className="block text-slate-600">{t("form.code")}</span>
              <input className={INPUT} value={core.code} onChange={(e) => setCore({ ...core, code: e.target.value })} maxLength={40} /></label>
            <label><span className="block text-slate-600">{t("form.name")}</span>
              <input className={INPUT} value={core.name} onChange={(e) => setCore({ ...core, name: e.target.value })} /></label>
            <label><span className="block text-slate-600">{t("form.role")}</span>
              <select className={INPUT} value={core.role} onChange={(e) => setCore({ ...core, role: e.target.value as ActivityRole })}>
                {ROLES.map((r) => <option key={r} value={r}>{t(`roles.${r}`)}</option>)}
              </select>
            </label>
            <label><span className="block text-slate-600">{t("form.legalEntity")}</span>
              <select className={INPUT} value={legalEntityId ?? a.legal_entity_id} onChange={(e) => setLegalEntityId(e.target.value)}>
                {entities.data?.map((le) => <option key={le.id} value={le.id}>{le.name_th}</option>)}
              </select>
            </label>
            <label><span className="block text-slate-600">{t("form.orgUnit")}</span>
              <select className={INPUT} value={core.org_unit_id} onChange={(e) => setCore({ ...core, org_unit_id: e.target.value })}>
                {units.data?.map((u) => <option key={u.id} value={u.id}>{u.name_th}</option>)}
              </select>
            </label>
            <label className="sm:col-span-2"><span className="block text-slate-600">{t("form.rightsAndAccess")}</span>
              <textarea className={INPUT} value={core.rights_and_access} onChange={(e) => setCore({ ...core, rights_and_access: e.target.value })} maxLength={4000} rows={3} /></label>
            {m.save.isError && <p className="text-red-700 sm:col-span-2" role="alert">{t("form.saveError", { detail: detail(m.save.error) })}</p>}
            <div className="flex gap-2 sm:col-span-2">
              <Button
                onClick={() => m.save.mutate({ activity: a, input: {
                  legal_entity_id: legalEntityId ?? a.legal_entity_id, org_unit_id: core.org_unit_id, code: core.code, name: core.name,
                  role: core.role, controller_party_id: a.controller_party_id, owner_user_id: a.owner_user_id,
                  rights_and_access: core.rights_and_access || undefined,
                } }, { onSuccess: () => setCore(null) })}
                disabled={m.save.isPending}
              >{t("form.save")}</Button>
              <Button variant="secondary" onClick={() => { m.save.reset(); setCore(null); }}>{t("form.cancel")}</Button>
            </div>
          </div>
        ) : (
          <dl className="grid gap-x-4 gap-y-1 sm:grid-cols-2">
            <div><dt className="text-slate-600">{t("form.role")}</dt><dd>{t(`roles.${a.role}`)}</dd></div>
            <div><dt className="text-slate-600">{t("form.rightsAndAccess")}</dt><dd>{a.rights_and_access || "—"}</dd></div>
          </dl>
        )}
        {editable && a.status !== "pending_approval" && (
          <div className="pt-2">
            <Button onClick={() => m.submit.mutate(a)} disabled={m.submit.isPending}>{t("submit")}</Button>
            {m.submit.isError && <p className="mt-1 text-red-700" role="alert">{t("form.saveError", { detail: detail(m.submit.error) })}</p>}
          </div>
        )}
      </section>

      <section className="space-y-2 rounded-md border border-slate-200 bg-white p-4" data-testid="purposes-section">
        <h2 className="font-semibold">{t("sections.purposes")}</h2>
        <ul className="divide-y divide-slate-100">
          {purposes.data?.map((p) => (
            <li key={p.id} className="flex items-center justify-between py-2">
              <span>{p.purpose_text} — {p.lawful_basis_code}</span>
              {editable && <button className="text-red-700 underline" onClick={() => m.deletePurpose.mutate(p.id)}>{t("remove")}</button>}
            </li>
          ))}
        </ul>
        {editable && (
          <div className="grid gap-2 sm:grid-cols-3">
            <input className={INPUT} placeholder={t("form.purposeText")} value={purposeDraft.purpose_text} onChange={(e) => setPurposeDraft({ ...purposeDraft, purpose_text: e.target.value })} />
            <select className={INPUT} value={purposeDraft.lawful_basis_code} onChange={(e) => setPurposeDraft({ ...purposeDraft, lawful_basis_code: e.target.value })}>
              <option value="">{t("form.chooseLawfulBasis")}</option>
              {lawfulBases.data?.map((b) => <option key={b.code} value={b.code}>{b.code} — {b.name_th}</option>)}
            </select>
            <select className={INPUT} value={purposeDraft.consent_purpose_id} onChange={(e) => setPurposeDraft({ ...purposeDraft, consent_purpose_id: e.target.value })}>
              <option value="">{t("form.noConsentPurpose")}</option>
              {consentPurposes.data?.map((p) => <option key={p.id} value={p.id}>{p.code}</option>)}
            </select>
            <div className="sm:col-span-3">
              <Button
                onClick={() => m.addPurpose.mutate({ purpose_text: purposeDraft.purpose_text, lawful_basis_code: purposeDraft.lawful_basis_code, consent_purpose_id: purposeDraft.consent_purpose_id || undefined },
                  { onSuccess: () => setPurposeDraft({ purpose_text: "", lawful_basis_code: "", consent_purpose_id: "" }) })}
                disabled={m.addPurpose.isPending || !purposeDraft.purpose_text || !purposeDraft.lawful_basis_code}
              >{t("add")}</Button>
            </div>
          </div>
        )}
      </section>

      <section className="space-y-2 rounded-md border border-slate-200 bg-white p-4" data-testid="data-section">
        <h2 className="font-semibold">{t("sections.data")}</h2>
        <ul className="divide-y divide-slate-100">
          {data.data?.map((d) => (
            <li key={d.id} className="flex items-center justify-between py-2">
              <span>
                {categories.data?.find((c) => c.id === d.data_category_id)?.name_th ?? d.data_category_id}
                {d.is_sensitive && <span className="ml-2 rounded bg-amber-100 px-2 py-0.5 text-xs text-amber-800">{t("sensitiveTag")}</span>}
                {" — "}{t(`dataSources.${d.source}`)}
              </span>
              {editable && <button className="text-red-700 underline" onClick={() => m.deleteData.mutate(d.id)}>{t("remove")}</button>}
            </li>
          ))}
        </ul>
        {editable && (
          <div className="grid gap-2 sm:grid-cols-4">
            <select className={INPUT} value={dataDraft.data_category_id} onChange={(e) => setDataDraft({ ...dataDraft, data_category_id: e.target.value })}>
              <option value="">{t("form.dataCategory")}</option>
              {categories.data?.map((c) => <option key={c.id} value={c.id}>{c.name_th}{c.is_sensitive ? " ⚠" : ""}</option>)}
            </select>
            <select className={INPUT} value={dataDraft.subject_type_id} onChange={(e) => setDataDraft({ ...dataDraft, subject_type_id: e.target.value })}>
              <option value="">{t("form.subjectType")}</option>
              {subjectTypes.data?.map((s) => <option key={s.id} value={s.id}>{s.name_th}</option>)}
            </select>
            <select className={INPUT} value={dataDraft.source} onChange={(e) => setDataDraft({ ...dataDraft, source: e.target.value as ActivityDataSource })}>
              <option value="">{t("form.source")}</option>
              {SOURCES.map((s) => <option key={s} value={s}>{t(`dataSources.${s}`)}</option>)}
            </select>
            <select className={INPUT} value={dataDraft.volume_band} onChange={(e) => setDataDraft({ ...dataDraft, volume_band: e.target.value as ActivityVolumeBand })}>
              <option value="">{t("form.volumeBand")}</option>
              {VOLUMES.map((v) => <option key={v} value={v}>{t(`volumeBands.${v}`)}</option>)}
            </select>
            <div className="sm:col-span-4">
              <Button
                onClick={() => m.addData.mutate({ data_category_id: dataDraft.data_category_id, subject_type_id: dataDraft.subject_type_id, source: dataDraft.source as ActivityDataSource, volume_band: dataDraft.volume_band || undefined },
                  { onSuccess: () => setDataDraft({ data_category_id: "", subject_type_id: "", source: "", volume_band: "" }) })}
                disabled={m.addData.isPending || !dataDraft.data_category_id || !dataDraft.subject_type_id || !dataDraft.source}
              >{t("add")}</Button>
            </div>
          </div>
        )}
      </section>

      <section className="space-y-2 rounded-md border border-slate-200 bg-white p-4" data-testid="retention-section">
        <h2 className="font-semibold">{t("sections.retention")}</h2>
        <ul className="divide-y divide-slate-100">
          {retention.data?.map((r) => (
            <li key={r.id} className="flex items-center justify-between py-2">
              <span>{r.trigger_event} → {t(`disposalMethods.${r.disposal_method}`)}{r.retention_months ? ` (${r.retention_months} ${t("months")})` : ""}</span>
              {editable && <button className="text-red-700 underline" onClick={() => m.deleteRetentionRule.mutate(r.id)}>{t("remove")}</button>}
            </li>
          ))}
        </ul>
        {editable && (
          <div className="grid gap-2 sm:grid-cols-4">
            <input className={INPUT} placeholder={t("form.retentionBasis")} value={retentionDraft.retention_basis} onChange={(e) => setRetentionDraft({ ...retentionDraft, retention_basis: e.target.value })} />
            <input className={INPUT} placeholder={t("form.triggerEvent")} value={retentionDraft.trigger_event} onChange={(e) => setRetentionDraft({ ...retentionDraft, trigger_event: e.target.value })} maxLength={60} />
            <input className={INPUT} type="number" min={1} placeholder={t("form.retentionMonths")} value={retentionDraft.retention_months} onChange={(e) => setRetentionDraft({ ...retentionDraft, retention_months: e.target.value })} />
            <select className={INPUT} value={retentionDraft.disposal_method} onChange={(e) => setRetentionDraft({ ...retentionDraft, disposal_method: e.target.value as ActivityDisposalMethod })}>
              <option value="">{t("form.disposalMethod")}</option>
              {DISPOSALS.map((d) => <option key={d} value={d}>{t(`disposalMethods.${d}`)}</option>)}
            </select>
            <div className="sm:col-span-4">
              <Button
                onClick={() => m.addRetentionRule.mutate({
                  retention_basis: retentionDraft.retention_basis, trigger_event: retentionDraft.trigger_event,
                  disposal_method: retentionDraft.disposal_method as ActivityDisposalMethod,
                  retention_months: retentionDraft.retention_months ? Number(retentionDraft.retention_months) : undefined,
                  data_category_id: retentionDraft.data_category_id || undefined,
                }, { onSuccess: () => setRetentionDraft({ data_category_id: "", retention_months: "", retention_basis: "", trigger_event: "", disposal_method: "" }) })}
                disabled={m.addRetentionRule.isPending || !retentionDraft.retention_basis || !retentionDraft.trigger_event || !retentionDraft.disposal_method}
              >{t("add")}</Button>
            </div>
          </div>
        )}
      </section>

      <section className="space-y-2 rounded-md border border-slate-200 bg-white p-4" data-testid="recipients-section">
        <h2 className="font-semibold">{t("sections.recipients")}</h2>
        <ul className="divide-y divide-slate-100">
          {recipients.data?.map((r) => (
            <li key={r.id} className="flex items-center justify-between py-2">
              <span>
                {parties.data?.pages?.flatMap((p) => p.data).find((p) => p.id === r.party_id)?.name_th ?? r.party_id} — {t(`recipientRoles.${r.recipient_role}`)}
                {r.disclosure_basis ? ` (${r.disclosure_basis})` : ` ⚠ ${t("missingBasis")}`}
              </span>
              {editable && <button className="text-red-700 underline" onClick={() => m.deleteRecipient.mutate(r.id)}>{t("remove")}</button>}
            </li>
          ))}
        </ul>
        {editable && (
          <div className="grid gap-2 sm:grid-cols-3">
            <select className={INPUT} value={recipientDraft.party_id} onChange={(e) => setRecipientDraft({ ...recipientDraft, party_id: e.target.value })}>
              <option value="">{t("form.party")}</option>
              {parties.data?.pages?.flatMap((p) => p.data).map((p) => <option key={p.id} value={p.id}>{p.name_th}</option>)}
            </select>
            <select className={INPUT} value={recipientDraft.recipient_role} onChange={(e) => setRecipientDraft({ ...recipientDraft, recipient_role: e.target.value as ActivityRecipientRole })}>
              <option value="">{t("form.recipientRole")}</option>
              {RECIPIENT_ROLES.map((r) => <option key={r} value={r}>{t(`recipientRoles.${r}`)}</option>)}
            </select>
            <input className={INPUT} placeholder={t("form.disclosureBasis")} value={recipientDraft.disclosure_basis} onChange={(e) => setRecipientDraft({ ...recipientDraft, disclosure_basis: e.target.value })} maxLength={40} />
            <div className="sm:col-span-3">
              <Button
                onClick={() => m.addRecipient.mutate({ party_id: recipientDraft.party_id, recipient_role: recipientDraft.recipient_role as ActivityRecipientRole, disclosure_basis: recipientDraft.disclosure_basis || undefined },
                  { onSuccess: () => setRecipientDraft({ party_id: "", recipient_role: "", disclosure_basis: "" }) })}
                disabled={m.addRecipient.isPending || !recipientDraft.party_id || !recipientDraft.recipient_role}
              >{t("add")}</Button>
            </div>
          </div>
        )}
      </section>

      <section className="space-y-2 rounded-md border border-slate-200 bg-white p-4" data-testid="transfers-section">
        <h2 className="font-semibold">{t("sections.transfers")}</h2>
        <ul className="divide-y divide-slate-100">
          {transfers.data?.map((tr) => (
            <li key={tr.id} className="flex items-center justify-between py-2">
              <span>
                {countries.data?.find((c) => c.code === tr.country_code)?.name_th ?? tr.country_code} — {t(`transferBases.${tr.transfer_basis}`)}
              </span>
              {editable && <button className="text-red-700 underline" onClick={() => m.deleteTransfer.mutate(tr.id)}>{t("remove")}</button>}
            </li>
          ))}
        </ul>
        {editable && (
          <div className="grid gap-2 sm:grid-cols-4">
            <select className={INPUT} value={transferDraft.recipient_id} onChange={(e) => setTransferDraft({ ...transferDraft, recipient_id: e.target.value })}>
              <option value="">{t("form.noRecipient")}</option>
              {recipients.data?.map((r) => <option key={r.id} value={r.id}>{parties.data?.pages?.flatMap((p) => p.data).find((p) => p.id === r.party_id)?.name_th ?? r.party_id}</option>)}
            </select>
            <select className={INPUT} value={transferDraft.country_code} onChange={(e) => setTransferDraft({ ...transferDraft, country_code: e.target.value })}>
              <option value="">{t("form.country")}</option>
              {countries.data?.map((c) => <option key={c.code} value={c.code}>{c.name_th}</option>)}
            </select>
            <select className={INPUT} value={transferDraft.transfer_basis} onChange={(e) => setTransferDraft({ ...transferDraft, transfer_basis: e.target.value as ActivityTransferBasis })}>
              <option value="">{t("form.transferBasis")}</option>
              {TRANSFER_BASES.map((b) => <option key={b} value={b}>{t(`transferBases.${b}`)}</option>)}
            </select>
            <input className={INPUT} placeholder={t("form.safeguards")} value={transferDraft.safeguards} onChange={(e) => setTransferDraft({ ...transferDraft, safeguards: e.target.value })} />
            <div className="sm:col-span-4">
              <Button
                onClick={() => m.addTransfer.mutate({ recipient_id: transferDraft.recipient_id || undefined, country_code: transferDraft.country_code, transfer_basis: transferDraft.transfer_basis as ActivityTransferBasis, safeguards: transferDraft.safeguards || undefined },
                  { onSuccess: () => setTransferDraft({ recipient_id: "", country_code: "", transfer_basis: "", safeguards: "" }) })}
                disabled={m.addTransfer.isPending || !transferDraft.country_code || !transferDraft.transfer_basis}
              >{t("add")}</Button>
            </div>
          </div>
        )}
      </section>
    </main>
  );
}
