"use client";

import { useEffect, useMemo } from "react";
import { useTranslations } from "next-intl";
import { EditorContent, Node, mergeAttributes, useEditor, useEditorState, type Editor, type JSONContent } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";

/** A merge field chip: filled from the organization / document when the document is rendered (PLT-16). */
const MergeField = Node.create<{ labels: Record<string, string> }>({
  name: "mergeField",
  group: "inline",
  inline: true,
  atom: true,
  selectable: true,
  addOptions: () => ({ labels: {} }),
  addAttributes: () => ({ key: { default: "" } }),
  parseHTML: () => [{ tag: "span[data-merge-field]", getAttrs: (el) => ({ key: (el as HTMLElement).dataset.mergeField }) }],
  renderHTML({ node, HTMLAttributes }) {
    const key = String(node.attrs.key);
    return ["span", mergeAttributes(HTMLAttributes, { "data-merge-field": key, class: "rounded bg-amber-100 px-1 text-amber-900" }), `{${this.options.labels[key] ?? key}}`];
  },
  renderText: ({ node }) => `{${node.attrs.key}}`,
});

/** A library clause, cited by code and version: its text is inserted when the document is rendered. */
const Clause = Node.create<{ titles: Record<string, string> }>({
  name: "clause",
  group: "block",
  atom: true,
  selectable: true,
  draggable: true,
  addOptions: () => ({ titles: {} }),
  addAttributes: () => ({
    code: { default: "" },
    version: { default: 1, parseHTML: (el) => Number((el as HTMLElement).dataset.version) || 1 },
  }),
  parseHTML: () => [{ tag: "div[data-clause]", getAttrs: (el) => ({ code: (el as HTMLElement).dataset.clause, version: Number((el as HTMLElement).dataset.version) || 1 }) }],
  renderHTML({ node, HTMLAttributes }) {
    const ref = `${node.attrs.code}@${node.attrs.version}`;
    return ["div", mergeAttributes(HTMLAttributes, { "data-clause": node.attrs.code, "data-version": node.attrs.version, class: "my-2 rounded border border-sky-300 bg-sky-50 px-3 py-2 text-sm text-sky-900" }),
      `§ ${this.options.titles[ref] ?? ""} [${ref}]`];
  },
});

export type Heading = { level: number; text: string; pos: number };

/** Headings of the editor content, for the outline (สารบัญ). */
export function outline(editor: Editor | null): Heading[] {
  const out: Heading[] = [];
  editor?.state.doc.descendants((n, pos) => {
    if (n.type.name === "heading") out.push({ level: n.attrs.level as number, text: n.textContent, pos });
    return n.type.name !== "heading";
  });
  return out;
}

const emptyDoc: JSONContent = { type: "doc", content: [{ type: "paragraph" }] };

/**
 * The document editor (TipTap on ProseMirror): paragraphs, headings 1–3, lists, quotes, dividers, bold / italic /
 * underline / strike, merge fields and clause blocks — exactly the nodes the server accepts. `value` is read once
 * per `docKey`; edits go to `onChange` as ProseMirror JSON.
 */
export function DocEditor({
  docKey,
  value,
  onChange,
  editable = true,
  fields,
  clauses,
  allowClauses = true,
  onEditor,
  label,
}: {
  docKey: string;
  value?: JSONContent;
  onChange: (doc: JSONContent) => void;
  editable?: boolean;
  fields: { key: string; label: string }[];
  clauses: { code: string; version: number; title: string }[];
  allowClauses?: boolean;
  onEditor?: (e: Editor | null) => void;
  label: string;
}) {
  const labels = useMemo(() => Object.fromEntries(fields.map((f) => [f.key, f.label])), [fields]);
  const titles = useMemo(() => Object.fromEntries(clauses.map((c) => [`${c.code}@${c.version}`, c.title])), [clauses]);
  const editor = useEditor(
    {
      immediatelyRender: false,
      editable,
      extensions: [
        StarterKit.configure({ heading: { levels: [1, 2, 3] }, code: false, codeBlock: false, link: false }),
        MergeField.configure({ labels }),
        ...(allowClauses ? [Clause.configure({ titles })] : []),
      ],
      content: value ?? emptyDoc,
      editorProps: { attributes: { class: "prose-doc min-h-[16rem] px-4 py-3 focus:outline-none", "aria-label": label, "data-testid": "doc-editor" } },
      onUpdate: ({ editor }) => onChange(editor.getJSON()),
    },
    // Recreated per document / language, and when the labels arrive.
    [docKey, labels, titles, allowClauses],
  );
  useEffect(() => {
    editor?.setEditable(editable);
  }, [editor, editable]);
  useEffect(() => {
    onEditor?.(editor);
  }, [editor, onEditor]);

  return (
    <div className="rounded-md border border-slate-300 bg-white">
      {editable && editor && <Toolbar editor={editor} fields={fields} clauses={allowClauses ? clauses : []} />}
      <EditorContent editor={editor} />
    </div>
  );
}

