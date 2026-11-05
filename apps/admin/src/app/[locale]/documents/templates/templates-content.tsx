"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import type { JSONContent } from "@tiptap/react";
import { Button } from "@pdpa/ui";
import { useDocumentTemplateMutations, useDocumentTemplates, type DocType, type DocumentContent, type DocumentTemplate } from "@pdpa/api-client";
import { DocEditor } from "@/components/doc-editor";
import { DocStatus, DocsNav, input, problemText, useClient, useEditorCatalog } from "../shared";

const empty: JSONContent = { type: "doc", content: [{ type: "paragraph" }] };

/** Document templates (PLT-16): the starting content of new documents; wording is legal text, published by the type's publishers. */
export function TemplatesContent() {
  const t = useTranslations("docs.templates");
  const td = useTranslations("docs");
  const { types } = useEditorCatalog();
  const client = useClient();
  const list = useDocumentTemplates(client);
  const [selected, setSelected] = useState<string | "new">();
  const writable = (types.data?.types ?? []).filter((x) => x.can_write_templates);

  return (
    <main className="mx-auto max-w-7xl space-y-4 p-8 text-sm">
      <header className="flex flex-wrap items-start justify-between gap-2">
        <h1 className="text-xl font-semibold">{t("title")}</h1>
        {writable.length > 0 && <Button onClick={() => setSelected("new")} data-testid="new-template">{t("new")}</Button>}
      </header>
      <DocsNav current="templates" />
      <p className="rounded-md bg-amber-50 p-2 text-amber-900">{t("legalNote")}</p>
      <div className="grid gap-4 lg:grid-cols-[20rem_1fr]">
        <ul className="divide-y divide-slate-100 self-start rounded-md border border-slate-200 bg-white" data-testid="template-list">
          {(list.data ?? []).length === 0 && <li className="p-3 text-slate-500">{t("empty")}</li>}
          {(list.data ?? []).map((x) => (
            <li key={x.id}>
              <button type="button" onClick={() => setSelected(x.id)} className={`w-full space-y-1 px-3 py-2 text-left ${selected === x.id ? "bg-slate-100" : ""}`}>
                <span className="block font-medium">{x.name}</span>
                <span className="flex items-center gap-2 text-xs text-slate-500">
                  {td(`types.${x.doc_type}`)} {t("version", { no: x.version })} <DocStatus status={x.status} />
                </span>
              </button>
            </li>
          ))}
        </ul>
        {selected ? (
          <TemplateForm
            key={selected === "new" ? "new" : `${selected}:${list.data?.find((x) => x.id === selected)?.row_version}`}
            template={list.data?.find((x) => x.id === selected)}
            docTypes={writable.map((x) => x.doc_type)}
            canWrite={(dt) => !!types.data?.types.find((x) => x.doc_type === dt)?.can_write_templates}
            canPublish={(dt) => !!types.data?.types.find((x) => x.doc_type === dt)?.can_publish}
            onSaved={setSelected}
          />
        ) : <p className="text-slate-500">{t("select")}</p>}
      </div>
    </main>
  );
}

function TemplateForm({ template, docTypes, canWrite, canPublish, onSaved }: {
  template?: DocumentTemplate;
  docTypes: DocType[];
  canWrite: (dt: string) => boolean;
  canPublish: (dt: string) => boolean;
  onSaved: (id: string) => void;
}) {
  const t = useTranslations("docs.templates");
  const td = useTranslations("docs");
  const client = useClient();
  const m = useDocumentTemplateMutations(client);
  const [docType, setDocType] = useState<DocType>(template?.doc_type ?? docTypes[0]);
  const { fields, clauses } = useEditorCatalog(docType);
  const [code, setCode] = useState(template?.code ?? "");
  const [name, setName] = useState(template?.name ?? "");
  const [content, setContent] = useState<DocumentContent>(template?.content ?? { th: empty as DocumentContent["th"] });
  const editable = !template || (!template.global && template.status !== "retired" && canWrite(template.doc_type));
  const err = m.create.error ?? m.update.error ?? m.publish.error;

  return (
    <form
      className="space-y-3 rounded-md border border-slate-200 bg-white p-4"
      data-testid="template-form"
      onSubmit={(e) => {
        e.preventDefault();
        if (!template) m.create.mutate({ doc_type: docType, code, name, content }, { onSuccess: (x) => onSaved(x.id) });
        else m.update.mutate({ id: template.id, rowVersion: template.row_version, name, content }, { onSuccess: (x) => onSaved(x.id) });
      }}
    >
      {template && <p className="flex items-center gap-2"><code>{template.code}</code> {t("version", { no: template.version })} <DocStatus status={template.status} /></p>}
      <div className="grid gap-3 md:grid-cols-3">
        <label className="space-y-1"><span>{t("docType")}</span>
          <select className={input} value={docType} disabled={!!template} onChange={(e) => setDocType(e.target.value as DocType)}>
            {(template ? [template.doc_type] : docTypes).map((x) => <option key={x} value={x}>{td(`types.${x}`)}</option>)}
          </select>
        </label>
        {!template && (
          <label className="space-y-1"><span>{t("code")}</span>
            <input className={input} required pattern="[a-z][a-z0-9_.\-]{1,79}" value={code} onChange={(e) => setCode(e.target.value)} name="code" />
          </label>
        )}
        <label className="space-y-1"><span>{t("name")}</span>
          <input className={input} required maxLength={300} value={name} disabled={!editable} onChange={(e) => setName(e.target.value)} name="name" />
        </label>
      </div>
      <DocEditor docKey={`${template?.id ?? "new"}:th`} value={content.th as JSONContent} onChange={(doc) => setContent((c) => ({ ...c, th: doc as DocumentContent["th"] }))}
        editable={editable} fields={fields} clauses={clauses} label={name || t("name")} />
      {err && <p className="text-red-700" role="alert">{t("error")}: {problemText(err)}</p>}
      <div className="flex gap-2">
        {editable && <Button type="submit" disabled={m.create.isPending || m.update.isPending} data-testid="save-template">{t("save")}</Button>}
        {template && template.status === "draft" && canPublish(template.doc_type) && (
          <Button type="button" onClick={() => m.publish.mutate(template)} data-testid="publish-template">{t("publish")}</Button>
        )}
      </div>
    </form>
  );
}
