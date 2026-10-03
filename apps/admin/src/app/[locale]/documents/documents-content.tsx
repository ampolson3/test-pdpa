"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@pdpa/ui";
import { useDocumentMutations, useDocumentTemplates, useDocumentTypes, useDocuments, useLegalEntities, type DocType } from "@pdpa/api-client";
import { Link, useRouter } from "@/i18n/routing";
import { DocStatus, DocsNav, field, input, problemText, useClient, useWhen } from "./shared";

export function DocumentsContent() {
  const t = useTranslations("docs");
  const client = useClient();
  const types = useDocumentTypes(client);
  const [docType, setDocType] = useState<DocType | "">("");
  const [q, setQ] = useState("");
  const [search, setSearch] = useState("");
  const list = useDocuments(client, { doc_type: docType || undefined, q: search });
  const [creating, setCreating] = useState(false);
  const when = useWhen();
  const creatable = (types.data?.types ?? []).filter((x) => x.can_create);
  const rows = list.data?.pages.flatMap((p) => p.data) ?? [];

  return (
    <main className="mx-auto max-w-6xl space-y-4 p-8 text-sm">
      <header className="flex flex-wrap items-start justify-between gap-2">
        <h1 className="text-xl font-semibold">{t("list.title")}</h1>
        {creatable.length > 0 && <Button onClick={() => setCreating(true)} data-testid="new-document">{t("create.title")}</Button>}
      </header>
      <DocsNav current="documents" />
      {creating && <NewDocument types={creatable.map((x) => x.doc_type)} onCancel={() => setCreating(false)} />}
      <form className="flex flex-wrap items-end gap-2" onSubmit={(e) => { e.preventDefault(); setSearch(q); }}>
        <input className={`${field} w-64`} placeholder={t("list.search")} value={q} onChange={(e) => setQ(e.target.value)} />
        <select className={`${field} w-64`} value={docType} onChange={(e) => setDocType(e.target.value as DocType | "")} aria-label={t("list.type")}>
          <option value="">{t("list.allTypes")}</option>
          {(types.data?.types ?? []).map((x) => <option key={x.doc_type} value={x.doc_type}>{t(`types.${x.doc_type}`)}</option>)}
        </select>
      </form>
      {list.isPending ? <p className="text-slate-500">{t("loading")}</p> : list.isError ? <p className="text-red-700">{t("loadError")}</p> : rows.length === 0 ? (
        <p className="text-slate-500">{t("list.empty")}</p>
      ) : (
        <table className="w-full border-collapse overflow-hidden rounded-md border border-slate-200 bg-white">
          <thead className="bg-slate-50 text-left text-slate-600">
            <tr><th className="p-2">{t("create.name")}</th><th className="p-2">{t("list.type")}</th><th className="p-2" /><th className="p-2">{t("list.updated")}</th></tr>
          </thead>
          <tbody>
            {rows.map((d) => (
              <tr key={d.id} className="border-t border-slate-100">
                <td className="p-2"><Link className="text-sky-700 underline" href={`/documents/${d.id}`}>{d.title}</Link></td>
                <td className="p-2">{t(`types.${d.doc_type}`)}</td>
                <td className="p-2"><DocStatus status={d.status} /></td>
                <td className="p-2 text-slate-600">{when(d.updated_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {list.hasNextPage && <Button variant="secondary" onClick={() => list.fetchNextPage()}>{t("list.more")}</Button>}
    </main>
  );
}

function NewDocument({ types, onCancel }: { types: DocType[]; onCancel: () => void }) {
  const t = useTranslations("docs");
  const client = useClient();
  const router = useRouter();
  const entities = useLegalEntities(client);
  const m = useDocumentMutations(client);
  const [docType, setDocType] = useState<DocType>(types[0]);
  const templates = useDocumentTemplates(client, { doc_type: docType, published_only: true });
  const [title, setTitle] = useState("");
  const [entity, setEntity] = useState("");
  const [template, setTemplate] = useState("");

  return (
    <form
      className="grid gap-3 rounded-md border border-slate-200 bg-white p-4 md:grid-cols-2"
      data-testid="new-document-form"
      onSubmit={(e) => {
        e.preventDefault();
        m.create.mutate(
          { doc_type: docType, title, legal_entity_id: entity || undefined, template_id: template || undefined },
          { onSuccess: (d) => router.push(`/documents/${d.id}`) },
        );
      }}
    >
      <label className="space-y-1">
        <span>{t("create.docType")}</span>
        <select className={input} value={docType} onChange={(e) => { setDocType(e.target.value as DocType); setTemplate(""); }} name="doc_type">
          {types.map((x) => <option key={x} value={x}>{t(`types.${x}`)}</option>)}
        </select>
      </label>
      <label className="space-y-1">
        <span>{t("create.name")}</span>
        <input className={input} required maxLength={300} value={title} onChange={(e) => setTitle(e.target.value)} name="title" />
      </label>
      <label className="space-y-1">
        <span>{t("create.legalEntity")}</span>
        <select className={input} value={entity} onChange={(e) => setEntity(e.target.value)} name="legal_entity_id">
          <option value="">{t("create.none")}</option>
          {(entities.data ?? []).map((le) => <option key={le.id} value={le.id}>{le.name_th}</option>)}
        </select>
      </label>
      <label className="space-y-1">
        <span>{t("create.template")}</span>
        <select className={input} value={template} onChange={(e) => setTemplate(e.target.value)} name="template_id">
          <option value="">{t("create.blank")}</option>
          {(templates.data ?? []).map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}
        </select>
      </label>
      {m.create.isError && <p className="text-red-700 md:col-span-2" role="alert">{problemText(m.create.error)}</p>}
      <div className="flex gap-2 md:col-span-2">
        <Button type="submit" disabled={m.create.isPending}>{t("create.submit")}</Button>
        <Button type="button" variant="secondary" onClick={onCancel}>{t("back")}</Button>
      </div>
    </form>
  );
}
