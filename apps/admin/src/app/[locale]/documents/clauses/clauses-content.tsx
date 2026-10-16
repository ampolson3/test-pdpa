"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import type { JSONContent } from "@tiptap/react";
import { usePermission } from "@pdpa/authz";
import { Button } from "@pdpa/ui";
import { useClause, useClauseMutations, useClauses, type DocType, type DocumentClause, type DocumentClauseInput } from "@pdpa/api-client";
import { DocEditor } from "@/components/doc-editor";
import { DocStatus, DocsNav, input, problemText, useClient, useEditorCatalog } from "../shared";

const APPLIES: DocType[] = ["notice", "policy", "dpa", "dsa", "dsar_letter", "pdpc_form", "breach_letter"];
const empty: JSONContent = { type: "doc", content: [{ type: "paragraph" }] };

/** The clause library (DSA-06 / PLT-16): the latest version of each clause; editing a published one starts the next version. */
export function ClausesContent() {
  const t = useTranslations("docs.clauses");
  const client = useClient();
  const canCreate = usePermission("agreement.clause.create");
  const list = useClauses(client);
  const [selected, setSelected] = useState<string | "new">();

  return (
    <main className="mx-auto max-w-7xl space-y-4 p-8 text-sm">
      <header className="flex flex-wrap items-start justify-between gap-2">
        <h1 className="text-xl font-semibold">{t("title")}</h1>
        {canCreate && <Button onClick={() => setSelected("new")} data-testid="new-clause">{t("new")}</Button>}
      </header>
      <DocsNav current="clauses" />
      <div className="grid gap-4 lg:grid-cols-[20rem_1fr]">
        <ul className="divide-y divide-slate-100 self-start rounded-md border border-slate-200 bg-white" data-testid="clause-list">
          {(list.data ?? []).length === 0 && <li className="p-3 text-slate-500">{t("empty")}</li>}
          {(list.data ?? []).map((c) => (
            <li key={c.id}>
              <button type="button" onClick={() => setSelected(c.id)} className={`w-full space-y-1 px-3 py-2 text-left ${selected === c.id ? "bg-slate-100" : ""}`}>
                <span className="block font-medium">{c.body.th.title}</span>
                <span className="flex items-center gap-2 text-xs text-slate-500">
                  <code>{c.code}</code> {t("version", { no: c.version })} <DocStatus status={c.status} /> {c.global && <span>{t("global")}</span>}
                </span>
              </button>
            </li>
          ))}
        </ul>
        {selected ? <ClauseEditor key={selected} id={selected === "new" ? undefined : selected} onSaved={setSelected} /> : <p className="text-slate-500">{t("select")}</p>}
      </div>
    </main>
  );
}

function ClauseEditor({ id, onSaved }: { id?: string; onSaved: (id: string) => void }) {
  const t = useTranslations("docs.clauses");
  const td = useTranslations("docs");
  const client = useClient();
  const detail = useClause(client, id);
  const m = useClauseMutations(client);
  const canUpdate = usePermission("agreement.clause.update");
  const canPublish = usePermission("agreement.clause.publish");
  const { fields } = useEditorCatalog();
  if (id && !detail.data) return <p className="text-slate-500">{td("loading")}</p>;
  return <ClauseForm key={detail.data ? `${detail.data.id}:${detail.data.row_version}` : "new"} clause={detail.data} versions={detail.data?.versions ?? []}
    fields={fields} canSave={!detail.data ? true : canUpdate && !detail.data.global && detail.data.status !== "retired"} canPublish={canPublish && !detail.data?.global}
    m={m} onSaved={onSaved} t={t} />;
}

