"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useAgreements,
  useCreateAgreement,
  useVendors,
  useLegalEntities,
  useActivities,
  useDocumentTemplates,
  useAgreementTypeCheck,
  type AgreementType,
  type AgreementCounterpartyRole,
} from "@pdpa/api-client";
import { Link } from "@/i18n/routing";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";
const AGREEMENT_TYPES: AgreementType[] = ["dpa", "dsa", "joint_controller", "inbound_dpa"];

const STATUS_STYLE: Record<string, string> = {
  draft: "bg-slate-100 text-slate-700",
  active: "bg-emerald-100 text-emerald-800",
  expired: "bg-amber-100 text-amber-800",
  terminated: "bg-slate-100 text-slate-500",
};

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

type Draft = {
  agreement_type: AgreementType;
  our_role: "controller" | "processor" | "joint_controller";
  vendor_id: string;
  legal_entity_id: string;
  template_id: string;
  title: string;
  activity_ids: Set<string>;
  auto_renew: boolean;
  renewal_notice_days: string;
};

const blank: Draft = {
  agreement_type: "dpa", our_role: "controller", vendor_id: "", legal_entity_id: "", template_id: "",
  title: "", activity_ids: new Set(), auto_renew: false, renewal_notice_days: "",
};

/** DPA-02's wizard: pick a VEN-01 vendor + its RoPA activities (and optionally a DPA-01 published
 *  template), and the agreement engine drafts a complete DPA document in one call — "โหมดกรอกเอง" is
 *  simply leaving the template unselected. */
