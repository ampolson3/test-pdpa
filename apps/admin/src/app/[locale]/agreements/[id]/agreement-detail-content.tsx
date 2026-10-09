"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useAgreement,
  useAgreementClauses,
  useMissingClauses,
  useAddAgreementClause,
  useRemoveAgreementClause,
  useClauses,
  useProcessingSchedule,
  useSetAgreementSchedule,
  useAgreementParties,
  useAddAgreementParty,
  useRemoveAgreementParty,
  useExternalParties,
  useLegalEntities,
  type ApiClient,
  type Agreement,
  type AgreementPartyInput,
} from "@pdpa/api-client";
import { Link } from "@/i18n/routing";

const STATUS_STYLE: Record<string, string> = {
  draft: "bg-slate-100 text-slate-700",
  active: "bg-emerald-100 text-emerald-800",
  expired: "bg-amber-100 text-amber-800",
  terminated: "bg-slate-100 text-slate-500",
};

/** DPA-02's own agreement — editing/exporting/publishing the composed content happens on the linked
 *  PLT-16 document page; this page is the agreement's own metadata (parties, activities, renewal). */
export function AgreementDetailContent({ id }: { id: string }) {
  const t = useTranslations("agreements");
  const canRead = usePermission("agreement.dpa.read");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const agreement = useAgreement(client, id);

  if (!canRead) return <main className="mx-auto max-w-4xl p-8 text-slate-600">{t("forbidden")}</main>;
  if (agreement.isPending) return <main className="mx-auto max-w-4xl p-8 text-slate-500">{t("loading")}</main>;
  if (agreement.isError || !agreement.data) return <main className="mx-auto max-w-4xl p-8 text-red-700">{t("loadError")}</main>;

  const a = agreement.data;

  return (
    <main className="mx-auto max-w-4xl space-y-6 p-8 text-sm">
      <Link className="text-sky-700 underline" href="/agreements">{t("backToList")}</Link>

      <header className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{a.title}</h1>
          <p className="text-slate-600">{a.agreement_no}</p>
        </div>
        <span className={`rounded px-2 py-1 text-xs ${STATUS_STYLE[a.status] ?? "bg-slate-100 text-slate-700"}`}>{t(`statuses.${a.status}`)}</span>
      </header>

      <dl className="grid gap-3 rounded-md border border-slate-200 bg-white p-4 sm:grid-cols-2">
        <div><dt className="text-slate-500">{t("table.type")}</dt><dd>{t(`types.${a.agreement_type}`)}</dd></div>
        <div><dt className="text-slate-500">{t("table.ourRole")}</dt><dd>{t(`roles.${a.our_role}`)}</dd></div>
        <div><dt className="text-slate-500">{t("detail.autoRenew")}</dt><dd>{a.auto_renew ? t("detail.yes") : t("detail.no")}</dd></div>
        <div><dt className="text-slate-500">{t("detail.renewalNoticeDays")}</dt><dd>{a.renewal_notice_days}</dd></div>
        {a.effective_from && <div><dt className="text-slate-500">{t("detail.effectiveFrom")}</dt><dd>{a.effective_from}</dd></div>}
        {a.effective_to && <div><dt className="text-slate-500">{t("detail.effectiveTo")}</dt><dd>{a.effective_to}</dd></div>}
      </dl>

      <RenewalScheduleSection client={client} agreement={a} />

      <section className="space-y-2">
        <h2 className="font-semibold">{t("detail.activities")}</h2>
        {a.activity_ids.length === 0 ? (
          <p className="text-slate-500">{t("detail.noActivities")}</p>
        ) : (
          <ul className="list-inside list-disc rounded-md border border-slate-200 bg-white p-3">
            {a.activity_ids.map((id) => (
              <li key={id}><Link className="text-sky-700 underline" href={`/ropa/activities/${id}`}>{id}</Link></li>
            ))}
          </ul>
        )}
      </section>

      <PartiesPanel client={client} agreementId={id} />

      <ClausePanel client={client} agreementId={id} isDraft={a.status === "draft"} />

      <ProcessingScheduleSection client={client} agreementId={id} />

      <Link className="inline-block rounded-md bg-sky-700 px-3 py-1.5 text-white" href={`/documents/${a.document_id}`}>{t("detail.openDocument")}</Link>
    </main>
  );
}

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

/** DPA-10: the registry's own start/end dates and renewal settings — editing effective_to/renewal_notice_days
 *  reschedules the single expiry reminder (notifies role LEGAL) server-side. */
