"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useLegalEntities,
  useOrgUnits,
  useMasterData,
  useConsentPurposes,
  useExternalParties,
  useActivityMutations,
  type ProcessingActivityWizardLegalReason,
  type ProcessingActivityWizardRetention,
  type ProcessingActivityWizardRecipientKind,
} from "@pdpa/api-client";
import { useRouter } from "@/i18n/routing";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";

type Step = "basics" | "purpose" | "data" | "retention" | "recipient" | "summary";
const STEPS: Step[] = ["basics", "purpose", "data", "retention", "recipient", "summary"];

const LEGAL_REASONS: ProcessingActivityWizardLegalReason[] = ["consent", "contract", "law", "benefit"];
const RETENTION_CHOICES: ProcessingActivityWizardRetention[] = ["until_withdrawn", "contract_plus_1y", "five_years", "ten_years"];
const RECIPIENT_KINDS: ProcessingActivityWizardRecipientKind[] = ["processor", "other_controller", "government"];

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

/** RTG-05: a plain-language wizard — no RoPA/legal jargon in any question — that maps each answer to a
 * real RoPA field (purpose + lawful basis, who the data is about, what's collected, how long it's kept,
 * optionally who it's shared with), finishing with a summary before the one create call fires, the same
 * "preview before the one call" pattern RTG-04's own batch wizard already uses. */
