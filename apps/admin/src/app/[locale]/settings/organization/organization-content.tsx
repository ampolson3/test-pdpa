"use client";

import { useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { createApiClient, useLegalEntities, useOrgUnitMutations, useOrgUnits, useSaveLegalEntity, type LegalEntity, type OrgUnit, type OrgUnitType } from "@pdpa/api-client";
import { FileUploader } from "@/components/file-uploader";

const INPUT = "mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1";
const TYPES: OrgUnitType[] = ["group", "company", "division", "department", "branch", "team"];

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

type Draft = {
  id?: string; rowVersion?: number; name_th: string; name_en: string; registration_no: string; tax_id: string; parent_id: string;
  line1: string; subdistrict: string; district: string; province: string; postal_code: string; contact_email: string; contact_phone: string;
  is_controller: boolean; is_processor: boolean; status: "active" | "inactive"; logo_file_id?: string;
};

const blank: Draft = { name_th: "", name_en: "", registration_no: "", tax_id: "", parent_id: "", line1: "", subdistrict: "", district: "", province: "", postal_code: "",
  contact_email: "", contact_phone: "", is_controller: true, is_processor: false, status: "active" };

function fromEntity(e: LegalEntity): Draft {
  const a = e.address;
  return { id: e.id, rowVersion: e.row_version, name_th: e.name_th, name_en: e.name_en ?? "", registration_no: e.registration_no ?? "", tax_id: e.tax_id ?? "",
    parent_id: e.parent_id ?? "", line1: a.line1 ?? "", subdistrict: a.subdistrict ?? "", district: a.district ?? "", province: a.province ?? "", postal_code: a.postal_code ?? "",
    contact_email: e.contact_email ?? "", contact_phone: e.contact_phone ?? "", is_controller: e.is_controller, is_processor: e.is_processor, status: e.status, logo_file_id: e.logo_file_id };
}

export function OrganizationContent() {
  const t = useTranslations("organization");
  const canRead = usePermission("org.structure.read");
  const canCreate = usePermission("org.structure.create");
  const canUpdate = usePermission("org.structure.update");
  const canClose = usePermission("org.structure.delete");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const entities = useLegalEntities(client);
  const save = useSaveLegalEntity(client);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [selected, setSelected] = useState<string>();
  const [saved, setSaved] = useState(false);

  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;
  const current = entities.data?.find((e) => e.id === selected) ?? entities.data?.[0];
  const form = draft ?? (current ? fromEntity(current) : null);
  const set = (p: Partial<Draft>) => { setSaved(false); setDraft({ ...(form ?? blank), ...p }); };
  const editable = form && (form.id ? canUpdate : canCreate);

  const submit = () => {
    if (!form) return;
    save.mutate({
      id: form.id, rowVersion: form.rowVersion,
      input: { name_th: form.name_th, name_en: form.name_en || undefined, registration_no: form.registration_no || undefined, tax_id: form.tax_id || undefined,
        parent_id: form.parent_id || undefined, contact_email: form.contact_email || undefined, contact_phone: form.contact_phone || undefined,
        address: { line1: form.line1 || undefined, subdistrict: form.subdistrict || undefined, district: form.district || undefined, province: form.province || undefined, postal_code: form.postal_code || undefined },
        is_controller: form.is_controller, is_processor: form.is_processor, status: form.status, logo_file_id: form.logo_file_id },
    }, { onSuccess: (e) => { setSelected(e.id); setDraft(null); setSaved(true); } });
  };
  const field = (key: keyof Draft, label: string, extra?: string) => (
    <label className={extra}><span className="block text-slate-600">{label}</span>
      <input className={INPUT} value={String(form?.[key] ?? "")} onChange={(e) => set({ [key]: e.target.value } as Partial<Draft>)} /></label>
  );

  return (
    <main className="mx-auto grid max-w-7xl gap-6 p-8 text-sm lg:grid-cols-[1fr_3fr]">
      <section className="space-y-3">
        <header className="flex items-center justify-between">
          <h1 className="text-xl font-semibold">{t("title")}</h1>
          {canCreate && <Button variant="secondary" onClick={() => { save.reset(); setSaved(false); setDraft({ ...blank }); }}>{t("newEntity")}</Button>}
        </header>
        <h2 className="font-medium text-slate-600">{t("entities")}</h2>
        {entities.isPending ? <p className="text-slate-500">{t("loading")}</p> : entities.isError ? <p className="text-red-700">{t("loadError")}</p> : entities.data.length === 0 ? (
          <p className="rounded-md bg-amber-50 p-3 text-amber-800">{t("empty")}</p>
        ) : (
          <ul className="divide-y divide-slate-100 rounded-md border border-slate-200 bg-white">
            {entities.data.map((e) => (
              <li key={e.id}>
                <button className={`w-full px-3 py-2 text-left hover:bg-slate-50 ${e.id === current?.id && !draft ? "bg-slate-50 font-medium" : ""}`} onClick={() => { save.reset(); setSaved(false); setSelected(e.id); setDraft(null); }}>
                  {e.name_th}{e.status === "inactive" && <span className="ml-2 text-xs text-slate-500">({t("inactive")})</span>}
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="space-y-6">
        {form && (
          <fieldset className="grid gap-3 rounded-md border border-slate-200 bg-white p-4 sm:grid-cols-2" disabled={!editable}>
            <legend className="px-1 font-semibold">{t("form.details")}</legend>
            {field("name_th", t("form.nameTh"))}
            {field("name_en", t("form.nameEn"))}
            {field("registration_no", t("form.registrationNo"))}
            {field("tax_id", t("form.taxId"))}
            {field("line1", t("form.line1"), "sm:col-span-2")}
            {field("subdistrict", t("form.subdistrict"))}
            {field("district", t("form.district"))}
            {field("province", t("form.province"))}
            {field("postal_code", t("form.postalCode"))}
            {field("contact_email", t("form.email"))}
            {field("contact_phone", t("form.phone"))}
            <label><span className="block text-slate-600">{t("form.parent")}</span>
              <select className={INPUT} value={form.parent_id} onChange={(e) => set({ parent_id: e.target.value })}>
                <option value="">{t("form.none")}</option>
                {entities.data?.filter((x) => x.id !== form.id).map((x) => <option key={x.id} value={x.id}>{x.name_th}</option>)}
              </select>
            </label>
            <div className="flex flex-wrap items-end gap-4">
              <label className="flex items-center gap-1"><input type="checkbox" checked={form.is_controller} onChange={(e) => set({ is_controller: e.target.checked })} />{t("form.controller")}</label>
              <label className="flex items-center gap-1"><input type="checkbox" checked={form.is_processor} onChange={(e) => set({ is_processor: e.target.checked })} />{t("form.processor")}</label>
              <label className="flex items-center gap-1"><input type="checkbox" checked={form.status === "active"} onChange={(e) => set({ status: e.target.checked ? "active" : "inactive" })} />{t("form.active")}</label>
            </div>
            <div className="sm:col-span-2">
              <span className="block text-slate-600">{t("form.logo")}{form.logo_file_id && <span className="ml-2 text-emerald-700">· {t("form.logoSet")}</span>}</span>
              {editable && <FileUploader onUploaded={(f) => set({ logo_file_id: f.id })} />}
            </div>
            {save.isError && <p className="text-red-700 sm:col-span-2" role="alert">{t("form.saveError", { detail: detail(save.error) })}</p>}
            {saved && <p className="text-emerald-800 sm:col-span-2" role="status">{t("form.saved")}</p>}
            {editable && (
              <div className="flex gap-2 sm:col-span-2">
                <Button onClick={submit} disabled={save.isPending || !form.name_th.trim()}>{t("form.save")}</Button>
                {draft && <Button variant="secondary" onClick={() => { save.reset(); setDraft(null); }}>{t("form.cancel")}</Button>}
              </div>
            )}
          </fieldset>
        )}
        {current && !draft && <OrgTree key={current.id} entity={current} canCreate={canCreate} canUpdate={canUpdate} canClose={canClose} />}
      </section>
    </main>
  );
}

function OrgTree({ entity, canCreate, canUpdate, canClose }: { entity: LegalEntity; canCreate: boolean; canUpdate: boolean; canClose: boolean }) {
  const t = useTranslations("organization.tree");
  const locale = useLocale();
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const [showClosed, setShowClosed] = useState(false);
  const units = useOrgUnits(client, entity.id, showClosed);
  const m = useOrgUnitMutations(client);
  const [q, setQ] = useState("");
  const [adding, setAdding] = useState<string | "root">();
  const [editing, setEditing] = useState<string>();
  const [dragged, setDragged] = useState<OrgUnit>();

  const list = units.data ?? [];
  const byId = new Map(list.map((u) => [u.id, u]));
  const name = (u: OrgUnit) => (locale === "en" && u.name_en) || u.name_th;
  // Search keeps matches and their ancestors, so the tree stays readable.
  const visible = new Set<string>();
  const needle = q.trim().toLowerCase();
  for (const u of list) {
    if (!needle || u.name_th.toLowerCase().includes(needle) || (u.name_en ?? "").toLowerCase().includes(needle) || u.code.toLowerCase().includes(needle)) {
      for (let cur: OrgUnit | undefined = u; cur; cur = cur.parent_id ? byId.get(cur.parent_id) : undefined) visible.add(cur.id);
    }
  }
  const failure = [m.create, m.update, m.move, m.close].find((x) => x.isError)?.error;
  const drop = (parentId: string | null) => {
    if (dragged && dragged.id !== parentId && dragged.parent_id !== (parentId ?? undefined)) m.move.mutate({ unit: dragged, parentId });
    setDragged(undefined);
  };

  return (
    <div className="space-y-3" data-testid="org-tree">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-lg font-semibold">{t("title", { name: entity.name_th })}</h2>
        <div className="flex items-center gap-3">
          <input className="rounded-md border border-slate-300 px-2 py-1" placeholder={t("search")} aria-label={t("search")} value={q} onChange={(e) => setQ(e.target.value)} />
          <label className="flex items-center gap-1"><input type="checkbox" checked={showClosed} onChange={(e) => setShowClosed(e.target.checked)} />{t("showClosed")}</label>
        </div>
      </header>
      {canUpdate && <p className="text-xs text-slate-500">{t("dragHint")}</p>}
      {failure != null && <p className="text-red-700" role="alert">{t("actionError", { detail: detail(failure) })}</p>}
      {units.isPending ? null : list.length === 0 ? <p className="text-slate-500">{t("empty")}</p> : (
        <ul className="rounded-md border border-slate-200 bg-white">
          {list.filter((u) => visible.has(u.id)).map((u) => (
            <li key={u.id} data-testid={`unit-${u.code}`} className={`border-b border-slate-100 ${dragged && dragged.id !== u.id ? "hover:bg-sky-50" : ""}`}
              draggable={canUpdate && u.status === "active"} onDragStart={() => setDragged(u)} onDragEnd={() => setDragged(undefined)}
              onDragOver={(e) => { if (dragged && u.status === "active") e.preventDefault(); }} onDrop={(e) => { e.preventDefault(); drop(u.id); }}>
              <div className="flex flex-wrap items-center justify-between gap-2 py-2 pr-3" style={{ paddingLeft: `${u.depth * 1.25}rem` }}>
                <span className={u.status === "closed" ? "text-slate-400 line-through" : ""}>
                  <span className="mr-2 font-mono text-xs text-slate-500">{u.code}</span>{name(u)}
                  <span className="ml-2 text-xs text-slate-500">{t(`types.${u.unit_type}`)}{u.status === "closed" ? ` · ${t("closed")}` : ""}</span>
                </span>
                {u.status === "active" && (
                  <span className="flex gap-2 text-xs">
                    {canCreate && <button className="underline" onClick={() => setAdding(u.id)}>{t("addChild")}</button>}
                    {canUpdate && <button className="underline" onClick={() => setEditing(u.id)}>{t("rename")}</button>}
                    {canUpdate && (
                      <select aria-label={`${t("moveTo")} ${u.code}`} className="rounded border border-slate-300 bg-white" value="" onChange={(e) => e.target.value && m.move.mutate({ unit: u, parentId: e.target.value === "root" ? null : e.target.value })}>
                        <option value="">{t("moveTo")}</option>
                        {u.parent_id && <option value="root">{t("root")}</option>}
                        {list.filter((p) => p.status === "active" && p.id !== u.id && p.id !== u.parent_id && !isBelow(p, u, byId)).map((p) => <option key={p.id} value={p.id}>{p.code} {name(p)}</option>)}
                      </select>
                    )}
                    {canClose && <button className="text-red-700 underline" onClick={() => m.close.mutate(u)}>{t("close")}</button>}
                  </span>
                )}
              </div>
              {adding === u.id && <UnitForm onCancel={() => setAdding(undefined)} onSubmit={(v) => m.create.mutate({ legalEntityId: entity.id, parentId: u.id, ...v }, { onSuccess: () => setAdding(undefined) })} indent={u.depth + 1} />}
              {editing === u.id && <UnitForm initial={u} submitLabel={t("save")} onCancel={() => setEditing(undefined)} onSubmit={(v) => m.update.mutate({ unit: u, ...v }, { onSuccess: () => setEditing(undefined) })} indent={u.depth} />}
            </li>
          ))}
        </ul>
      )}
      {dragged?.parent_id && (
        <div className="rounded-md border-2 border-dashed border-sky-300 p-3 text-center text-sky-800" data-testid="drop-root" onDragOver={(e) => e.preventDefault()} onDrop={(e) => { e.preventDefault(); drop(null); }}>{t("dropRoot")}</div>
      )}
      {canCreate && (adding === "root" ? (
        <UnitForm onCancel={() => setAdding(undefined)} onSubmit={(v) => m.create.mutate({ legalEntityId: entity.id, ...v }, { onSuccess: () => setAdding(undefined) })} indent={0} />
      ) : (
        <Button variant="secondary" onClick={() => setAdding("root")}>{t("addRoot")}</Button>
      ))}
    </div>
  );
}

function isBelow(candidate: OrgUnit, unit: OrgUnit, byId: Map<string, OrgUnit>): boolean {
  for (let cur: OrgUnit | undefined = candidate; cur; cur = cur.parent_id ? byId.get(cur.parent_id) : undefined) if (cur.id === unit.id) return true;
  return false;
}

function UnitForm({ initial, submitLabel, onSubmit, onCancel, indent }: { initial?: OrgUnit; submitLabel?: string; onSubmit: (v: { code: string; nameTh: string; nameEn?: string; unitType: OrgUnitType }) => void; onCancel: () => void; indent: number }) {
  const t = useTranslations("organization.tree");
  const [v, setV] = useState({ code: initial?.code ?? "", nameTh: initial?.name_th ?? "", nameEn: initial?.name_en ?? "", unitType: (initial?.unit_type ?? "department") as OrgUnitType });
  return (
    <div className="flex flex-wrap items-end gap-2 bg-slate-50 py-2 pr-3" style={{ paddingLeft: `${indent * 1.25 + 0.5}rem` }} data-testid="unit-form">
      <label><span className="block text-xs text-slate-600">{t("code")}</span><input className="w-24 rounded border border-slate-300 px-2 py-1" value={v.code} onChange={(e) => setV({ ...v, code: e.target.value })} /></label>
      <label><span className="block text-xs text-slate-600">{t("name")}</span><input className="rounded border border-slate-300 px-2 py-1" value={v.nameTh} onChange={(e) => setV({ ...v, nameTh: e.target.value })} /></label>
      <label><span className="block text-xs text-slate-600">{t("nameEn")}</span><input className="rounded border border-slate-300 px-2 py-1" value={v.nameEn} onChange={(e) => setV({ ...v, nameEn: e.target.value })} /></label>
      <label><span className="block text-xs text-slate-600">{t("type")}</span>
        <select className="rounded border border-slate-300 bg-white px-2 py-1" value={v.unitType} onChange={(e) => setV({ ...v, unitType: e.target.value as OrgUnitType })}>
          {TYPES.map((x) => <option key={x} value={x}>{t(`types.${x}`)}</option>)}
        </select>
      </label>
      <Button onClick={() => onSubmit(v)} disabled={!v.code.trim() || !v.nameTh.trim()}>{submitLabel ?? t("add")}</Button>
      <Button variant="ghost" onClick={onCancel}>×</Button>
    </div>
  );
}