function ClauseForm({ clause, versions, fields, canSave, canPublish, m, onSaved, t }: {
  clause?: DocumentClause;
  versions: DocumentClause[];
  fields: { key: string; label: string }[];
  canSave: boolean;
  canPublish: boolean;
  m: ReturnType<typeof useClauseMutations>;
  onSaved: (id: string) => void;
  t: ReturnType<typeof useTranslations<"docs.clauses">>;
}) {
  const td = useTranslations("docs");
  const [form, setForm] = useState<DocumentClauseInput>(() => ({
    code: clause?.code ?? "", category: clause?.category ?? "", legal_ref: clause?.legal_ref ?? "", applies_to: (clause?.applies_to as DocType[]) ?? ["dpa", "dsa"],
    is_mandatory: clause?.is_mandatory ?? false,
    body: { th: clause?.body.th ?? { title: "", doc: empty }, ...(clause?.body.en ? { en: clause.body.en } : {}) },
  }));
  const [hasEn, setHasEn] = useState(!!clause?.body.en);
  const set = (patch: Partial<DocumentClauseInput>) => setForm((f) => ({ ...f, ...patch }));
  const setBody = (l: "th" | "en", patch: Partial<{ title: string; doc: JSONContent }>) =>
    setForm((f) => ({ ...f, body: { ...f.body, [l]: { ...(f.body[l] ?? { title: "", doc: empty }), ...patch } } }));
  const err = m.create.error ?? m.update.error ?? m.publish.error ?? m.retire.error;
  const submit = () => {
    const body: DocumentClauseInput = { ...form, legal_ref: form.legal_ref || undefined, body: hasEn ? form.body : { th: form.body.th } };
    if (!clause) m.create.mutate(body, { onSuccess: (c) => onSaved(c.id) });
    else m.update.mutate({ id: clause.id, rowVersion: clause.row_version, body }, { onSuccess: (c) => onSaved(c.id) });
  };

  return (
    <form className="space-y-3 rounded-md border border-slate-200 bg-white p-4" data-testid="clause-form" onSubmit={(e) => { e.preventDefault(); submit(); }}>
      {clause && (
        <p className="flex items-center gap-2">
          <code>{clause.code}</code> {t("version", { no: clause.version })} <DocStatus status={clause.status} />
          {clause.status === "published" && canSave && <span className="text-xs text-slate-500">{t("newVersion")}</span>}
        </p>
      )}
      <div className="grid gap-3 md:grid-cols-2">
        {!clause && (
          <label className="space-y-1"><span>{t("code")}</span>
            <input className={input} required pattern="[a-z][a-z0-9_.\-]{1,79}" value={form.code ?? ""} onChange={(e) => set({ code: e.target.value })} name="code" />
          </label>
        )}
        <label className="space-y-1"><span>{t("category")}</span>
          <input className={input} required maxLength={60} value={form.category} disabled={!canSave} onChange={(e) => set({ category: e.target.value })} name="category" />
        </label>
        <label className="space-y-1"><span>{t("legalRef")}</span>
          <input className={input} maxLength={500} value={form.legal_ref ?? ""} disabled={!canSave} onChange={(e) => set({ legal_ref: e.target.value })} />
        </label>
        <label className="flex items-center gap-2"><input type="checkbox" checked={!!form.is_mandatory} disabled={!canSave} onChange={(e) => set({ is_mandatory: e.target.checked })} /> {t("mandatory")}</label>
      </div>
      <fieldset className="space-y-1">
        <legend>{t("appliesTo")}</legend>
        <div className="flex flex-wrap gap-3">
          {APPLIES.map((a) => (
            <label key={a} className="flex items-center gap-1">
              <input type="checkbox" disabled={!canSave} checked={form.applies_to?.includes(a) ?? false}
                onChange={(e) => set({ applies_to: e.target.checked ? [...(form.applies_to ?? []), a] : (form.applies_to ?? []).filter((x) => x !== a) })} />
              {td(`types.${a}`)}
            </label>
          ))}
        </div>
      </fieldset>
      {(["th", "en"] as const).map((l) =>
        l === "th" || hasEn ? (
          <div key={l} className="space-y-1">
            <label className="block space-y-1"><span>{t(l === "th" ? "titleTh" : "titleEn")}</span>
              <input className={input} required maxLength={300} value={form.body[l]?.title ?? ""} disabled={!canSave} onChange={(e) => setBody(l, { title: e.target.value })} name={`title_${l}`} />
            </label>
            <span>{t(l === "th" ? "bodyTh" : "bodyEn")}</span>
            <DocEditor docKey={`${clause?.id ?? "new"}:${l}`} value={form.body[l]?.doc as JSONContent} onChange={(doc) => setBody(l, { doc })} editable={canSave}
              fields={fields} clauses={[]} allowClauses={false} label={t(l === "th" ? "bodyTh" : "bodyEn")} />
          </div>
        ) : null,
      )}
      {canSave && !hasEn && <Button type="button" variant="secondary" onClick={() => setHasEn(true)}>{td("editor.addEnglish")}</Button>}
      {err && <p className="text-red-700" role="alert">{t("error")}: {problemText(err)}</p>}
      <div className="flex flex-wrap gap-2">
        {canSave && <Button type="submit" disabled={m.create.isPending || m.update.isPending} data-testid="save-clause">{t("save")}</Button>}
        {clause && canPublish && clause.status === "draft" && <Button type="button" onClick={() => m.publish.mutate(clause)} data-testid="publish-clause">{t("publish")}</Button>}
        {clause && canPublish && clause.status === "published" && <Button type="button" variant="secondary" onClick={() => m.retire.mutate(clause)}>{t("retire")}</Button>}
      </div>
      {versions.length > 1 && (
        <section>
          <h3 className="font-medium">{t("history")}</h3>
          <ul className="flex flex-wrap gap-2 text-xs">
            {versions.map((v) => <li key={v.id} className="flex items-center gap-1 rounded bg-slate-50 px-2 py-1">{t("version", { no: v.version })} <DocStatus status={v.status} /></li>)}
          </ul>
        </section>
      )}
    </form>
  );
}