export function RopaWizardContent() {
  const t = useTranslations("ropaWizard");
  const canCreate = usePermission("ropa.activity.create");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const router = useRouter();

  const [step, setStep] = useState<Step>("basics");
  const [legalEntityId, setLegalEntityId] = useState("");
  const [orgUnitId, setOrgUnitId] = useState("");
  const [code, setCode] = useState("");
  const [name, setName] = useState("");

  const [purpose, setPurpose] = useState("");
  const [legalReason, setLegalReason] = useState<ProcessingActivityWizardLegalReason | "">("");
  const [consentPurposeId, setConsentPurposeId] = useState("");

  const [subjectTypeId, setSubjectTypeId] = useState("");
  const [dataCategoryIds, setDataCategoryIds] = useState<Set<string>>(new Set());

  const [retention, setRetention] = useState<ProcessingActivityWizardRetention | "">("");

  const [recipientKind, setRecipientKind] = useState<ProcessingActivityWizardRecipientKind | "">("");
  const [recipientPartyId, setRecipientPartyId] = useState("");
  const [recipientNote, setRecipientNote] = useState("");

  const entities = useLegalEntities(client);
  const units = useOrgUnits(client, legalEntityId || undefined);
  const subjectTypes = useMasterData(client, "data_subject_types");
  const dataCategories = useMasterData(client, "data_categories");
  const consentPurposes = useConsentPurposes(client);
  const parties = useExternalParties(client);
  const partyRows = parties.data?.pages.flatMap((p) => p.data) ?? [];
  const { createFromWizard } = useActivityMutations(client);

  if (!canCreate) {
    return <main className="mx-auto max-w-3xl p-8 text-slate-600">{t("forbidden")}</main>;
  }

  const toggleCategory = (id: string) => {
    setDataCategoryIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const categoryName = (id: string) => dataCategories.data?.find((c) => c.id === id)?.name_th ?? id;
  const subjectTypeName = (id: string) => subjectTypes.data?.find((s) => s.id === id)?.name_th ?? id;
  const partyName = (id: string) => partyRows.find((p) => p.id === id)?.name_th ?? id;

  const create = () => {
    if (!legalEntityId || !orgUnitId || !code.trim() || !name.trim() || !purpose.trim() || !legalReason || !subjectTypeId
      || dataCategoryIds.size === 0 || !retention) return;
    createFromWizard.mutate(
      {
        legal_entity_id: legalEntityId, org_unit_id: orgUnitId, code: code.trim(), name: name.trim(),
        purpose: purpose.trim(), legal_reason: legalReason,
        consent_purpose_id: legalReason === "consent" ? consentPurposeId || undefined : undefined,
        subject_type_id: subjectTypeId, data_category_ids: Array.from(dataCategoryIds),
        retention,
        recipient_kind: recipientKind || undefined,
        recipient_party_id: recipientKind ? recipientPartyId || undefined : undefined,
        recipient_note: recipientKind ? recipientNote.trim() || undefined : undefined,
      },
      { onSuccess: (a) => a && router.push(`/ropa/activities/${a.id}`) },
    );
  };

  return (
    <main className="mx-auto max-w-3xl space-y-6 p-8 text-sm">
      <header>
        <h1 className="text-xl font-semibold">{t("title")}</h1>
        <p className="text-slate-600">{t("intro")}</p>
      </header>

      <ol className="flex flex-wrap gap-3 text-xs text-slate-500" data-testid="wizard-steps">
        {STEPS.map((s, i) => (
          <li key={s} className={step === s ? "font-semibold text-sky-700" : undefined}>
            {i + 1}. {t(`steps.${s}`)}
          </li>
        ))}
      </ol>

      {step === "basics" && (
        <section className="space-y-3">
          <label className="block"><span className="block text-slate-600">{t("basics.name")}</span>
            <input className={INPUT} value={name} onChange={(e) => setName(e.target.value)} placeholder={t("basics.namePlaceholder")} /></label>
          <label className="block"><span className="block text-slate-600">{t("basics.code")}</span>
            <input className={INPUT} value={code} onChange={(e) => setCode(e.target.value)} maxLength={40} /></label>
          <label className="block"><span className="block text-slate-600">{t("basics.legalEntity")}</span>
            <select className={INPUT} value={legalEntityId} onChange={(e) => { setLegalEntityId(e.target.value); setOrgUnitId(""); }}>
              <option value="">{t("choose")}</option>
              {entities.data?.map((le) => <option key={le.id} value={le.id}>{le.name_th}</option>)}
            </select></label>
          <label className="block"><span className="block text-slate-600">{t("basics.orgUnit")}</span>
            <select className={INPUT} value={orgUnitId} onChange={(e) => setOrgUnitId(e.target.value)} disabled={!legalEntityId}>
              <option value="">{t("choose")}</option>
              {units.data?.map((u) => <option key={u.id} value={u.id}>{u.name_th}</option>)}
            </select></label>
          <Button onClick={() => setStep("purpose")} disabled={!legalEntityId || !orgUnitId || !code.trim() || !name.trim()}>{t("next")}</Button>
        </section>
      )}

      {step === "purpose" && (
        <section className="space-y-3">
          <label className="block"><span className="block text-slate-600">{t("purpose.why")}</span>
            <textarea className={INPUT} rows={3} value={purpose} onChange={(e) => setPurpose(e.target.value)}
              placeholder={t("purpose.whyPlaceholder")} /></label>
          <fieldset className="space-y-1">
            <legend className="text-slate-600">{t("purpose.reason")}</legend>
            {LEGAL_REASONS.map((r) => (
              <label key={r} className="flex items-center gap-2">
                <input type="radio" name="legalReason" checked={legalReason === r} onChange={() => setLegalReason(r)} />
                {t(`purpose.reasons.${r}`)}
              </label>
            ))}
          </fieldset>
          {legalReason === "consent" && (
            <label className="block"><span className="block text-slate-600">{t("purpose.consentPurpose")}</span>
              <select className={INPUT} value={consentPurposeId} onChange={(e) => setConsentPurposeId(e.target.value)}>
                <option value="">{t("choose")}</option>
                {consentPurposes.data?.map((p) => <option key={p.id} value={p.id}>{p.live?.name?.th ?? p.code}</option>)}
              </select>
            </label>
          )}
          <div className="flex gap-2">
            <Button variant="secondary" onClick={() => setStep("basics")}>{t("back")}</Button>
            <Button onClick={() => setStep("data")} disabled={!purpose.trim() || !legalReason || (legalReason === "consent" && !consentPurposeId)}>{t("next")}</Button>
          </div>
        </section>
      )}

      {step === "data" && (
        <section className="space-y-3">
          <label className="block"><span className="block text-slate-600">{t("data.subjectType")}</span>
            <select className={INPUT} value={subjectTypeId} onChange={(e) => setSubjectTypeId(e.target.value)}>
              <option value="">{t("choose")}</option>
              {subjectTypes.data?.map((s) => <option key={s.id} value={s.id}>{s.name_th}</option>)}
            </select></label>
          <fieldset className="space-y-1">
            <legend className="text-slate-600">{t("data.whatData")}</legend>
            <ul className="max-h-64 divide-y divide-slate-100 overflow-y-auto rounded-md border border-slate-200 bg-white">
              {dataCategories.data?.map((c) => (
                <li key={c.id} className="flex items-center gap-2 px-3 py-1.5">
                  <input type="checkbox" id={`cat-${c.id}`} checked={dataCategoryIds.has(c.id!)} onChange={() => toggleCategory(c.id!)} />
                  <label htmlFor={`cat-${c.id}`} className="flex-1">{c.name_th}{c.is_sensitive && <span className="ml-2 text-xs text-amber-700">{t("data.sensitive")}</span>}</label>
                </li>
              ))}
            </ul>
          </fieldset>
          <div className="flex gap-2">
            <Button variant="secondary" onClick={() => setStep("purpose")}>{t("back")}</Button>
            <Button onClick={() => setStep("retention")} disabled={!subjectTypeId || dataCategoryIds.size === 0}>{t("next")}</Button>
          </div>
        </section>
      )}

      {step === "retention" && (
        <section className="space-y-3">
          <fieldset className="space-y-1">
            <legend className="text-slate-600">{t("retention.howLong")}</legend>
            {RETENTION_CHOICES.map((r) => (
              <label key={r} className="flex items-center gap-2">
                <input type="radio" name="retention" checked={retention === r} onChange={() => setRetention(r)} />
                {t(`retention.choices.${r}`)}
              </label>
            ))}
          </fieldset>
          <div className="flex gap-2">
            <Button variant="secondary" onClick={() => setStep("data")}>{t("back")}</Button>
            <Button onClick={() => setStep("recipient")} disabled={!retention}>{t("next")}</Button>
          </div>
        </section>
      )}

      {step === "recipient" && (
        <section className="space-y-3">
          <p className="text-slate-600">{t("recipient.intro")}</p>
          <fieldset className="space-y-1">
            <legend className="text-slate-600">{t("recipient.who")}</legend>
            <label className="flex items-center gap-2">
              <input type="radio" name="recipientKind" checked={recipientKind === ""} onChange={() => setRecipientKind("")} />
              {t("recipient.none")}
            </label>
            {RECIPIENT_KINDS.map((k) => (
              <label key={k} className="flex items-center gap-2">
                <input type="radio" name="recipientKind" checked={recipientKind === k} onChange={() => setRecipientKind(k)} />
                {t(`recipient.kinds.${k}`)}
              </label>
            ))}
          </fieldset>
          {recipientKind && (
            <>
              <label className="block"><span className="block text-slate-600">{t("recipient.party")}</span>
                <select className={INPUT} value={recipientPartyId} onChange={(e) => setRecipientPartyId(e.target.value)}>
                  <option value="">{t("choose")}</option>
                  {partyRows.map((p) => <option key={p.id} value={p.id}>{p.name_th}</option>)}
                </select></label>
              <label className="block"><span className="block text-slate-600">{t("recipient.note")}</span>
                <input className={INPUT} value={recipientNote} onChange={(e) => setRecipientNote(e.target.value)} /></label>
            </>
          )}
          <div className="flex gap-2">
            <Button variant="secondary" onClick={() => setStep("retention")}>{t("back")}</Button>
            <Button onClick={() => setStep("summary")} disabled={!!recipientKind && !recipientPartyId}>{t("next")}</Button>
          </div>
        </section>
      )}

      {step === "summary" && (
        <section className="space-y-3" data-testid="wizard-summary">
          <h2 className="font-semibold">{t("summary.title")}</h2>
          <dl className="space-y-1 rounded-md border border-slate-200 bg-white p-3">
            <div><dt className="inline font-medium">{t("basics.name")}: </dt><dd className="inline">{name}</dd></div>
            <div><dt className="inline font-medium">{t("purpose.why")}: </dt><dd className="inline">{purpose}</dd></div>
            <div><dt className="inline font-medium">{t("purpose.reason")}: </dt><dd className="inline">{legalReason && t(`purpose.reasons.${legalReason}`)}</dd></div>
            <div><dt className="inline font-medium">{t("data.subjectType")}: </dt><dd className="inline">{subjectTypeName(subjectTypeId)}</dd></div>
            <div><dt className="inline font-medium">{t("data.whatData")}: </dt><dd className="inline">{Array.from(dataCategoryIds).map(categoryName).join(", ")}</dd></div>
            <div><dt className="inline font-medium">{t("retention.howLong")}: </dt><dd className="inline">{retention && t(`retention.choices.${retention}`)}</dd></div>
            {recipientKind && (
              <div><dt className="inline font-medium">{t("recipient.who")}: </dt>
                <dd className="inline">{t(`recipient.kinds.${recipientKind}`)} — {partyName(recipientPartyId)}</dd></div>
            )}
          </dl>
          {createFromWizard.isError && <p className="text-red-700" role="alert">{t("createError", { detail: detail(createFromWizard.error) })}</p>}
          <div className="flex gap-2">
            <Button variant="secondary" onClick={() => setStep("recipient")}>{t("back")}</Button>
            <Button onClick={create} disabled={createFromWizard.isPending}>{t("create")}</Button>
          </div>
        </section>
      )}
    </main>
  );
}