function RenewalScheduleSection({ client, agreement }: { client: ApiClient; agreement: Agreement }) {
  const t = useTranslations("agreements");
  const canUpdate = usePermission("agreement.dpa.update");
  const setSchedule = useSetAgreementSchedule(client, agreement.id);
  const [editing, setEditing] = useState(false);
  const [effectiveFrom, setEffectiveFrom] = useState("");
  const [effectiveTo, setEffectiveTo] = useState("");
  const [autoRenew, setAutoRenew] = useState(false);
  const [renewalNoticeDays, setRenewalNoticeDays] = useState(60);

  const startEdit = () => {
    setSchedule.reset();
    setEffectiveFrom(agreement.effective_from ?? "");
    setEffectiveTo(agreement.effective_to ?? "");
    setAutoRenew(agreement.auto_renew);
    setRenewalNoticeDays(agreement.renewal_notice_days);
    setEditing(true);
  };

  const submit = () => {
    setSchedule.mutate(
      {
        rowVersion: agreement.row_version,
        input: {
          effective_from: effectiveFrom || null,
          effective_to: effectiveTo || null,
          auto_renew: autoRenew,
          renewal_notice_days: renewalNoticeDays,
        },
      },
      { onSuccess: () => setEditing(false) },
    );
  };

  return (
    <section className="space-y-3 rounded-md border border-slate-200 bg-white p-4">
      <h2 className="font-semibold">{t("detail.renewalSchedule.title")}</h2>

      {!agreement.effective_to && !editing && <p className="text-slate-500">{t("detail.renewalSchedule.noEndDate")}</p>}

      {editing ? (
        <div className="grid gap-3 sm:grid-cols-2">
          <label className="block"><span className="block text-slate-600">{t("detail.renewalSchedule.effectiveFrom")}</span>
            <input type="date" className={INPUT} value={effectiveFrom} onChange={(e) => setEffectiveFrom(e.target.value)} /></label>
          <label className="block"><span className="block text-slate-600">{t("detail.renewalSchedule.effectiveTo")}</span>
            <input type="date" className={INPUT} value={effectiveTo} onChange={(e) => setEffectiveTo(e.target.value)} /></label>
          <label className="flex items-center gap-2">
            <input type="checkbox" checked={autoRenew} onChange={(e) => setAutoRenew(e.target.checked)} />
            <span className="text-slate-600">{t("detail.renewalSchedule.autoRenew")}</span>
          </label>
          <label className="block"><span className="block text-slate-600">{t("detail.renewalSchedule.renewalNoticeDays")}</span>
            <input type="number" min={0} className={INPUT} value={renewalNoticeDays}
              onChange={(e) => setRenewalNoticeDays(Number(e.target.value))} /></label>
          {setSchedule.isError && <p className="text-red-700 sm:col-span-2" role="alert">{t("detail.renewalSchedule.saveError", { detail: detail(setSchedule.error) })}</p>}
          <div className="flex gap-2 sm:col-span-2">
            <Button onClick={submit} disabled={setSchedule.isPending}>{t("detail.renewalSchedule.save")}</Button>
            <Button variant="secondary" onClick={() => setEditing(false)}>{t("detail.renewalSchedule.cancel")}</Button>
          </div>
        </div>
      ) : (
        canUpdate && <Button variant="secondary" onClick={startEdit}>{t("detail.renewalSchedule.edit")}</Button>
      )}
    </section>
  );
}

/** DPA-04: the ม.40 processing-schedule annex, composed live from every RoPA activity the agreement
 *  covers — read-only, nothing here is ever saved (it always matches the activity's current RoPA data). */
