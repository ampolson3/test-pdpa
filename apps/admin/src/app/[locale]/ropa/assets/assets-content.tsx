"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useAssets,
  useSaveAsset,
  useLegalEntities,
  useOrgUnits,
  useExternalParties,
  type Asset,
  type AssetType,
} from "@pdpa/api-client";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";
const TYPES: AssetType[] = ["application", "database", "file_share", "saas", "paper", "device", "other"];
const HOSTING = ["on_prem", "cloud", "hybrid"] as const;
const CLASSIFICATIONS = ["public", "internal", "confidential", "restricted"] as const;

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

type Draft = {
  id?: string; rowVersion?: number; name: string; asset_type: AssetType; org_unit_id: string; owner_user_id: string;
  provider_party_id: string; hosting_country_code: string; hosting_type: string; classification: string; status: "active" | "retired";
};

const blank: Draft = { name: "", asset_type: "application", org_unit_id: "", owner_user_id: "", provider_party_id: "",
  hosting_country_code: "", hosting_type: "", classification: "", status: "active" };

function fromAsset(a: Asset): Draft {
  return { id: a.id, rowVersion: a.row_version, name: a.name, asset_type: a.asset_type, org_unit_id: a.org_unit_id ?? "",
    owner_user_id: a.owner_user_id ?? "", provider_party_id: a.provider_party_id ?? "", hosting_country_code: a.hosting_country_code ?? "",
    hosting_type: a.hosting_type ?? "", classification: a.classification ?? "", status: a.status };
}

