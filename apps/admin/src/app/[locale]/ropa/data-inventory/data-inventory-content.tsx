"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useDataInventory,
  useSaveDataInventoryItem,
  useAssets,
  useMasterData,
  useLegalEntities,
  useOrgUnits,
  type DataInventoryItem,
  type DataInventorySource,
} from "@pdpa/api-client";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";
const SOURCES: DataInventorySource[] = ["direct", "indirect", "derived"];

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

type Draft = {
  id?: string; rowVersion?: number; asset_id: string; data_category_id: string; org_unit_id: string;
  source: DataInventorySource | ""; location_detail: string;
};

const blank: Draft = { asset_id: "", data_category_id: "", org_unit_id: "", source: "", location_detail: "" };

function fromItem(it: DataInventoryItem): Draft {
  return { id: it.id, rowVersion: it.row_version, asset_id: it.asset_id, data_category_id: it.data_category_id,
    org_unit_id: it.org_unit_id ?? "", source: it.source ?? "", location_detail: it.location_detail ?? "" };
}

export function DataInventoryContent() {
  const t = useTranslations("dataInventory");
  const canRead = usePermission("ropa.inventory.read");
  const canCreate = usePermission("ropa.inventory.create");
  const canUpdate = usePermission("ropa.inventory.update");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const [sensitiveOnly, setSensitiveOnly] = useState(false);
  const [orgUnitFilter, setOrgUnitFilter] = useState("");
  const list = useDataInventory(client, { sensitive_only: sensitiveOnly || undefined, org_unit_id: orgUnitFilter || undefined });
  const save = useSaveDataInventoryItem(client);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [saved, setSaved] = useState(false);
  const [legalEntityId, setLegalEntityId] = useState<string>();

  const assets = useAssets(client);
  const categories = useMasterData(client, "data_categories");
  const entities = useLegalEntities(client);
  const units = useOrgUnits(client, legalEntityId);

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;

  const rows = list.data?.pages.flatMap((p) => p.data) ?? [];
  const set = (p: Partial<Draft>) => { setSaved(false); setDraft({ ...(draft ?? blank), ...p }); };
  const editable = draft && (draft.id ? canUpdate : canCreate);

  const submit = () => {
    if (!draft) return;
    save.mutate({
      id: draft.id, rowVersion: draft.rowVersion,
      input: {
        asset_id: draft.asset_id, data_category_id: draft.data_category_id, org_unit_id: draft.org_unit_id || undefined,
        source: draft.source || undefined, location_detail: draft.location_detail || undefined,
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
        {canCreate && <Button onClick={() => { save.reset(); setSaved(false); setDraft({ ...blank }); }}>{t("newItem")}</Button>}
      </header>

      <div className="flex flex-wrap items-end gap-3">
        <label className="flex items-center gap-1"><input type="checkbox" checked={sensitiveOnly} onChange={(e) => setSensitiveOnly(e.target.checked)} data-testid="sensitive-only" />{t("sensitiveOnly")}</label>
        <label><span className="block text-slate-600">{t("form.legalEntity")}</span>
          <select className={INPUT} value={legalEntityId ?? ""} onChange={(e) => { setLegalEntityId(e.target.value || undefined); setOrgUnitFilter(""); }}>
            <option value="">{t("form.none")}</option>
            {entities.data?.map((le) => <option key={le.id} value={le.id}>{le.name_th}</option>)}
          </select>
        </label>
        <label><span className="block text-slate-600">{t("filterUnit")}</span>
          <select className={INPUT} value={orgUnitFilter} onChange={(e) => setOrgUnitFilter(e.target.value)} disabled={!legalEntityId}>
            <option value="">{t("allUnits")}</option>
            {units.data?.map((u) => <option key={u.id} value={u.id}>{u.name_th}</option>)}
          </select>
        </label>
      </div>

      {draft && (
        <fieldset className="grid gap-3 rounded-md border border-slate-200 bg-white p-4 sm:grid-cols-2" disabled={!editable}>
          <legend className="px-1 font-semibold">{t("form.details")}</legend>
          <label><span className="block text-slate-600">{t("form.asset")}</span>
            <select className={INPUT} value={draft.asset_id} onChange={(e) => set({ asset_id: e.target.value })}>
              <option value="">{t("form.choose")}</option>
              {assets.data?.pages.flatMap((p) => p.data).map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
            </select>
          </label>
          <label><span className="block text-slate-600">{t("form.dataCategory")}</span>
            <select className={INPUT} value={draft.data_category_id} onChange={(e) => set({ data_category_id: e.target.value })}>
              <option value="">{t("form.choose")}</option>
              {categories.data?.map((c) => <option key={c.id} value={c.id}>{c.name_th}{c.is_sensitive ? ` ⚠ ${t("form.sensitiveTag")}` : ""}</option>)}
            </select>
          </label>
          <label><span className="block text-slate-600">{t("form.orgUnit")}</span>
            <select className={INPUT} value={draft.org_unit_id} onChange={(e) => set({ org_unit_id: e.target.value })} disabled={!legalEntityId}>
              <option value="">{t("form.none")}</option>
              {units.data?.map((u) => <option key={u.id} value={u.id}>{u.name_th}</option>)}
            </select>
          </label>
          <label><span className="block text-slate-600">{t("form.source")}</span>
            <select className={INPUT} value={draft.source} onChange={(e) => set({ source: e.target.value as DataInventorySource })}>
              <option value="">{t("form.none")}</option>
              {SOURCES.map((x) => <option key={x} value={x}>{t(`sources.${x}`)}</option>)}
            </select>
          </label>
          <label className="sm:col-span-2"><span className="block text-slate-600">{t("form.locationDetail")}</span>
            <input className={INPUT} value={draft.location_detail} onChange={(e) => set({ location_detail: e.target.value })} /></label>
          {save.isError && <p className="text-red-700 sm:col-span-2" role="alert">{t("form.saveError", { detail: detail(save.error) })}</p>}
          {saved && <p className="text-emerald-800 sm:col-span-2" role="status">{t("form.saved")}</p>}
          {editable && (
            <div className="flex gap-2 sm:col-span-2">
              <Button onClick={submit} disabled={save.isPending || !draft.asset_id || !draft.data_category_id}>{t("form.save")}</Button>
              <Button variant="secondary" onClick={() => { save.reset(); setDraft(null); }}>{t("form.cancel")}</Button>
            </div>
          )}
        </fieldset>
      )}

      {list.isPending ? <p className="text-slate-500">{t("loading")}</p> : list.isError ? <p className="text-red-700">{t("loadError")}</p> : rows.length === 0 ? (
        <p className="rounded-md bg-amber-50 p-3 text-amber-800">{t("empty")}</p>
      ) : (
        <table className="w-full rounded-md border border-slate-200 bg-white" data-testid="inventory-list">
          <thead className="bg-slate-50 text-left text-slate-600">
            <tr><th className="px-3 py-2">{t("form.dataCategory")}</th><th className="px-3 py-2">{t("form.source")}</th><th className="px-3 py-2" /></tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {rows.map((it) => (
              <tr key={it.id}>
                <td className="px-3 py-2">
                  {it.category_name_th}
                  {it.is_sensitive && <span className="ml-2 rounded bg-amber-100 px-2 py-0.5 text-xs text-amber-800" data-testid="sensitive-badge">{t("sensitiveTag")}</span>}
                </td>
                <td className="px-3 py-2">{it.source ? t(`sources.${it.source}`) : "—"}</td>
                <td className="px-3 py-2 text-right">
                  {canUpdate && <button className="text-sky-700 underline" onClick={() => { save.reset(); setSaved(false); setDraft(fromItem(it)); }}>{t("edit")}</button>}
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
