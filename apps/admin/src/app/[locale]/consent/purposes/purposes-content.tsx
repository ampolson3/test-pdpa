"use client";

import { useEffect, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useConsentPurposes,
  useLegalEntities,
  useMasterData,
  usePurposeMutations,
  useRecordVersions,
  type ConsentPreference,
  type ConsentPurpose,
  type PurposeContent,
} from "@pdpa/api-client";
import { RecordVersions } from "@/components/record-versions";
import { StatusBadge, TextPair, input, problemText, useText } from "../shared";

const PURPOSE_TYPE = "consent_purpose";
const emptyContent: PurposeContent = { name: { th: "" }, consent_text: { th: "" }, data_category_codes: [], change_type: "minor", requires_reconsent: false, preferences: [] };

export function PurposesContent() {
  const t = useTranslations("consent");
  const canRead = usePermission("consent.purpose.read");
  const canCreate = usePermission("consent.purpose.create");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const list = useConsentPurposes(client);
  const text = useText();
  const [selected, setSelected] = useState<string | "new" | null>(null);
  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;
  const current = list.data?.find((p) => p.id === selected);

  return (
    <main className="mx-auto max-w-6xl space-y-4 p-8 text-sm">
      <header className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{t("purposes.title")}</h1>
          <p className="text-slate-600">{t("purposes.intro")}</p>
        </div>
        {canCreate && <Button onClick={() => setSelected("new")} data-testid="new-purpose">{t("purposes.new")}</Button>}
      </header>
      <div className="grid gap-4 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
        <section className="rounded-md border border-slate-200 bg-white">
          {list.isPending && <p className="p-3 text-slate-500">{t("loading")}</p>}
          {list.data?.length === 0 && <p className="p-3 text-slate-500">{t("purposes.empty")}</p>}
          <ul className="divide-y divide-slate-100">
            {list.data?.map((p) => (
              <li key={p.id}>
                <button className={`flex w-full items-center justify-between gap-2 px-3 py-2 text-left hover:bg-slate-50 ${p.id === selected ? "bg-slate-50" : ""}`} onClick={() => setSelected(p.id)} data-testid={`purpose-${p.code}`}>
                  <span>
                    <span className="font-medium">{text(p.live.name) || p.code}</span>
                    <span className="ml-2 font-mono text-xs text-slate-500">{p.code}</span>
                    {p.is_sensitive && <span className="ml-2 whitespace-nowrap rounded bg-rose-50 px-1.5 text-xs text-rose-800">{t("sensitive")}</span>}
                  </span>
                  <span className="flex shrink-0 items-center gap-2 whitespace-nowrap text-xs text-slate-500">
                    {p.current_version ? t("purposes.versionNo", { no: p.current_version }) : t("purposes.unpublished")}
                    <StatusBadge status={p.status} />
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </section>
        <section>
          {selected === "new" && <NewPurpose onCreated={(id) => setSelected(id)} />}
          {current && <PurposeEditor key={current.id} purpose={current} />}
        </section>
      </div>
    </main>
  );
}

function NewPurpose({ onCreated }: { onCreated: (id: string) => void }) {
  const t = useTranslations("consent");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const entities = useLegalEntities(client);
  const m = usePurposeMutations(client);
  const [code, setCode] = useState("");
  const [entity, setEntity] = useState("");
  const [content, setContent] = useState<PurposeContent>(emptyContent);
  const entityId = entity || entities.data?.[0]?.id || "";
  return (
    <div className="space-y-3 rounded-md border border-slate-200 bg-white p-4">
      <h2 className="font-semibold">{t("purposes.new")}</h2>
      <div className="grid gap-2 md:grid-cols-2">
        <label className="space-y-0.5">
          <span className="font-medium">{t("code")}</span>
          <input className={`${input} font-mono`} value={code} onChange={(e) => setCode(e.target.value)} data-testid="purpose-code" />
        </label>
        <label className="space-y-0.5">
          <span className="font-medium">{t("legalEntity")}</span>
          <select className={input} value={entityId} onChange={(e) => setEntity(e.target.value)}>
            {entities.data?.map((e) => <option key={e.id} value={e.id}>{e.name_th}</option>)}
          </select>
        </label>
      </div>
      <ContentForm value={content} onChange={setContent} />
      {m.create.isError && <p className="text-red-700" role="alert">{problemText(m.create.error)}</p>}
      <Button disabled={!code || !entityId || m.create.isPending} data-testid="create-purpose"
        onClick={() => m.create.mutate({ code, legal_entity_id: entityId, content }, { onSuccess: (p) => onCreated(p.id) })}>
        {t("purposes.create")}
      </Button>
    </div>
  );
}

function PurposeEditor({ purpose }: { purpose: ConsentPurpose }) {
  const t = useTranslations("consent");
  const canUpdate = usePermission("consent.purpose.update");
  const canPublish = usePermission("consent.purpose.publish");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const versions = useRecordVersions(client, PURPOSE_TYPE, purpose.id);
  const m = usePurposeMutations(client);
  const qc = useQueryClient();
  const open = versions.data?.find((v) => v.status === "draft" || v.status === "in_review" || v.status === "approved");
  // Publishing happens in RecordVersions (PLT-08); refresh the purpose when the published version changes there.
  const publishedNo = versions.data?.find((v) => v.status === "published")?.version;
  useEffect(() => {
    if (publishedNo) qc.invalidateQueries({ queryKey: ["consent", "purposes"] });
  }, [publishedNo, qc]);
  const locked = !!open && open.status !== "draft";
  if (versions.isPending) return <p className="text-slate-500">{t("loading")}</p>;
  const start = (open?.snapshot as PurposeContent | undefined) ?? purpose.live;

  return (
    <div className="space-y-4">
      <div className="space-y-3 rounded-md border border-slate-200 bg-white p-4">
        <header className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="font-semibold">{purpose.code}</h2>
          <span className="flex items-center gap-2 text-xs text-slate-600">
            {t("lawfulBasis")}: <span className="font-mono">{purpose.lawful_basis}</span>
            <StatusBadge status={purpose.status} />
          </span>
        </header>
        {purpose.requires_explicit && <p className="rounded-md bg-rose-50 p-2 text-rose-900">{t("purposes.explicitNotice")}</p>}
        <DraftForm key={open?.id ?? "live"} start={start} disabled={!canUpdate || locked || purpose.status === "retired"}
          onSave={(content) => m.saveDraft.mutate({ id: purpose.id, content })} saving={m.saveDraft.isPending} />
        {locked && <p className="text-amber-800">{t("purposes.locked")}</p>}
        {m.saveDraft.isError && <p className="text-red-700" role="alert">{problemText(m.saveDraft.error)}</p>}
        {m.saveDraft.isSuccess && <p className="text-emerald-700" role="status">{t("purposes.saved")}</p>}
        {canUpdate && purpose.status === "active" && (
          <div className="border-t border-slate-100 pt-3">
            <Button variant="secondary" onClick={() => m.retire.mutate(purpose.id)} disabled={m.retire.isPending}>{t("purposes.retire")}</Button>
            {m.retire.isError && <p className="text-red-700" role="alert">{problemText(m.retire.error)}</p>}
          </div>
        )}
      </div>
      <div className="rounded-md border border-slate-200 bg-white p-4">
        <RecordVersions entityType={PURPOSE_TYPE} entityId={purpose.id} canEdit={canUpdate} canPublish={canPublish} />
      </div>
      {purpose.versions.length > 0 && (
        <div className="rounded-md border border-slate-200 bg-white p-4">
          <h3 className="mb-2 font-semibold">{t("purposes.published")}</h3>
          <ul className="space-y-1">
            {purpose.versions.map((v) => (
              <li key={v.id} className="flex flex-wrap gap-2">
                <span className="font-medium">{t("purposes.versionNo", { no: v.version })}</span>
                <span className="text-slate-500">{t(`changeType.${v.change_type}`)}</span>
                {v.requires_reconsent && <span className="text-amber-800">{t("purposes.reconsent")}</span>}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

function DraftForm({ start, disabled, onSave, saving }: { start: PurposeContent; disabled: boolean; onSave: (c: PurposeContent) => void; saving: boolean }) {
  const t = useTranslations("consent");
  // A new version is a minor or material change of the live one ("initial" is only the first version's type).
  const [content, setContent] = useState<PurposeContent>({ ...emptyContent, ...start, change_type: start.change_type === "material" ? "material" : "minor" });
  return (
    <div className="space-y-3">
      <fieldset disabled={disabled} className="space-y-3">
        <ContentForm value={content} onChange={setContent} />
      </fieldset>
      {!disabled && <Button onClick={() => onSave(content)} disabled={saving} data-testid="save-draft">{t("purposes.saveDraft")}</Button>}
    </div>
  );
}

function ContentForm({ value, onChange }: { value: PurposeContent; onChange: (c: PurposeContent) => void }) {
  const t = useTranslations("consent");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const cats = useMasterData(client, "data_categories");
  const text = useText();
  const codes = value.data_category_codes ?? [];
  const sensitive = (cats.data ?? []).some((c) => c.is_sensitive && codes.includes(c.code));
  const set = (patch: Partial<PurposeContent>) => onChange({ ...value, ...patch });
  const num = (s: string) => (s === "" ? null : Number(s));
  return (
    <div className="space-y-3">
      <TextPair label={t("purposes.name")} value={value.name} onChange={(name) => set({ name })} testId="purpose-name" />
      <TextPair label={t("purposes.description")} value={value.description} onChange={(description) => set({ description })} multiline />
      <TextPair label={t("purposes.consentText")} value={value.consent_text} onChange={(consent_text) => set({ consent_text })} multiline testId="consent-text" />
      <fieldset className="space-y-1">
        <legend className="font-medium">{t("purposes.dataCategories")}</legend>
        <div className="flex flex-wrap gap-2">
          {cats.data?.map((c) => (
            <label key={c.code} className={`flex items-center gap-1 rounded-md px-2 py-1 ring-1 ${c.is_sensitive ? "ring-rose-200" : "ring-slate-200"}`}>
              <input type="checkbox" checked={codes.includes(c.code)} data-testid={`cat-${c.code}`}
                onChange={(e) => set({ data_category_codes: e.target.checked ? [...codes, c.code] : codes.filter((x) => x !== c.code) })} />
              {text({ th: c.name_th, en: c.name_en ?? undefined })}
              {c.is_sensitive && <span className="text-xs text-rose-700">{t("sensitive")}</span>}
            </label>
          ))}
        </div>
      </fieldset>
      {sensitive && <TextPair label={t("purposes.explicitText")} value={value.explicit_text} onChange={(explicit_text) => set({ explicit_text })} multiline testId="explicit-text" />}
      <div className="grid gap-2 md:grid-cols-4">
        <label className="space-y-0.5">
          <span className="font-medium">{t("purposes.minAge")}</span>
          <input type="number" min={0} max={25} className={input} value={value.min_age ?? ""} onChange={(e) => set({ min_age: num(e.target.value) })} />
        </label>
        <label className="space-y-0.5">
          <span className="font-medium">{t("purposes.lifespan")}</span>
          <input type="number" min={1} max={3650} className={input} value={value.lifespan_days ?? ""} onChange={(e) => set({ lifespan_days: num(e.target.value) })} />
        </label>
        <label className="space-y-0.5">
          <span className="font-medium">{t("purposes.changeType")}</span>
          <select className={input} value={value.change_type ?? "minor"} onChange={(e) => set({ change_type: e.target.value as "minor" | "material" })}>
            <option value="minor">{t("changeType.minor")}</option>
            <option value="material">{t("changeType.material")}</option>
          </select>
        </label>
        <label className="flex items-center gap-2 self-end pb-2">
          <input type="checkbox" checked={!!value.requires_reconsent} onChange={(e) => set({ requires_reconsent: e.target.checked })} />
          {t("purposes.reconsent")}
        </label>
      </div>
      <PreferencesEditor value={value.preferences ?? []} onChange={(preferences) => set({ preferences })} />
    </div>
  );
}

/** Preferences: one per line of options "value | Thai label | English label". */
function PreferencesEditor({ value, onChange }: { value: ConsentPreference[]; onChange: (v: ConsentPreference[]) => void }) {
  const t = useTranslations("consent");
  const setAt = (i: number, p: ConsentPreference) => onChange(value.map((x, j) => (j === i ? p : x)));
  const optionsText = (p: ConsentPreference) => p.options.map((o) => [o.value, o.label.th, o.label.en ?? ""].join(" | ")).join("\n");
  const parse = (s: string) =>
    s.split("\n").map((l) => l.split("|").map((x) => x.trim())).filter((x) => x[0])
      .map(([v, th, en]) => ({ value: v, label: { th: th || v, ...(en ? { en } : {}) } }));
  return (
    <fieldset className="space-y-2">
      <legend className="font-medium">{t("purposes.preferences")}</legend>
      {value.map((p, i) => (
        <div key={i} className="grid gap-2 rounded-md bg-slate-50 p-2 md:grid-cols-[8rem_1fr_1fr_8rem_auto]">
          <input className={`${input} font-mono`} placeholder={t("code")} value={p.code} onChange={(e) => setAt(i, { ...p, code: e.target.value })} />
          <input className={input} placeholder={t("thai")} value={p.name.th} onChange={(e) => setAt(i, { ...p, name: { ...p.name, th: e.target.value } })} />
          <input className={input} placeholder={t("english")} value={p.name.en ?? ""} onChange={(e) => setAt(i, { ...p, name: { ...p.name, en: e.target.value || undefined } })} />
          <select className={input} value={p.type} onChange={(e) => setAt(i, { ...p, type: e.target.value as ConsentPreference["type"] })}>
            {(["channel", "topic", "frequency", "other"] as const).map((x) => <option key={x} value={x}>{t(`prefType.${x}`)}</option>)}
          </select>
          <Button variant="secondary" onClick={() => onChange(value.filter((_, j) => j !== i))}>{t("remove")}</Button>
          <textarea className={`${input} md:col-span-5`} rows={3} placeholder={t("purposes.optionsHint")} defaultValue={optionsText(p)} onBlur={(e) => setAt(i, { ...p, options: parse(e.target.value) })} />
        </div>
      ))}
      {value.length < 10 && (
        <Button variant="secondary" onClick={() => onChange([...value, { code: "", name: { th: "" }, type: "channel", options: [] }])}>{t("purposes.addPreference")}</Button>
      )}
    </fieldset>
  );
}