export function AssetsContent() {
  const t = useTranslations("assets");
  const canRead = usePermission("ropa.inventory.read");
  const canCreate = usePermission("ropa.inventory.create");
  const canUpdate = usePermission("ropa.inventory.update");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const [typeFilter, setTypeFilter] = useState<AssetType | "">("");
  const [q, setQ] = useState("");
  const list = useAssets(client, { asset_type: typeFilter || undefined, q: q || undefined });
  const save = useSaveAsset(client);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [saved, setSaved] = useState(false);
  const [legalEntityId, setLegalEntityId] = useState<string>();
  const entities = useLegalEntities(client);
  const units = useOrgUnits(client, legalEntityId);
  const parties = useExternalParties(client);

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  const rows = list.data?.pages.flatMap((p) => p.data) ?? [];
  const set = (p: Partial<Draft>) => { setSaved(false); setDraft({ ...(draft ?? blank), ...p }); };
  const editable = draft && (draft.id ? canUpdate : canCreate);

  const submit = () => {
    if (!draft) return;
    save.mutate({
      id: draft.id, rowVersion: draft.rowVersion,
      input: {
        name: draft.name, asset_type: draft.asset_type, org_unit_id: draft.org_unit_id || undefined,
        owner_user_id: draft.owner_user_id || undefined, provider_party_id: draft.provider_party_id || undefined,
        hosting_country_code: draft.hosting_country_code || undefined, hosting_type: (draft.hosting_type || undefined) as never,
        classification: (draft.classification || undefined) as never, status: draft.status,
      },
    }, { onSuccess: () => { setDraft(null); setSaved(true); } });
  };

  return (
    <main className="mx-auto max-w-6xl space-y-6 p-8 text-sm">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{t("title")}</h1>
          <p className="text-slate-600">{t("intro")}</p>
        </div>
        {canCreate && <Button onClick={() => { save.reset(); setSaved(false); setDraft({ ...blank }); }}>{t("newAsset")}</Button>}
      </header>

      <div className="flex flex-wrap items-end gap-3">
        <label><span className="block text-slate-600">{t("filterType")}</span>
          <select className={INPUT} value={typeFilter} onChange={(e) => setTypeFilter(e.target.value as AssetType | "")}>
            <option value="">{t("allTypes")}</option>
            {TYPES.map((x) => <option key={x} value={x}>{t(`types.${x}`)}</option>)}
          </select>
        </label>
        <label className="flex-1"><span className="block text-slate-600">{t("search")}</span>
          <input className={INPUT} value={q} onChange={(e) => setQ(e.target.value)} placeholder={t("searchHint")} /></label>
      </div>

      {draft && (
        <fieldset className="grid gap-3 rounded-md border border-slate-200 bg-white p-4 sm:grid-cols-2" disabled={!editable}>
          <legend className="px-1 font-semibold">{t("form.details")}</legend>
          <label><span className="block text-slate-600">{t("form.name")}</span>
            <input className={INPUT} value={draft.name} onChange={(e) => set({ name: e.target.value })} /></label>
          <label><span className="block text-slate-600">{t("form.assetType")}</span>
            <select className={INPUT} value={draft.asset_type} onChange={(e) => set({ asset_type: e.target.value as AssetType })}>
              {TYPES.map((x) => <option key={x} value={x}>{t(`types.${x}`)}</option>)}
            </select>
          </label>
          <label><span className="block text-slate-600">{t("form.legalEntity")}</span>
            <select className={INPUT} value={legalEntityId ?? ""} onChange={(e) => { setLegalEntityId(e.target.value || undefined); set({ org_unit_id: "" }); }}>
              <option value="">{t("form.none")}</option>
              {entities.data?.map((le) => <option key={le.id} value={le.id}>{le.name_th}</option>)}
            </select>
          </label>
          <label><span className="block text-slate-600">{t("form.orgUnit")}</span>
            <select className={INPUT} value={draft.org_unit_id} onChange={(e) => set({ org_unit_id: e.target.value })} disabled={!legalEntityId}>
              <option value="">{t("form.none")}</option>
              {units.data?.map((u) => <option key={u.id} value={u.id}>{u.name_th}</option>)}
            </select>
          </label>
          <label><span className="block text-slate-600">{t("form.provider")}</span>
            <select className={INPUT} value={draft.provider_party_id} onChange={(e) => set({ provider_party_id: e.target.value })}>
              <option value="">{t("form.none")}</option>
              {parties.data?.pages.flatMap((p) => p.data).map((pt) => <option key={pt.id} value={pt.id}>{pt.name_th}</option>)}
            </select>
          </label>
          <label><span className="block text-slate-600">{t("form.hostingCountry")}</span>
            <input className={INPUT} maxLength={2} value={draft.hosting_country_code} onChange={(e) => set({ hosting_country_code: e.target.value.toUpperCase() })} /></label>
          <label><span className="block text-slate-600">{t("form.hostingType")}</span>
            <select className={INPUT} value={draft.hosting_type} onChange={(e) => set({ hosting_type: e.target.value })}>
              <option value="">{t("form.none")}</option>
              {HOSTING.map((x) => <option key={x} value={x}>{t(`hosting.${x}`)}</option>)}
            </select>
          </label>
          <label><span className="block text-slate-600">{t("form.classification")}</span>
            <select className={INPUT} value={draft.classification} onChange={(e) => set({ classification: e.target.value })}>
              <option value="">{t("form.none")}</option>
              {CLASSIFICATIONS.map((x) => <option key={x} value={x}>{t(`classification.${x}`)}</option>)}
            </select>
          </label>
          <label className="flex items-center gap-1"><input type="checkbox" checked={draft.status === "active"} onChange={(e) => set({ status: e.target.checked ? "active" : "retired" })} />{t("form.active")}</label>
          {save.isError && <p className="text-red-700 sm:col-span-2" role="alert">{t("form.saveError", { detail: detail(save.error) })}</p>}
          {saved && <p className="text-emerald-800 sm:col-span-2" role="status">{t("form.saved")}</p>}
          {editable && (
            <div className="flex gap-2 sm:col-span-2">
              <Button onClick={submit} disabled={save.isPending || !draft.name.trim()}>{t("form.save")}</Button>
              <Button variant="secondary" onClick={() => { save.reset(); setDraft(null); }}>{t("form.cancel")}</Button>
            </div>
          )}
        </fieldset>
      )}

      {list.isPending ? <p className="text-slate-500">{t("loading")}</p> : list.isError ? <p className="text-red-700">{t("loadError")}</p> : rows.length === 0 ? (
        <p className="rounded-md bg-amber-50 p-3 text-amber-800">{t("empty")}</p>
      ) : (
        <table className="w-full rounded-md border border-slate-200 bg-white" data-testid="asset-list">
          <thead className="bg-slate-50 text-left text-slate-600">
            <tr><th className="px-3 py-2">{t("form.name")}</th><th className="px-3 py-2">{t("form.assetType")}</th><th className="px-3 py-2">{t("form.hostingCountry")}</th><th className="px-3 py-2" /></tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.map((a) => (
              <tr key={a.id}>
                <td className="px-3 py-2">{a.name}{a.status === "retired" && <span className="ml-2 text-xs text-slate-500">({t("retired")})</span>}</td>
                <td className="px-3 py-2">{t(`types.${a.asset_type}`)}</td>
                <td className="px-3 py-2">{a.hosting_country_code ?? "—"}</td>
                <td className="px-3 py-2 text-right">
                  {canUpdate && <button className="text-sky-700 underline" onClick={() => { save.reset(); setSaved(false); setDraft(fromAsset(a)); }}>{t("edit")}</button>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {list.hasNextPage && <Button variant="secondary" onClick={() => list.fetchNextPage()}>{t("more")}</Button>}
    </main>
  );
}