function ProcessingScheduleSection({ client, agreementId }: { client: ApiClient; agreementId: string }) {
  const t = useTranslations("agreements");
  const schedule = useProcessingSchedule(client, agreementId);

  return (
    <section className="space-y-3 rounded-md border border-slate-200 bg-white p-4">
      <h2 className="font-semibold">{t("detail.schedule.title")}</h2>
      {schedule.isPending && <p className="text-slate-500">{t("loading")}</p>}
      {schedule.data && schedule.data.activities.length === 0 && (
        <p className="text-slate-500">{t("detail.schedule.empty")}</p>
      )}
      {(schedule.data?.activities ?? []).map((sa) => (
        <div key={sa.activity_id} className="space-y-2 rounded-md border border-slate-100 p-3">
          <p className="font-medium">
            {sa.activity_name} <span className="text-xs text-slate-500">({sa.activity_code})</span>
          </p>

          <div>
            <p className="text-xs font-medium text-slate-500">{t("detail.schedule.purposes")}</p>
            <ul className="list-inside list-disc text-sm">
              {sa.purposes.map((p, i) => (
                <li key={i}>
                  {p.text} — {p.lawful_basis_name_th || p.lawful_basis_code}
                </li>
              ))}
              {sa.purposes.length === 0 && <li className="list-none text-slate-400">—</li>}
            </ul>
          </div>

          <div>
            <p className="text-xs font-medium text-slate-500">{t("detail.schedule.data")}</p>
            <ul className="list-inside list-disc text-sm">
              {sa.data.map((d, i) => (
                <li key={i}>
                  {d.category_name_th} / {d.subject_type_name_th}
                  {d.is_sensitive ? ` (${t("detail.schedule.sensitive")})` : ""}
                </li>
              ))}
              {sa.data.length === 0 && <li className="list-none text-slate-400">—</li>}
            </ul>
          </div>

          <div>
            <p className="text-xs font-medium text-slate-500">{t("detail.schedule.retention")}</p>
            <ul className="list-inside list-disc text-sm">
              {sa.retention.map((r, i) => (
                <li key={i}>
                  {r.category_name_th ?? "—"}: {r.retention_months ?? "—"} {t("detail.schedule.months")} ({r.retention_basis}, {r.disposal_method})
                </li>
              ))}
              {sa.retention.length === 0 && <li className="list-none text-slate-400">—</li>}
            </ul>
          </div>

          <div>
            <p className="text-xs font-medium text-slate-500">{t("detail.schedule.securityMeasures")}</p>
            <ul className="list-inside list-disc text-sm">
              {sa.security_measures.map((m, i) => (
                <li key={i}>{m.name}</li>
              ))}
              {sa.security_measures.length === 0 && <li className="list-none text-slate-400">—</li>}
            </ul>
          </div>
        </div>
      ))}
    </section>
  );
}

const PARTY_ROLES = ["disclosing", "receiving", "joint_controller", "controller", "processor"] as const;

/** DSA-02: every party on the agreement — the counterparty CreateWizard wrote plus any added since, each
 *  either an external party (ORG-06) or one of our own legal entities named as a joint controller. The
 *  acceptance criterion itself is just this panel existing: an agreement can carry more than two. */
function PartiesPanel({ client, agreementId }: { client: ApiClient; agreementId: string }) {
  const t = useTranslations("agreements");
  const canUpdate = usePermission("agreement.dpa.update");
  const parties = useAgreementParties(client, agreementId);
  const externalParties = useExternalParties(client, {});
  const legalEntities = useLegalEntities(client);
  const addParty = useAddAgreementParty(client, agreementId);
  const removeParty = useRemoveAgreementParty(client, agreementId);

  const [partyKind, setPartyKind] = useState<"external" | "ours">("external");
  const [partyRefId, setPartyRefId] = useState("");
  const [partyRole, setPartyRole] = useState<AgreementPartyInput["party_role"]>("receiving");

  const externalRows = externalParties.data?.pages.flatMap((p) => p.data) ?? [];

  const submit = () => {
    if (!partyRefId) return;
    addParty.mutate(
      {
        party_id: partyKind === "external" ? partyRefId : undefined,
        legal_entity_id: partyKind === "ours" ? partyRefId : undefined,
        party_role: partyRole,
      },
      { onSuccess: () => setPartyRefId("") },
    );
  };

  return (
    <section className="space-y-3 rounded-md border border-slate-200 bg-white p-4">
      <h2 className="font-semibold">{t("detail.parties.title")}</h2>

      <ul className="divide-y divide-slate-100 rounded-md border border-slate-200">
        {(parties.data ?? []).map((p) => (
          <li key={p.id} className="flex items-center justify-between gap-2 p-2">
            <div>
              <p className="font-medium">{t(`detail.parties.roles.${p.party_role}`)}</p>
              {p.signatory_name && <p className="text-xs text-slate-500">{p.signatory_name}</p>}
            </div>
            {canUpdate && (
              <button type="button" className="text-xs text-red-700 underline" onClick={() => removeParty.mutate(p.id)}>
                {t("detail.parties.remove")}
              </button>
            )}
          </li>
        ))}
        {(parties.data ?? []).length === 0 && <li className="p-2 text-slate-500">{t("detail.parties.empty")}</li>}
      </ul>

      {canUpdate && (
        <div className="flex flex-wrap items-center gap-2">
          <select className="rounded-md border border-slate-300 px-2 py-1" value={partyKind}
            onChange={(e) => { setPartyKind(e.target.value as "external" | "ours"); setPartyRefId(""); }}>
            <option value="external">{t("detail.parties.kindExternal")}</option>
            <option value="ours">{t("detail.parties.kindOurs")}</option>
          </select>
          <select className="flex-1 rounded-md border border-slate-300 px-2 py-1" value={partyRefId} onChange={(e) => setPartyRefId(e.target.value)}>
            <option value="">{t("detail.parties.pick")}</option>
            {partyKind === "external"
              ? externalRows.map((p) => <option key={p.id} value={p.id}>{p.name_th}</option>)
              : (legalEntities.data ?? []).map((le) => <option key={le.id} value={le.id}>{le.name_th}</option>)}
          </select>
          <select className="rounded-md border border-slate-300 px-2 py-1" value={partyRole}
            onChange={(e) => setPartyRole(e.target.value as AgreementPartyInput["party_role"])}>
            {PARTY_ROLES.map((r) => <option key={r} value={r}>{t(`detail.parties.roles.${r}`)}</option>)}
          </select>
          <Button onClick={submit} disabled={!partyRefId || addParty.isPending}>{t("detail.parties.add")}</Button>
        </div>
      )}
      {addParty.isError && <p className="text-red-700" role="alert">{t("detail.parties.saveError", { detail: detail(addParty.error) })}</p>}
    </section>
  );
}