export function AgreementsContent() {
  const t = useTranslations("agreements");
  const canRead = usePermission("agreement.dpa.read");
  const canCreate = usePermission("agreement.dpa.create");
  const canCheckType = usePermission("agreement.dsa.read");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const typeCheck = useAgreementTypeCheck(client);
  const [checkRole, setCheckRole] = useState<AgreementCounterpartyRole>("processor");
  const [typeFilter, setTypeFilter] = useState<AgreementType | "">("");
  const list = useAgreements(client, { agreementType: typeFilter || undefined });
  const vendors = useVendors(client, {});
  const entities = useLegalEntities(client);
  const activities = useActivities(client, {});
  const templates = useDocumentTemplates(client, { doc_type: "dpa", published_only: true });
  const create = useCreateAgreement(client);
  const [draft, setDraft] = useState<Draft | null>(null);

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  const rows = list.data?.pages.flatMap((p) => p.data) ?? [];
  const vendorRows = vendors.data?.pages.flatMap((p) => p.data) ?? [];
  const activityRows = activities.data?.pages.flatMap((p) => p.data) ?? [];

  const toggleActivity = (id: string) => {
    if (!draft) return;
    const next = new Set(draft.activity_ids);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    setDraft({ ...draft, activity_ids: next });
  };

  const submit = () => {
    if (!draft || !draft.vendor_id || !draft.legal_entity_id || !draft.title.trim()) return;
    create.mutate(
      {
        agreement_type: draft.agreement_type, our_role: draft.our_role, vendor_id: draft.vendor_id,
        legal_entity_id: draft.legal_entity_id, title: draft.title,
        template_id: draft.template_id || undefined,
        activity_ids: Array.from(draft.activity_ids),
        auto_renew: draft.auto_renew,
        renewal_notice_days: draft.renewal_notice_days ? Number(draft.renewal_notice_days) : 60,
      },
      { onSuccess: () => setDraft(null) },
    );
  };

  return (
    <main className="mx-auto max-w-6xl space-y-6 p-8 text-sm">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{t("title")}</h1>
          <p className="text-slate-600">{t("intro")}</p>
        </div>
        {canCreate && <Button onClick={() => { create.reset(); setDraft({ ...blank, activity_ids: new Set() }); }} data-testid="new-agreement">{t("newAgreement")}</Button>}
      </header>

      {canCheckType && (
        <fieldset className="flex flex-wrap items-end gap-3 rounded-md border border-slate-200 bg-white p-4" data-testid="agreement-type-check">
          <legend className="px-1 font-semibold">{t("typeCheck.title")}</legend>
          <label><span className="block text-slate-600">{t("typeCheck.counterpartyRole")}</span>
            <select className={INPUT} value={checkRole} onChange={(e) => setCheckRole(e.target.value as AgreementCounterpartyRole)}>
              <option value="processor">{t("roles.processor")}</option>
              <option value="controller">{t("roles.controller")}</option>
              <option value="joint_controller">{t("roles.joint_controller")}</option>
            </select>
          </label>
          <Button variant="secondary" onClick={() => typeCheck.mutate(checkRole)} disabled={typeCheck.isPending}>{t("typeCheck.check")}</Button>
          {typeCheck.data && (
            <p className="w-full text-slate-700" data-testid="agreement-type-check-result">
              {t("typeCheck.result", { type: t(`types.${typeCheck.data.agreement_type}`), ref: typeCheck.data.legal_ref })}
              {" — "}{t(`typeCheck.reasons.${typeCheck.data.reason_code}`)}
            </p>
          )}
        </fieldset>
      )}

      {draft && (
        <fieldset className="grid gap-3 rounded-md border border-slate-200 bg-white p-4 sm:grid-cols-2" data-testid="agreement-wizard">
          <legend className="px-1 font-semibold">{t("form.details")}</legend>
          <label><span className="block text-slate-600">{t("form.agreementType")}</span>
            <select className={INPUT} value={draft.agreement_type} onChange={(e) => setDraft({ ...draft, agreement_type: e.target.value as AgreementType })}>
              {AGREEMENT_TYPES.map((v) => <option key={v} value={v}>{t(`types.${v}`)}</option>)}
            </select>
          </label>
          <label><span className="block text-slate-600">{t("form.ourRole")}</span>
            <select className={INPUT} value={draft.our_role} onChange={(e) => setDraft({ ...draft, our_role: e.target.value as Draft["our_role"] })}>
              <option value="controller">{t("roles.controller")}</option>
              <option value="processor">{t("roles.processor")}</option>
              <option value="joint_controller">{t("roles.joint_controller")}</option>
            </select>
          </label>
          <label><span className="block text-slate-600">{t("form.vendor")}</span>
            <select className={INPUT} value={draft.vendor_id} onChange={(e) => setDraft({ ...draft, vendor_id: e.target.value })}>
              <option value="">{t("form.choose")}</option>
              {vendorRows.map((v) => <option key={v.id} value={v.id}>{v.service_description}</option>)}
            </select>
          </label>
          <label><span className="block text-slate-600">{t("form.legalEntity")}</span>
            <select className={INPUT} value={draft.legal_entity_id} onChange={(e) => setDraft({ ...draft, legal_entity_id: e.target.value })}>
              <option value="">{t("form.choose")}</option>
              {entities.data?.map((le) => <option key={le.id} value={le.id}>{le.name_th}</option>)}
            </select>
          </label>
          <label className="sm:col-span-2"><span className="block text-slate-600">{t("form.agreementTitle")}</span>
            <input className={INPUT} value={draft.title} onChange={(e) => setDraft({ ...draft, title: e.target.value })} /></label>
          <label><span className="block text-slate-600">{t("form.template")}</span>
            <select className={INPUT} value={draft.template_id} onChange={(e) => setDraft({ ...draft, template_id: e.target.value })}>
              <option value="">{t("form.manualMode")}</option>
              {templates.data?.map((tpl) => <option key={tpl.id} value={tpl.id}>{tpl.name}</option>)}
            </select>
          </label>
          <label><span className="block text-slate-600">{t("form.renewalNoticeDays")}</span>
            <input className={INPUT} type="number" min={0} value={draft.renewal_notice_days}
              onChange={(e) => setDraft({ ...draft, renewal_notice_days: e.target.value })} placeholder="60" /></label>
          <label className="flex items-center gap-2 sm:col-span-2">
            <input type="checkbox" checked={draft.auto_renew} onChange={(e) => setDraft({ ...draft, auto_renew: e.target.checked })} />
            <span className="text-slate-600">{t("form.autoRenew")}</span>
          </label>
          <div className="sm:col-span-2">
            <span className="block text-slate-600">{t("form.activities")}</span>
            <ul className="mt-1 max-h-48 divide-y divide-slate-100 overflow-y-auto rounded-md border border-slate-200" data-testid="agreement-activity-list">
              {activityRows.map((a) => (
                <li key={a.id} className="flex items-center gap-2 px-3 py-1.5">
                  <input type="checkbox" id={`act-${a.id}`} checked={draft.activity_ids.has(a.id)} onChange={() => toggleActivity(a.id)} />
                  <label htmlFor={`act-${a.id}`} className="flex-1">{a.name}</label>
                </li>
              ))}
              {activityRows.length === 0 && <li className="px-3 py-1.5 text-slate-500">{t("form.noActivities")}</li>}
            </ul>
          </div>
          {create.isError && <p className="text-red-700 sm:col-span-2" role="alert">{t("form.saveError", { detail: detail(create.error) })}</p>}
          <div className="flex gap-2 sm:col-span-2">
            <Button onClick={submit} disabled={create.isPending || !draft.vendor_id || !draft.legal_entity_id || !draft.title.trim()}>{t("form.save")}</Button>
            <Button variant="secondary" onClick={() => { create.reset(); setDraft(null); }}>{t("form.cancel")}</Button>
          </div>
        </fieldset>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <span className="text-slate-600">{t("filterType")}</span>
        <select className={INPUT + " w-auto"} value={typeFilter} onChange={(e) => setTypeFilter(e.target.value as AgreementType | "")}>
          <option value="">{t("allTypes")}</option>
          {AGREEMENT_TYPES.map((v) => <option key={v} value={v}>{t(`types.${v}`)}</option>)}
        </select>
      </div>

      {list.isPending ? (
        <p className="text-slate-500">{t("loading")}</p>
      ) : list.isError ? (
        <p className="text-red-700">{t("loadError")}</p>
      ) : rows.length === 0 ? (
        <p className="text-slate-500">{t("empty")}</p>
      ) : (
        <table className="w-full border-collapse text-left" data-testid="agreements-table">
          <thead>
            <tr className="border-b border-slate-200 text-slate-500">
              <th className="py-2">{t("table.agreementNo")}</th>
              <th className="py-2">{t("table.title")}</th>
              <th className="py-2">{t("table.type")}</th>
              <th className="py-2">{t("table.ourRole")}</th>
              <th className="py-2">{t("table.status")}</th>
              <th className="py-2" />
            </tr>
          </thead>
          <tbody>
            {rows.map((a) => (
              <tr key={a.id} className="border-b border-slate-100">
                <td className="py-2">{a.agreement_no}</td>
                <td className="py-2">{a.title}</td>
                <td className="py-2">{t(`types.${a.agreement_type}`)}</td>
                <td className="py-2">{t(`roles.${a.our_role}`)}</td>
                <td className="py-2"><span className={`rounded px-2 py-0.5 text-xs ${STATUS_STYLE[a.status] ?? "bg-slate-100 text-slate-700"}`}>{t(`statuses.${a.status}`)}</span></td>
                <td className="py-2"><Link className="text-sky-700 underline" href={`/agreements/${a.id}`}>{t("open")}</Link></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {list.hasNextPage && (
        <Button variant="secondary" onClick={() => list.fetchNextPage()} disabled={list.isFetchingNextPage}>{t("more")}</Button>
      )}
    </main>
  );
}
