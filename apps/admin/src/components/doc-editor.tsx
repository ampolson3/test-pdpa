"use client";

import { useEffect, useMemo } from "react";
import { useTranslations } from "next-intl";
import { EditorContent, Extension, Mark, Node, mergeAttributes, useEditor, useEditorState, type Editor, type JSONContent } from "@tiptap/react";
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

/** A document-wide table of contents (DPA-06): the editor shows a placeholder; the server computes the real list
 *  of headings fresh at render/export time (render.Outline), so it can never go stale against the body. */
const TableOfContents = Node.create({
  name: "tableOfContents",
  group: "block",
  atom: true,
  selectable: true,
  parseHTML: () => [{ tag: "div[data-toc]" }],
  renderHTML: ({ HTMLAttributes }) => ["div", mergeAttributes(HTMLAttributes, { "data-toc": "", class: "rounded border border-dashed border-slate-400 bg-slate-50 px-3 py-2 text-slate-500" }), "สารบัญ (คำนวณใหม่ตอนส่งออก)"],
});

/** Paragraph/heading text alignment (DPA-06's "ย่อหน้า"): a plain attribute on both block types, stored as
 *  textAlign so the server's own renderers (render.go's alignedOpen / docx.go's jcValue) read it directly. */
const TextAlign = Extension.create({
  name: "textAlign",
  addGlobalAttributes() {
    return [{
      types: ["paragraph", "heading"],
      attributes: { textAlign: { default: null, parseHTML: (el) => el.style.textAlign || null, renderHTML: (attrs) => (attrs.textAlign ? { style: `text-align: ${attrs.textAlign}` } : {}) } },
    }];
  },
  addCommands() {
    return {
      setTextAlign: (align: string) => ({ commands }: { commands: { updateAttributes: (type: string, attrs: Record<string, unknown>) => boolean } }) =>
        commands.updateAttributes("paragraph", { textAlign: align }) && commands.updateAttributes("heading", { textAlign: align }),
    } as never;
  },
});

/** A font-size mark (DPA-06's "ฟอนต์"): two presets only — kept narrow so every renderer (and the one embedded
 *  Sarabun font every export already relies on for correct Thai glyphs) stays exact, never an arbitrary family. */
const FontSize = Mark.create({
  name: "fontSize",
  addAttributes: () => ({ size: { default: "normal", parseHTML: (el) => (el as HTMLElement).dataset.fontSize || "normal" } }),
  parseHTML: () => [{ tag: "span[data-font-size]" }],
  renderHTML: ({ HTMLAttributes, mark }) => {
    const pct = mark.attrs.size === "large" ? "130%" : mark.attrs.size === "small" ? "80%" : undefined;
    return ["span", mergeAttributes(HTMLAttributes, { "data-font-size": mark.attrs.size, ...(pct ? { style: `font-size: ${pct}` } : {}) }), 0];
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
        TableOfContents,
        TextAlign,
        FontSize,
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
      align: (e.getAttributes("paragraph").textAlign ?? e.getAttributes("heading").textAlign ?? "left") as string,
      fontSize: (e.getAttributes("fontSize").size ?? "normal") as string,
    }),
  });
  // Clicking a toolbar <button> or <select> moves DOM focus there first; tiptap's own focus() only reclaims it
  // on the next animation frame, so typing right after a toolbar action would otherwise lose its first keystrokes.
  const chain = () => {
    editor.view.dom.focus();
    return editor.chain().focus();
  };
  type BoolKey = "bold" | "italic" | "underline" | "strike" | "h1" | "h2" | "h3" | "bullet" | "ordered" | "quote";
  const buttons: { key: BoolKey | "rule" | "undo" | "redo"; text: string; run: () => void }[] = [
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
  const aligns: { value: string; text: string }[] = [
    { value: "left", text: "⇤" }, { value: "center", text: "↔" }, { value: "right", text: "⇥" }, { value: "justify", text: "≡" },
  ];
  const sizes: { value: string; label: string }[] = [
    { value: "normal", label: tb("fontSizeNormal") }, { value: "small", label: tb("fontSizeSmall") }, { value: "large", label: tb("fontSizeLarge") },
  ];
  return (
    <div className="flex flex-wrap items-center gap-1 border-b border-slate-200 bg-slate-50 px-2 py-1 text-sm" role="toolbar">
      {buttons.map((b) => (
        <button
          key={b.key}
          type="button"
          title={tb(b.key)}
          aria-label={tb(b.key)}
          aria-pressed={b.key in state ? state[b.key as BoolKey] : undefined}
          onClick={b.run}
          className={`min-w-8 rounded px-2 py-1 ${b.key in state && state[b.key as BoolKey] ? "bg-slate-800 text-white" : "hover:bg-slate-200"}`}
        >
          {b.text}
        </button>
      ))}
      <span className="ml-2 flex items-center gap-1" role="group" aria-label={tb("align")}>
        {aligns.map((a) => (
          <button key={a.value} type="button" title={tb(`align${a.value}` as "alignleft")} aria-label={tb(`align${a.value}` as "alignleft")}
            aria-pressed={state.align === a.value} onClick={() => chain().updateAttributes("paragraph", { textAlign: a.value }).updateAttributes("heading", { textAlign: a.value }).run()}
            className={`min-w-8 rounded px-2 py-1 ${state.align === a.value ? "bg-slate-800 text-white" : "hover:bg-slate-200"}`}>{a.text}</button>
        ))}
      </span>
      <select
        className="rounded border border-slate-300 bg-white px-1 py-1"
        aria-label={tb("fontSize")}
        value={state.fontSize}
        onChange={(e) => {
          const v = e.target.value;
          if (v === "normal") chain().unsetMark("fontSize").run();
          else chain().setMark("fontSize", { size: v }).run();
        }}
      >
        {sizes.map((s) => <option key={s.value} value={s.value}>{s.label}</option>)}
      </select>
      <button type="button" title={tb("insertToc")} aria-label={tb("insertToc")} onClick={() => chain().insertContent({ type: "tableOfContents" }).run()}
        className="rounded px-2 py-1 hover:bg-slate-200">{tb("insertToc")}</button>
      <select
        className="rounded border border-slate-300 bg-white px-1 py-1"
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