/** DPA-03: the mandatory-clause panel — what's attached, what's still missing (blocks PLT-08's own
 *  submit-for-approval endpoint), and an add form drawing from the published clause library (DPA-01). */
function ClausePanel({ client, agreementId, isDraft }: { client: ApiClient; agreementId: string; isDraft: boolean }) {
  const t = useTranslations("agreements");
  const canUpdate = usePermission("agreement.dpa.update");
  const [selected, setSelected] = useState("");
  const clauses = useAgreementClauses(client, agreementId);
  const missing = useMissingClauses(client, agreementId);
  const library = useClauses(client, { applies_to: "dpa", published_only: true });
  const addClause = useAddAgreementClause(client, agreementId);
  const removeClause = useRemoveAgreementClause(client, agreementId);

  const attachedCodes = new Set((clauses.data ?? []).map((c) => c.clause_code));
  const available = (library.data ?? []).filter((c) => !attachedCodes.has(c.code));

  return (
    <section className="space-y-3 rounded-md border border-slate-200 bg-white p-4">
      <h2 className="font-semibold">{t("detail.clauses.title")}</h2>

      {missing.data && missing.data.length > 0 && (
        <div className="rounded-md border border-amber-300 bg-amber-50 p-3 text-amber-900">
          <p className="font-medium">{t("detail.clauses.missingTitle")}</p>
          <ul className="list-inside list-disc">
            {missing.data.map((m) => (
              <li key={m.clause_code}>
                {m.clause_code} ({m.legal_ref})
              </li>
            ))}
          </ul>
        </div>
      )}
      {missing.data && missing.data.length === 0 && (
        <p className="text-emerald-700">{t("detail.clauses.allAttached")}</p>
      )}

      <ul className="divide-y divide-slate-100 rounded-md border border-slate-200">
        {(clauses.data ?? []).map((c) => (
          <li key={c.id} className="flex items-center justify-between gap-2 p-2">
            <div>
              <p className="font-medium">{c.clause_title || c.clause_code}</p>
              <p className="text-xs text-slate-500">
                {c.clause_code} · {c.legal_ref}
                {c.is_mandatory ? ` · ${t("detail.clauses.mandatory")}` : ""}
              </p>
            </div>
            {canUpdate && isDraft && (
              <button
                type="button"
                className="text-xs text-red-700 underline"
                onClick={() => removeClause.mutate(c.id)}
              >
                {t("detail.clauses.remove")}
              </button>
            )}
          </li>
        ))}
        {(clauses.data ?? []).length === 0 && <li className="p-2 text-slate-500">{t("detail.clauses.empty")}</li>}
      </ul>

      {canUpdate && isDraft && (
        <div className="flex items-center gap-2">
          <select
            className="flex-1 rounded-md border border-slate-300 px-2 py-1"
            value={selected}
            onChange={(e) => setSelected(e.target.value)}
          >
            <option value="">{t("detail.clauses.pick")}</option>
            {available.map((c) => (
              <option key={c.id} value={c.id}>
                {c.body.th.title}
              </option>
            ))}
          </select>
          <button
            type="button"
            disabled={!selected || addClause.isPending}
            className="rounded-md bg-sky-700 px-3 py-1.5 text-white disabled:opacity-50"
            onClick={() => {
              addClause.mutate({ clause_id: selected });
              setSelected("");
            }}
          >
            {t("detail.clauses.add")}
          </button>
        </div>
      )}
    </section>
  );
}
