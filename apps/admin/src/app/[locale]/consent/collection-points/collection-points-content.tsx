"use client";

import { useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useCollectionPointMutations,
  useCollectionPoints,
  useConsentPurposes,
  useLegalEntities,
  type CollectionPointInput,
  type ConsentChecklist,
  type ConsentCollectionPoint,
} from "@pdpa/api-client";
import { StatusBadge, input, problemText, useText } from "../shared";

const CHANNELS = ["web", "app", "kiosk", "call_center", "pos", "line", "paper", "api"] as const;
const CHECKS: (keyof ConsentChecklist)[] = ["separate_text", "not_bundled", "plain_language", "withdrawal_info"];

export function CollectionPointsContent({ portalUrl }: { portalUrl: string }) {
  const t = useTranslations("consent");
  const canRead = usePermission("consent.collectionpoint.read");
  const canCreate = usePermission("consent.collectionpoint.create");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const list = useCollectionPoints(client);
  const [selected, setSelected] = useState<string | "new" | null>(null);
  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;
  const current = list.data?.find((c) => c.id === selected);

  return (
    <main className="mx-auto max-w-6xl space-y-4 p-8 text-sm">
      <header className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{t("cp.title")}</h1>
          <p className="text-slate-600">{t("cp.intro")}</p>
        </div>
        {canCreate && <Button onClick={() => setSelected("new")} data-testid="new-cp">{t("cp.new")}</Button>}
      </header>
      <div className="grid gap-4 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
        <section className="rounded-md border border-slate-200 bg-white">
          {list.isPending && <p className="p-3 text-slate-500">{t("loading")}</p>}
          {list.data?.length === 0 && <p className="p-3 text-slate-500">{t("cp.empty")}</p>}
          <ul className="divide-y divide-slate-100">
            {list.data?.map((c) => (
              <li key={c.id}>
                <button className={`flex w-full items-center justify-between gap-2 px-3 py-2 text-left hover:bg-slate-50 ${c.id === selected ? "bg-slate-50" : ""}`} onClick={() => setSelected(c.id)} data-testid={`cp-${c.code}`}>
                  <span>
                    <span className="font-medium">{c.name}</span>
                    <span className="ml-2 font-mono text-xs text-slate-500">{c.code}</span>
                  </span>
                  <span className="flex shrink-0 items-center gap-2 whitespace-nowrap text-xs text-slate-500">
                    {t(`channel.${c.channel}`)}
                    <StatusBadge status={c.status} />
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </section>
        <section className="space-y-4">
          {selected === "new" && <CPForm onSaved={(cp) => setSelected(cp.id)} />}
          {current && (
            <>
              <CPForm key={`${current.id}-${current.row_version}`} cp={current} onSaved={() => undefined} />
              <PublishPanel cp={current} portalUrl={portalUrl} />
            </>
          )}
        </section>
      </div>
    </main>
  );
}

function CPForm({ cp, onSaved }: { cp?: ConsentCollectionPoint; onSaved: (cp: ConsentCollectionPoint) => void }) {
  const t = useTranslations("consent");
  const canEdit = usePermission(cp ? "consent.collectionpoint.update" : "consent.collectionpoint.create");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const entities = useLegalEntities(client);
  const purposes = useConsentPurposes(client);
  const m = useCollectionPointMutations(client);
  const text = useText();
  const [code, setCode] = useState(cp?.code ?? "");
  const [v, setV] = useState<CollectionPointInput>({
    name: cp?.name ?? "",
    channel: (cp?.channel ?? "web") as CollectionPointInput["channel"],
    legal_entity_id: cp?.legal_entity_id ?? "",
    allowed_origins: cp?.allowed_origins ?? [],
    purposes: cp?.purposes.map((p) => ({ purpose_id: p.purpose_id, required: p.required })) ?? [],
  });
  const entityId = v.legal_entity_id || entities.data?.[0]?.id || "";
  const chosen = new Map(v.purposes.map((p) => [p.purpose_id, p]));
  const toggle = (id: string, on: boolean) =>
    setV({ ...v, purposes: on ? [...v.purposes, { purpose_id: id, required: false }] : v.purposes.filter((p) => p.purpose_id !== id) });
  const setRequired = (id: string, required: boolean) => setV({ ...v, purposes: v.purposes.map((p) => (p.purpose_id === id ? { ...p, required } : p)) });
  const mutation = cp ? m.update : m.create;
  const save = () => {
    const input = { ...v, legal_entity_id: entityId };
    if (cp) m.update.mutate({ cp, input }, { onSuccess: onSaved });
    else m.create.mutate({ ...input, code }, { onSuccess: onSaved });
  };
  const readOnly = !canEdit || cp?.status === "retired";

  return (
    <div className="space-y-3 rounded-md border border-slate-200 bg-white p-4">
      <h2 className="font-semibold">{cp ? cp.name : t("cp.new")}</h2>
      <fieldset disabled={readOnly} className="space-y-3">
        <div className="grid gap-2 md:grid-cols-2">
          <label className="space-y-0.5">
            <span className="font-medium">{t("code")}</span>
            <input className={`${input} font-mono`} value={code} disabled={!!cp} onChange={(e) => setCode(e.target.value)} data-testid="cp-code" />
          </label>
          <label className="space-y-0.5">
            <span className="font-medium">{t("cp.name")}</span>
            <input className={input} value={v.name} onChange={(e) => setV({ ...v, name: e.target.value })} data-testid="cp-name" />
          </label>
          <label className="space-y-0.5">
            <span className="font-medium">{t("cp.channel")}</span>
            <select className={input} value={v.channel} onChange={(e) => setV({ ...v, channel: e.target.value as CollectionPointInput["channel"] })}>
              {CHANNELS.map((c) => <option key={c} value={c}>{t(`channel.${c}`)}</option>)}
            </select>
          </label>
          <label className="space-y-0.5">
            <span className="font-medium">{t("legalEntity")}</span>
            <select className={input} value={entityId} onChange={(e) => setV({ ...v, legal_entity_id: e.target.value })}>
              {entities.data?.map((e) => <option key={e.id} value={e.id}>{e.name_th}</option>)}
            </select>
          </label>
        </div>
        <fieldset className="space-y-1">
          <legend className="font-medium">{t("cp.purposes")}</legend>
          <p className="text-xs text-slate-500">{t("cp.purposesHint")}</p>
          <ul className="divide-y divide-slate-100 rounded-md border border-slate-200">
            {purposes.data?.filter((p) => p.status !== "retired" || chosen.has(p.id)).map((p) => (
              <li key={p.id} className="flex flex-wrap items-center justify-between gap-2 px-3 py-1.5">
                <label className="flex items-center gap-2">
                  <input type="checkbox" checked={chosen.has(p.id)} onChange={(e) => toggle(p.id, e.target.checked)} data-testid={`cp-purpose-${p.code}`} />
                  {text(p.live.name) || p.code}
                  {p.is_sensitive && <span className="rounded bg-rose-50 px-1.5 text-xs text-rose-800">{t("sensitive")}</span>}
                  {p.status !== "active" && <StatusBadge status={p.status} />}
                </label>
                {chosen.has(p.id) && !p.is_sensitive && (
                  <label className="flex items-center gap-1 text-xs">
                    <input type="checkbox" checked={chosen.get(p.id)!.required} onChange={(e) => setRequired(p.id, e.target.checked)} />
                    {t("cp.required")}
                  </label>
                )}
              </li>
            ))}
          </ul>
        </fieldset>
        <label className="block space-y-0.5">
          <span className="font-medium">{t("cp.origins")}</span>
          <textarea className={`${input} font-mono`} rows={2} placeholder="https://www.example.co.th" value={v.allowed_origins?.join("\n") ?? ""}
            onChange={(e) => setV({ ...v, allowed_origins: e.target.value.split("\n").map((s) => s.trim()).filter(Boolean) })} />
          <span className="text-xs text-slate-500">{t("cp.originsHint")}</span>
        </label>
      </fieldset>
      {mutation.isError && <p className="text-red-700" role="alert">{problemText(mutation.error)}</p>}
      {!readOnly && <Button onClick={save} disabled={mutation.isPending || (!cp && !code)} data-testid="save-cp">{t("save")}</Button>}
    </div>
  );
}

function PublishPanel({ cp, portalUrl }: { cp: ConsentCollectionPoint; portalUrl: string }) {
  const t = useTranslations("consent");
  const locale = useLocale();
  const canPublish = usePermission("consent.collectionpoint.publish");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const m = useCollectionPointMutations(client);
  const [checks, setChecks] = useState<ConsentChecklist>(cp.checklist ?? { separate_text: false, not_bundled: false, plain_language: false, withdrawal_info: false });
  const link = cp.public_key ? `${portalUrl.replace(/\/$/, "")}/${locale}/c/${cp.public_key}` : "";

  return (
    <div className="space-y-3 rounded-md border border-slate-200 bg-white p-4" data-testid="publish-panel">
      <h3 className="font-semibold">{t("cp.publishTitle")}</h3>
      {cp.status === "draft" && (
        <>
          <p className="text-slate-600">{t("cp.checklistIntro")}</p>
          <ul className="space-y-1">
            {CHECKS.map((c) => (
              <li key={c}>
                <label className="flex items-start gap-2">
                  <input type="checkbox" className="mt-1" checked={checks[c]} disabled={!canPublish} onChange={(e) => setChecks({ ...checks, [c]: e.target.checked })} data-testid={`check-${c}`} />
                  {t(`checklist.${c}`)}
                </label>
              </li>
            ))}
          </ul>
          {m.publish.isError && <PublishErrors error={m.publish.error} />}
          {canPublish && <Button onClick={() => m.publish.mutate({ cp, checklist: checks })} disabled={m.publish.isPending} data-testid="publish-cp">{t("cp.publish")}</Button>}
        </>
      )}
      {cp.public_key && (
        <dl className="space-y-2">
          <div>
            <dt className="font-medium">{t("cp.hostedLink")}</dt>
            <dd className="flex flex-wrap items-center gap-2">
              <a className="break-all text-sky-700 underline" href={link} target="_blank" rel="noreferrer" data-testid="hosted-link">{link}</a>
              <Button variant="ghost" onClick={() => navigator.clipboard?.writeText(link)}>{t("copy")}</Button>
            </dd>
          </div>
          <div>
            <dt className="font-medium">{t("cp.publicKey")}</dt>
            <dd className="break-all font-mono text-xs" data-testid="public-key">{cp.public_key}</dd>
            <dd className="text-xs text-slate-500">{t("cp.publicKeyHint")}</dd>
          </div>
        </dl>
      )}
      {cp.status === "active" && canPublish && (
        <div className="border-t border-slate-100 pt-3">
          <Button variant="secondary" onClick={() => m.retire.mutate(cp)} disabled={m.retire.isPending}>{t("cp.retire")}</Button>
          <p className="text-xs text-slate-500">{t("cp.retireHint")}</p>
          {m.retire.isError && <p className="text-red-700" role="alert">{problemText(m.retire.error)}</p>}
        </div>
      )}
    </div>
  );
}

function PublishErrors({ error }: { error: unknown }) {
  const t = useTranslations("consent");
  const p = error as { code?: string; errors?: { field: string; code: string }[] };
  if (p?.code !== "consent.publish_checks") return <p className="text-red-700" role="alert">{problemText(error)}</p>;
  return (
    <ul className="list-disc space-y-0.5 pl-5 text-red-700" role="alert" data-testid="publish-errors">
      {(p.errors ?? []).map((e, i) => <li key={i}>{t.has(`checks.${e.code}`) ? t(`checks.${e.code}`) : e.code}</li>)}
    </ul>
  );
}
