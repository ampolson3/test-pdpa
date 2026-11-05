"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import type { Editor, JSONContent } from "@tiptap/react";
import { Button } from "@pdpa/ui";
import {
  documentExportHref,
  fileDownloadHref,
  useComposedDocument,
  useDocumentComparison,
  useDocumentMutations,
  useLegalEntities,
  usePublishedDocumentVersions,
  useRecordVersions,
  type ComposedDocument,
  type DocumentDraft,
} from "@pdpa/api-client";
import { Link } from "@/i18n/routing";
import { DocEditor, outline, type Heading } from "@/components/doc-editor";
import { RecordVersions } from "@/components/record-versions";
import { BFF, ComparisonView, DocStatus, field, input, problemText, useClient, useEditorCatalog, useWhen } from "../shared";

type Lang = "th" | "en";

export function DocumentContent({ id }: { id: string }) {
  const t = useTranslations("docs");
  const client = useClient();
  const doc = useComposedDocument(client, id);
  if (doc.isPending) return <main className="p-8 text-slate-500">{t("loading")}</main>;
  if (doc.isError) return <main className="p-8 text-red-700">{t("loadError")}</main>;
  // Remount the editor state when another version becomes the newest (e.g. after publishing, a save starts v+1).
  return <DocumentEditor key={doc.data.latest?.id ?? "none"} doc={doc.data} />;
}

