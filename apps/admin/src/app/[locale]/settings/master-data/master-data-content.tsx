"use client";

import { useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { createApiClient, useMasterData, useMasterDataMutations, type MasterDataInput, type MasterDataItem, type MasterDataKind } from "@pdpa/api-client";

const KINDS: MasterDataKind[] = ["data_categories", "data_subject_types", "processing_purposes", "lawful_bases", "countries"];
const EDITABLE = new Set<MasterDataKind>(["data_categories", "data_subject_types", "processing_purposes"]);

function detail(e: unknown): string {
  return typeof e === "object" && e !== null ? [(e as { title?: string }).title, (e as { detail?: string }).detail].filter(Boolean).join(" — ") : "";
}

export function MasterDataContent() {
  const t = useTranslations("masterData");
  const canRead = usePermission("org.masterdata.read");
  const [kind, setKind] = useState<MasterDataKind>("data_categories");
  if (!canRead) return <main className="mx-auto max-w-5xl p-8 text-slate-600">{t("forbidden")}</main>;
  return (
    <main className="mx-auto max-w-6xl space-y-4 p-8 text-sm">
      <header>
        <h1 className="text-xl font-semibold">{t("title")}</h1>
        <p className="text-slate-600">{t("intro")}</p>
      </header>
      <p className="rounded-md bg-amber-50 p-3 text-amber-900" data-testid="draft-notice">{t("draftNotice")}</p>
      <nav className="flex flex-wrap gap-2" role="tablist">
        {KINDS.map((k) => (
          <button key={k} role="tab" aria-selected={k === kind} className={`rounded-md px-3 py-1.5 ${k === kind ? "bg-slate-900 text-white" : "bg-white ring-1 ring-slate-200 hover:bg-slate-50"}`} onClick={() => setKind(k)}>
            {t(`kinds.${k}`)}
          </button>
        ))}
      </nav>
      <KindTable key={kind} kind={kind} />
    </main>
  );
}

type Draft = { code: string; name_th: string; name_en: string; is_sensitive: boolean; sensitive_type: string; parent_id: string; is_vulnerable: boolean; category: string };
const empty: Draft = { code: "", name_th: "", name_en: "", is_sensitive: false, sensitive_type: "", parent_id: "", is_vulnerable: false, category: "" };

function KindTable({ kind }: { kind: MasterDataKind }) {
  const t = useTranslations("masterData");
  const locale = useLocale();
  const canCreate = usePermission("org.masterdata.create");
  const canUpdate = usePermission("org.masterdata.update");
  const canDelete = usePermission("org.masterdata.delete");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const list = useMasterData(client, kind);
  const m = useMasterDataMutations(client, kind);
  const [q, setQ] = useState("");
  const [draft, setDraft] = useState<{ item?: MasterDataItem; v: Draft } | null>(null);
  const editable = EDITABLE.has(kind);

  const name = (it: MasterDataItem) => (locale === "en" && it.name_en) || it.name_th;
  const needle = q.trim().toLowerCase();
  const rows = (list.data ?? []).filter((it) => !needle || [it.code, it.name_th, it.name_en ?? ""].some((s) => s.toLowerCase().includes(needle)));
  const byId = new Map((list.data ?? []).filter((x) => x.id).map((x) => [x.id!, x]));
  const failure = [m.create, m.update, m.remove].find((x) => x.isError)?.error;
  const input = (v: Draft): MasterDataInput => ({ code: v.code, name_th: v.name_th, name_en: v.name_en || undefined, is_sensitive: v.is_sensitive,
    sensitive_type: v.sensitive_type || undefined, parent_id: v.parent_id || undefined, is_vulnerable: v.is_vulnerable, category: v.category || undefined });
  const submit = () => {
    if (!draft) return;
    const done = { onSuccess: () => setDraft(null) };
    if (draft.item) m.update.mutate({ item: draft.item, input: input(draft.v) }, done);
    else m.create.mutate(input(draft.v), done);
  };

  const describe = (it: MasterDataItem): string => {
    const parts: string[] = [];
    if (it.is_sensitive) parts.push(t("sensitive"));
    if (it.is_vulnerable) parts.push(t("vulnerable"));
    if (it.category) parts.push(`${t("category")}: ${it.category}`);
    if (it.parent_id && byId.get(it.parent_id)) parts.push(`${t("parent")} ${name(byId.get(it.parent_id)!)}`);
    if (it.section_ref) parts.push(it.section_ref);
    if (it.for_sensitive) parts.push(t("forSensitive"));
    if (it.requires_consent) parts.push(t("needsConsent"));
    if (it.requires_lia) parts.push(t("needsLia"));
    if (it.adequacy_status) parts.push(t(`adequacy.${it.adequacy_status}`));
    return parts.join(" · ");
  };

  return (
    <section className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <input className="rounded-md border border-slate-300 px-2 py-1" placeholder={t("search")} aria-label={t("search")} value={q} onChange={(e) => setQ(e.target.value)} />
        {!editable ? <span className="text-slate-500">{t("readOnly")}</span> : canCreate && <Button variant="secondary" onClick={() => setDraft({ v: { ...empty } })}>{t("add")}</Button>}
      </div>
      {failure != null && <p className="text-red-700" role="alert">{t("actionError", { detail: detail(failure) })}</p>}
      {draft && (
        <div className="grid gap-2 rounded-md border border-slate-200 bg-white p-3 sm:grid-cols-4" data-testid="master-form">
          <label><span className="block text-slate-600">{t("code")}</span><input className="mt-1 w-full rounded border border-slate-300 px-2 py-1 font-mono" value={draft.v.code} disabled={!!draft.item} onChange={(e) => setDraft({ ...draft, v: { ...draft.v, code: e.target.value } })} /></label>
          <label><span className="block text-slate-600">{t("name")}</span><input className="mt-1 w-full rounded border border-slate-300 px-2 py-1" value={draft.v.name_th} onChange={(e) => setDraft({ ...draft, v: { ...draft.v, name_th: e.target.value } })} /></label>
          <label><span className="block text-slate-600">{t("nameEn")}</span><input className="mt-1 w-full rounded border border-slate-300 px-2 py-1" value={draft.v.name_en} onChange={(e) => setDraft({ ...draft, v: { ...draft.v, name_en: e.target.value } })} /></label>
          <div className="space-y-1">
            {kind === "data_categories" && (
              <>
                <label className="flex items-center gap-1"><input type="checkbox" checked={draft.v.is_sensitive} onChange={(e) => setDraft({ ...draft, v: { ...draft.v, is_sensitive: e.target.checked } })} />{t("sensitive")}</label>
                <label><span className="block text-slate-600">{t("parent")}</span>
                  <select className="w-full rounded border border-slate-300 bg-white px-2 py-1" value={draft.v.parent_id} onChange={(e) => setDraft({ ...draft, v: { ...draft.v, parent_id: e.target.value } })}>
                    <option value="">{t("none")}</option>
                    {(list.data ?? []).filter((x) => x.id && x.id !== draft.item?.id).map((x) => <option key={x.id} value={x.id}>{name(x)}</option>)}
                  </select>
                </label>
              </>
            )}
            {kind === "data_subject_types" && <label className="flex items-center gap-1"><input type="checkbox" checked={draft.v.is_vulnerable} onChange={(e) => setDraft({ ...draft, v: { ...draft.v, is_vulnerable: e.target.checked } })} />{t("vulnerable")}</label>}
            {kind === "processing_purposes" && <label><span className="block text-slate-600">{t("category")}</span><input className="w-full rounded border border-slate-300 px-2 py-1" value={draft.v.category} onChange={(e) => setDraft({ ...draft, v: { ...draft.v, category: e.target.value } })} /></label>}
          </div>
          <div className="flex gap-2 sm:col-span-4">
            <Button onClick={submit} disabled={!draft.v.code.trim() || !draft.v.name_th.trim() || m.create.isPending || m.update.isPending}>{t("save")}</Button>
            <Button variant="secondary" onClick={() => setDraft(null)}>{t("cancel")}</Button>
          </div>
        </div>
      )}
      {list.isPending ? <p className="text-slate-500">{t("loading")}</p> : list.isError ? <p className="text-red-700">{t("loadError")}</p> : (
        <table className="w-full rounded-md border border-slate-200 bg-white">
          <thead className="bg-slate-50 text-left text-slate-600"><tr><th className="px-3 py-2">{t("code")}</th><th className="px-3 py-2">{t("name")}</th><th className="px-3 py-2">{t("details")}</th><th className="px-3 py-2" /></tr></thead>
          <tbody className="divide-y divide-slate-100">
            {rows.map((it) => (
              <tr key={it.id ?? it.code} data-testid={`md-${it.code}`}>
                <td className="px-3 py-2 font-mono text-xs">{it.code}</td>
                <td className="px-3 py-2">{name(it)}</td>
                <td className="px-3 py-2 text-xs text-slate-600">{describe(it)}</td>
                <td className="whitespace-nowrap px-3 py-2 text-right text-xs">
                  {it.global ? <span className="rounded bg-slate-100 px-2 py-0.5 text-slate-600">{t("default")}</span> : (
                    <span className="flex justify-end gap-2">
                      <span className="rounded bg-emerald-50 px-2 py-0.5 text-emerald-700">{t("own")}</span>
                      {canUpdate && <button className="underline" onClick={() => setDraft({ item: it, v: { code: it.code, name_th: it.name_th, name_en: it.name_en ?? "", is_sensitive: !!it.is_sensitive, sensitive_type: it.sensitive_type ?? "", parent_id: it.parent_id ?? "", is_vulnerable: !!it.is_vulnerable, category: it.category ?? "" } })}>{t("edit")}</button>}
                      {canDelete && <button className="text-red-700 underline" onClick={() => m.remove.mutate(it)}>{t("remove")}</button>}
                    </span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