function Toolbar({ editor, fields, clauses }: { editor: Editor; fields: { key: string; label: string }[]; clauses: { code: string; version: number; title: string }[] }) {
  const t = useTranslations("docs.editor");
  const tb = useTranslations("docs.editor.toolbar");
  const state = useEditorState({
    editor,
    selector: ({ editor: e }) => ({
      bold: e.isActive("bold"), italic: e.isActive("italic"), underline: e.isActive("underline"), strike: e.isActive("strike"),
      h1: e.isActive("heading", { level: 1 }), h2: e.isActive("heading", { level: 2 }), h3: e.isActive("heading", { level: 3 }),
      bullet: e.isActive("bulletList"), ordered: e.isActive("orderedList"), quote: e.isActive("blockquote"),
    }),
  });
  // Clicking a toolbar <button> or <select> moves DOM focus there first; tiptap's own focus() only reclaims it
  // on the next animation frame, so typing right after a toolbar action would otherwise lose its first keystrokes.
  const chain = () => {
    editor.view.dom.focus();
    return editor.chain().focus();
  };
  const buttons: { key: keyof typeof state | "rule" | "undo" | "redo"; text: string; run: () => void }[] = [
    { key: "bold", text: "B", run: () => chain().toggleBold().run() },
    { key: "italic", text: "I", run: () => chain().toggleItalic().run() },
    { key: "underline", text: "U", run: () => chain().toggleUnderline().run() },
    { key: "strike", text: "S", run: () => chain().toggleStrike().run() },
    { key: "h1", text: "H1", run: () => chain().toggleHeading({ level: 1 }).run() },
    { key: "h2", text: "H2", run: () => chain().toggleHeading({ level: 2 }).run() },
    { key: "h3", text: "H3", run: () => chain().toggleHeading({ level: 3 }).run() },
    { key: "bullet", text: "•", run: () => chain().toggleBulletList().run() },
    { key: "ordered", text: "1.", run: () => chain().toggleOrderedList().run() },
    { key: "quote", text: "❝", run: () => chain().toggleBlockquote().run() },
    { key: "rule", text: "—", run: () => chain().setHorizontalRule().run() },
    { key: "undo", text: "↶", run: () => chain().undo().run() },
    { key: "redo", text: "↷", run: () => chain().redo().run() },
  ];
  return (
    <div className="flex flex-wrap items-center gap-1 border-b border-slate-200 bg-slate-50 px-2 py-1 text-sm" role="toolbar">
      {buttons.map((b) => (
        <button
          key={b.key}
          type="button"
          title={tb(b.key)}
          aria-label={tb(b.key)}
          aria-pressed={b.key in state ? state[b.key as keyof typeof state] : undefined}
          onClick={b.run}
          className={`min-w-8 rounded px-2 py-1 ${b.key in state && state[b.key as keyof typeof state] ? "bg-slate-800 text-white" : "hover:bg-slate-200"}`}
        >
          {b.text}
        </button>
      ))}
      <select
        className="ml-2 rounded border border-slate-300 bg-white px-1 py-1"
        aria-label={t("insertField")}
        value=""
        onChange={(e) => {
          if (e.target.value) chain().insertContent({ type: "mergeField", attrs: { key: e.target.value } }).run();
        }}
      >
        <option value="">{t("insertField")}</option>
        {fields.map((f) => <option key={f.key} value={f.key}>{f.label}</option>)}
      </select>
      {clauses.length > 0 && (
        <select
          className="rounded border border-slate-300 bg-white px-1 py-1"
          aria-label={t("insertClause")}
          value=""
          onChange={(e) => {
            const c = clauses.find((x) => `${x.code}@${x.version}` === e.target.value);
            if (c) chain().insertContent({ type: "clause", attrs: { code: c.code, version: c.version } }).run();
          }}
        >
          <option value="">{t("insertClause")}</option>
          {clauses.map((c) => <option key={`${c.code}@${c.version}`} value={`${c.code}@${c.version}`}>{c.title} (v{c.version})</option>)}
        </select>
      )}
    </div>
  );
}