function DocumentEditor({ doc }: { doc: ComposedDocument }) {
  const t = useTranslations("docs");
  const te = useTranslations("docs.editor");
  const client = useClient();
  const { types, fields, clauses } = useEditorCatalog(doc.doc_type);
  const access = types.data?.types.find((x) => x.doc_type === doc.doc_type);
  const entities = useLegalEntities(client);
  const m = useDocumentMutations(client);
  const [draft, setDraft] = useState<DocumentDraft>(() => doc.draft ?? { title: doc.title, content: { th: { type: "doc", content: [{ type: "paragraph" }] } } });
  const [dirty, setDirty] = useState(false);
  const [lang, setLang] = useState<Lang>("th");
  const [editor, setEditor] = useState<Editor | null>(null);
  const [headings, setHeadings] = useState<Heading[]>([]);
  const status = doc.latest?.status;
  const locked = status === "in_review" || status === "approved";
  const editable = !!access?.can_update && !locked;

  const refreshOutline = useCallback((e: Editor | null) => setHeadings(outline(e)), []);
  const onEditor = useCallback((e: Editor | null) => { setEditor(e); refreshOutline(e); }, [refreshOutline]);
  useEffect(() => {
    if (!dirty) return;
    const warn = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);

  const change = (patch: Partial<DocumentDraft>) => {
    setDraft((d) => ({ ...d, ...patch }));
    setDirty(true);
  };
  const setBody = (l: Lang, body: JSONContent) => {
    setDraft((d) => ({ ...d, content: { ...d.content, [l]: body } }));
    setDirty(true);
    refreshOutline(editor);
  };
  const save = () =>
    m.save.mutate(
      { id: doc.id, rowVersion: doc.row_version, draft: { ...draft, change_summary: draft.change_summary || undefined, effective_from: draft.effective_from || undefined } },
      { onSuccess: () => setDirty(false) },
    );
  const saveError = m.save.error as { status?: number; code?: string } | null;

  return (
    <main className="mx-auto max-w-7xl space-y-4 p-8 text-sm">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <div className="space-y-1">
          <Link className="text-sky-700 underline" href="/documents">{t("back")}</Link>
          <h1 className="text-xl font-semibold" data-testid="doc-title">{doc.title}</h1>
          <p className="flex items-center gap-2 text-slate-600">
            {t(`types.${doc.doc_type}`)} <DocStatus status={doc.status} />
            {doc.latest && <span>{te("version", { no: doc.latest.version })}</span>}
          </p>
        </div>
        {editable && (
          <div className="flex items-center gap-2">
            {dirty && <span className="text-amber-700">{te("unsaved")}</span>}
            {!dirty && m.save.isSuccess && <span className="text-emerald-700" data-testid="saved">{te("saved")}</span>}
            <Button onClick={save} disabled={m.save.isPending || !dirty} data-testid="save-draft">{te("save")}</Button>
          </div>
        )}
      </header>
      {locked && <p className="rounded-md bg-amber-50 p-2 text-amber-900">{te("locked")}</p>}
      {m.save.isError && (
        <p className="text-red-700" role="alert">{saveError?.status === 412 ? te("conflict") : `${te("saveError")}: ${problemText(m.save.error)}`}</p>
      )}

      <div className="grid gap-4 lg:grid-cols-[1fr_16rem]">
        <section className="space-y-3">
          <div className="grid gap-3 md:grid-cols-2">
            <label className="space-y-1">
              <span>{te("title")}</span>
              <input className={input} value={draft.title} maxLength={300} disabled={!editable} onChange={(e) => change({ title: e.target.value })} data-testid="doc-title-input" />
            </label>
            <label className="space-y-1">
              <span>{te("legalEntity")}</span>
              <select className={input} value={draft.legal_entity_id ?? ""} disabled={!editable} onChange={(e) => change({ legal_entity_id: e.target.value || undefined })} data-testid="doc-entity">
                <option value="">{t("create.none")}</option>
                {(entities.data ?? []).map((le) => <option key={le.id} value={le.id}>{le.name_th}</option>)}
              </select>
            </label>
            <label className="space-y-1">
              <span>{te("effectiveFrom")}</span>
              <input type="date" className={input} value={draft.effective_from ?? ""} disabled={!editable} onChange={(e) => change({ effective_from: e.target.value || undefined })} data-testid="doc-effective" />
            </label>
            <label className="space-y-1">
              <span>{te("changeSummary")}</span>
              <input className={input} value={draft.change_summary ?? ""} maxLength={2000} disabled={!editable} onChange={(e) => change({ change_summary: e.target.value })} />
            </label>
          </div>

          <div className="flex flex-wrap items-center gap-2" role="tablist" aria-label={te("language")}>
            {(["th", "en"] as const).map((l) =>
              l === "th" || draft.content.en ? (
                <button key={l} type="button" role="tab" aria-selected={lang === l} onClick={() => setLang(l)} data-testid={`lang-${l}`}
                  className={`rounded-md px-3 py-1 ${lang === l ? "bg-slate-900 text-white" : "bg-slate-100"}`}>{te(l)}</button>
              ) : null,
            )}
            {editable && !draft.content.en && (
              <Button variant="secondary" onClick={() => { setBody("en", { type: "doc", content: [{ type: "paragraph" }] }); setLang("en"); }} data-testid="add-english">{te("addEnglish")}</Button>
            )}
            {editable && draft.content.en && lang === "en" && (
              <Button variant="secondary" onClick={() => { setDraft((d) => ({ ...d, content: { th: d.content.th } })); setDirty(true); setLang("th"); }}>{te("removeEnglish")}</Button>
            )}
          </div>

          {fields.length > 0 && (
            <DocEditor
              docKey={`${doc.id}:${doc.latest?.id}:${lang}`}
              value={(lang === "en" ? draft.content.en : draft.content.th) as JSONContent | undefined}
              onChange={(body) => setBody(lang, body)}
              editable={editable}
              fields={fields}
              clauses={clauses}
              onEditor={onEditor}
              label={`${doc.title} (${te(lang)})`}
            />
          )}

          {doc.missing && (doc.missing.fields.length > 0 || doc.missing.clauses.length > 0) && (
            <div className="rounded-md bg-amber-50 p-3 text-amber-900" data-testid="doc-missing">
              <p className="font-medium">{te("missing")}</p>
              <ul className="list-disc pl-5">
                {doc.missing.fields.map((k) => <li key={k}>{te("missingField", { key: fields.find((f) => f.key === k)?.label ?? k })}</li>)}
                {doc.missing.clauses.map((k) => <li key={k}>{te("missingClause", { key: k })}</li>)}
              </ul>
            </div>
          )}
        </section>

        <aside className="space-y-4">
          <section>
            <h2 className="font-semibold">{te("outline")}</h2>
            {headings.length === 0 ? <p className="text-slate-500">{te("noHeadings")}</p> : (
              <ol className="space-y-1" data-testid="doc-outline">
                {headings.map((h, i) => (
                  <li key={i} style={{ paddingLeft: `${(h.level - 1) * 0.75}rem` }}>
                    <button type="button" className="text-left text-sky-700 hover:underline" onClick={() => {
                      editor?.chain().focus().setTextSelection(h.pos + 1).scrollIntoView().run();
                    }}>{h.text || "…"}</button>
                  </li>
                ))}
              </ol>
            )}
          </section>
          {doc.latest && (
            <section className="space-y-1">
              <h2 className="font-semibold">{te("preview")}</h2>
              <p className="text-xs text-slate-500">{te("draftNote")}</p>
              <ul className="flex flex-wrap gap-2">
                {(["th", "en"] as const).filter((l) => l === "th" || doc.draft?.content.en).flatMap((l) =>
                  (["pdf", "docx"] as const).map((f) => (
                    <li key={`${l}${f}`}>
                      <a className="text-sky-700 underline" href={documentExportHref(BFF, doc.id, l, f, doc.latest!.id)} data-testid={`export-${l}-${f}`} target={f === "pdf" ? "_blank" : undefined} rel="noreferrer">
                        {te("download", { format: f === "pdf" ? "PDF" : "Word", lang: te(l) })}
                      </a>
                    </li>
                  )),
                )}
              </ul>
            </section>
          )}
        </aside>
      </div>

      <RecordVersions entityType={`document_${doc.doc_type}`} entityId={doc.id} canEdit={!!access?.can_update && !dirty} canPublish={!!access?.can_publish} compare={false} />
      <Published id={doc.id} />
      <Compare id={doc.id} entityType={`document_${doc.doc_type}`} />
    </main>
  );
}

function Published({ id }: { id: string }) {
  const te = useTranslations("docs.editor");
  const client = useClient();
  const when = useWhen();
  const list = usePublishedDocumentVersions(client, id);
  return (
    <section className="space-y-2" data-testid="doc-published">
      <h2 className="font-semibold">{te("published")}</h2>
      {!list.data?.length ? <p className="text-slate-500">{te("noPublished")}</p> : (
        <ul className="divide-y divide-slate-100 rounded-md border border-slate-200 bg-white">
          {list.data.map((v) => (
            <li key={v.id} className="flex flex-wrap items-center justify-between gap-2 px-3 py-2" data-version={v.version}>
              <span className="flex items-center gap-2">
                <span className="font-medium">{te("version", { no: v.version })}</span>
                <DocStatus status={v.render_status} />
                {v.approved_at && <span className="text-xs text-slate-500">{te("approvedAt", { date: when(v.approved_at) })}</span>}
                {v.change_summary && <span className="text-xs text-slate-600">— {v.change_summary}</span>}
              </span>
              <span className="flex gap-3">
                {v.files.map((f) => (
                  <a key={f.file_id} className="text-sky-700 underline" href={fileDownloadHref(BFF, f.file_id)} data-testid={`file-${v.version}-${f.language}-${f.format}`}>
                    {te("download", { format: f.format === "pdf" ? "PDF" : "Word", lang: te(f.language) })}
                  </a>
                ))}
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function Compare({ id, entityType }: { id: string; entityType: string }) {
  const te = useTranslations("docs.editor");
  const client = useClient();
  const versions = useRecordVersions(client, entityType, id);
  const list = versions.data ?? [];
  const [from, setFrom] = useState<string>();
  const [to, setTo] = useState<string>();
  const [lang, setLang] = useState<Lang>("th");
  const f = from ?? list[1]?.id;
  const tt = to ?? list[0]?.id;
  const cmp = useDocumentComparison(client, id, f, tt, lang);
  if (list.length < 2) return null;
  const opts = list.map((v) => <option key={v.id} value={v.id}>{te("version", { no: v.version })}</option>);
  return (
    <section className="space-y-2">
      <h2 className="font-semibold">{te("compare")}</h2>
      <div className="flex flex-wrap items-center gap-2">
        <label className="flex items-center gap-1">{te("from")} <select className={field} value={f} onChange={(e) => setFrom(e.target.value)} data-testid="compare-from">{opts}</select></label>
        <label className="flex items-center gap-1">{te("to")} <select className={field} value={tt} onChange={(e) => setTo(e.target.value)} data-testid="compare-to">{opts}</select></label>
        <select className={field} value={lang} onChange={(e) => setLang(e.target.value as Lang)} aria-label={te("language")}>
          <option value="th">{te("th")}</option>
          <option value="en">{te("en")}</option>
        </select>
      </div>
      {cmp.isError && <p className="text-red-700">{problemText(cmp.error)}</p>}
      {cmp.data && <ComparisonView comparison={cmp.data} />}
    </section>
  );
}
