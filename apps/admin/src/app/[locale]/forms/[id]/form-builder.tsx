"use client";

import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useLocale, useTranslations } from "next-intl";
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import { SortableContext, arrayMove, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { Button } from "@pdpa/ui";
import {
  createApiClient,
  useForm,
  useFormMutations,
  useFormResponseMutations,
  useFormResponses,
  useFormTypes,
  type FormDefinition,
  type FormDraft,
} from "@pdpa/api-client";
import {
  FormRenderer,
  QUESTION_TYPES,
  text,
  validateSchema,
  type Band,
  type Condition,
  type ConditionOp,
  type FormSchema,
  type FormText,
  type Language,
  type Question,
  type QuestionType,
  type SchemaIssue,
  type Scoring,
  type Section,
} from "@pdpa/form-renderer";
import { Link, useRouter } from "@/i18n/routing";
import { problemCode, useRendererMessages } from "@/components/form-messages";

// Builder state: the schema with a stable id on every section and question, so drag and drop keeps
// working while keys are being edited. Ids are stripped before saving.
type BQuestion = Question & { _id: string };
type BSection = Omit<Section, "questions"> & { _id: string; questions: BQuestion[] };
type Selection = { section: string; question?: string } | null;

let seq = 0;
const uid = () => `n${++seq}`;

function toBuilder(schema: FormSchema): BSection[] {
  return schema.sections.map((s) => ({ ...s, _id: uid(), questions: s.questions.map((q) => ({ ...q, _id: uid() })) }));
}

function fromBuilder(sections: BSection[]): FormSchema {
  return {
    sections: sections.map(({ _id, questions, ...s }) => ({
      ...s,
      questions: questions.map(({ _id: _q, ...q }) => q),
    })),
  };
}

export function FormBuilder({ id }: { id: string }) {
  const t = useTranslations("forms");
  const client = useMemo(() => createApiClient("/api/bff"), []);
  const form = useForm(client, id);
  const types = useFormTypes(client);
  const [tab, setTab] = useState<"build" | "responses">("build");
  // Outside Build: saving a new version remounts it (keyed by the latest version).
  const [saved, setSaved] = useState(false);

  if (form.isPending || types.isPending) return <main className="p-8 text-slate-500">{t("loading")}</main>;
  if (form.isError) return <main className="p-8 text-red-700">{t("loadError")}</main>;
  const caps = types.data?.find((x) => x.form_type === form.data.form_type);

  return (
    <main className="mx-auto max-w-7xl space-y-4 p-8 text-sm">
      <header className="flex flex-wrap items-baseline justify-between gap-2">
        <div>
          <Link href="/forms" className="text-slate-500 underline">{t("title")}</Link>
          <h1 className="text-xl font-semibold">{form.data.name}</h1>
          <p className="text-slate-600">
            {t(`types.${form.data.form_type}`)} · {t(`status.${form.data.status}`)} · <span className="font-mono">{form.data.code}</span>
          </p>
        </div>
        <div role="tablist" className="flex gap-1">
          {(["build", "responses"] as const).map((x) => (
            <button key={x} role="tab" aria-selected={tab === x} className={`rounded px-3 py-1 ${tab === x ? "bg-slate-900 text-white" : "bg-slate-100"}`} onClick={() => setTab(x)}>
              {t(`tabs.${x}`)}
            </button>
          ))}
        </div>
      </header>
      {tab === "build" ? (
        <Build key={form.data.versions[0]?.id} saved={saved} setSaved={setSaved} form={form.data} canEdit={!!caps?.can_update && !form.data.global} canPublish={!!caps?.can_publish && !form.data.global} client={client} />
      ) : (
        <Responses form={form.data} canRespond={!!caps?.can_respond} client={client} />
      )}
    </main>
  );
}

function Build({ form, canEdit, canPublish, client, saved, setSaved }: { form: FormDefinition; canEdit: boolean; canPublish: boolean; client: ReturnType<typeof createApiClient>; saved: boolean; setSaved: (v: boolean) => void }) {
  const t = useTranslations("forms");
  const latest = form.versions[0]!;
  const draftVersion = latest.published_at ? undefined : latest;
  const [sections, setSections] = useState<BSection[]>(() => toBuilder(latest.schema as FormSchema));
  const [bands, setBands] = useState<Band[]>(() => ((latest.scoring as Scoring | undefined)?.bands ?? []) as Band[]);
  const [english, setEnglish] = useState(latest.languages.includes("en"));
  const [selected, setSelected] = useState<Selection>(() => (sections[0] ? { section: sections[0]._id } : null));
  const [dirty, setDirty] = useState(false);
  const [issues, setIssues] = useState<SchemaIssue[]>([]);
  const m = useFormMutations(client);

  const schema = useMemo(() => fromBuilder(sections), [sections]);
  const scoring: Scoring | null = bands.length ? { bands } : null;
  const change = (next: BSection[]) => {
    setSections(next);
    setDirty(true);
    setSaved(false);
  };
  const changeBands = (next: Band[]) => {
    setBands(next);
    setDirty(true);
    setSaved(false);
  };

  const save = () => {
    const found = validateSchema(schema, scoring);
    setIssues(found);
    if (found.length) return;
    const draft: FormDraft = { schema, languages: english ? ["th", "en"] : ["th"], ...(scoring ? { scoring } : {}) } as FormDraft;
    m.saveDraft.mutate({ formId: form.id, draft, draftRowVersion: draftVersion?.row_version }, { onSuccess: () => { setDirty(false); setSaved(true); } });
  };
  const publish = () => draftVersion && m.publish.mutate({ formId: form.id, draftRowVersion: draftVersion.row_version });
  const error = m.saveDraft.error ?? m.publish.error;
  const errorText = error
    ? problemCode(error) === "forms.invalid_schema"
      ? t("builder.invalidSchema")
      : problemCode(error) === "conflict.version_mismatch"
        ? t("builder.conflict")
        : t("actionError")
    : null;

  const readOnly = !canEdit;
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2 rounded-md border border-slate-200 bg-white p-3">
        <span className="text-slate-600">
          {draftVersion ? t("builder.editingDraft", { no: draftVersion.version }) : t("builder.editingPublished", { no: latest.version })}
        </span>
        {canEdit && (
          <>
            <label className="ml-4 flex items-center gap-1">
              <input type="checkbox" checked={english} onChange={(e) => { setEnglish(e.target.checked); setDirty(true); }} />
              {t("builder.english")}
            </label>
            <Button onClick={save} disabled={m.saveDraft.isPending || (!dirty && !!draftVersion)}>
              {draftVersion ? t("builder.save") : t("builder.newVersion")}
            </Button>
          </>
        )}
        {canPublish && draftVersion && (
          <Button variant="secondary" onClick={publish} disabled={dirty || m.publish.isPending}>{t("builder.publish", { no: draftVersion.version })}</Button>
        )}
        {saved && <span role="status" className="text-emerald-800">{t("builder.saved")}</span>}
        {dirty && <span className="text-amber-700">{t("builder.unsaved")}</span>}
      </div>
      {errorText && <p role="alert" className="text-red-700">{errorText}</p>}
      {issues.length > 0 && (
        <ul role="alert" className="list-disc rounded-md border border-red-200 bg-red-50 p-3 pl-8 text-red-800">
          {issues.map((i, n) => <li key={n}>{t(`issues.${i.code}`, { at: i.at })}</li>)}
        </ul>
      )}
      <div className="grid gap-4 lg:grid-cols-[1fr_1fr]">
        <div className="space-y-4">
          <Outline sections={sections} onChange={change} selected={selected} onSelect={setSelected} readOnly={readOnly} />
          <Editor sections={sections} onChange={change} selected={selected} readOnly={readOnly} english={english} />
          <ScoringEditor bands={bands} onChange={changeBands} readOnly={readOnly} english={english} />
          <Versions form={form} />
        </div>
        <Preview schema={schema} scoring={scoring} english={english} />
      </div>
    </div>
  );
}

// ---- outline with drag and drop ----

function Outline({ sections, onChange, selected, onSelect, readOnly }: { sections: BSection[]; onChange: (s: BSection[]) => void; selected: Selection; onSelect: (s: Selection) => void; readOnly: boolean }) {
  const t = useTranslations("forms.builder");
  const lang = useLocale() as Language;
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }), useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }));

  const onDragEnd = (e: DragEndEvent) => {
    const { active, over } = e;
    if (!over || active.id === over.id) return;
    const a = String(active.id), o = String(over.id);
    const si = (id: string) => sections.findIndex((s) => s._id === id);
    if (si(a) >= 0) {
      if (si(o) >= 0) onChange(arrayMove(sections, si(a), si(o)));
      return;
    }
    // A question: within its section, or into another section (dropped on a question or a section).
    const from = sections.findIndex((s) => s.questions.some((q) => q._id === a));
    let to = sections.findIndex((s) => s.questions.some((q) => q._id === o));
    if (to < 0) to = si(o);
    if (from < 0 || to < 0) return;
    const next = sections.map((s) => ({ ...s, questions: [...s.questions] }));
    const fromQs = next[from]!.questions;
    const q = fromQs.splice(fromQs.findIndex((x) => x._id === a), 1)[0]!;
    const toQs = next[to]!.questions;
    const at = toQs.findIndex((x) => x._id === o);
    if (from === to) {
      const old = sections[from]!.questions.findIndex((x) => x._id === a);
      next[to]!.questions = arrayMove(sections[from]!.questions, old, sections[from]!.questions.findIndex((x) => x._id === o));
    } else toQs.splice(at < 0 ? toQs.length : at, 0, q);
    onChange(next);
  };

  const addSection = () => {
    const n = sections.length + 1;
    const s: BSection = { _id: uid(), key: `section_${n}_${Date.now() % 1000}`, title: { th: t("newSection") }, questions: [{ _id: uid(), key: `q_${Date.now() % 100000}`, type: "text", label: { th: t("newQuestion") } }] };
    onChange([...sections, s]);
    onSelect({ section: s._id });
  };
  const addQuestion = (sec: BSection) => {
    const q: BQuestion = { _id: uid(), key: `q_${Date.now() % 100000}`, type: "text", label: { th: t("newQuestion") } };
    onChange(sections.map((s) => (s._id === sec._id ? { ...s, questions: [...s.questions, q] } : s)));
    onSelect({ section: sec._id, question: q._id });
  };

  return (
    <section className="rounded-md border border-slate-200 bg-white p-3" aria-labelledby="outline">
      <h2 id="outline" className="mb-2 font-semibold">{t("outline")}</h2>
      {!readOnly && <p className="mb-2 text-xs text-slate-500">{t("dragHelp")}</p>}
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
        <SortableContext items={sections.map((s) => s._id)} strategy={verticalListSortingStrategy}>
          <ol className="space-y-2" data-testid="outline">
            {sections.map((sec) => (
              <SortableItem key={sec._id} id={sec._id} disabled={readOnly} handleLabel={t("dragSection", { name: text(sec.title, lang) })}>
                <div className={`rounded border ${selected?.section === sec._id && !selected.question ? "border-slate-900" : "border-slate-200"} bg-slate-50 p-2`}>
                  <button className="w-full text-left font-medium" onClick={() => onSelect({ section: sec._id })} data-testid={`section-${sec.key}`}>
                    {text(sec.title, lang)} <span className="font-mono text-xs text-slate-500">{sec.key}</span>
                    {sec.visible_if && <span className="ml-1 text-xs text-sky-700">{t("conditional")}</span>}
                  </button>
                  <SortableContext items={sec.questions.map((q) => q._id)} strategy={verticalListSortingStrategy}>
                    <ol className="mt-1 space-y-1 pl-2">
                      {sec.questions.map((q) => (
                        <SortableItem key={q._id} id={q._id} disabled={readOnly} handleLabel={t("dragQuestion", { name: text(q.label, lang) })}>
                          <button
                            className={`w-full rounded px-2 py-1 text-left ${selected?.question === q._id ? "bg-slate-900 text-white" : "bg-white hover:bg-slate-100"}`}
                            onClick={() => onSelect({ section: sec._id, question: q._id })}
                            data-testid={`question-${q.key}`}
                          >
                            {text(q.label, lang)} <span className="text-xs opacity-70">({t(`qtypes.${q.type}`)})</span>
                            {q.visible_if && <span className="ml-1 text-xs">{t("conditional")}</span>}
                          </button>
                        </SortableItem>
                      ))}
                    </ol>
                  </SortableContext>
                  {!readOnly && (
                    <button className="mt-1 text-xs text-slate-600 underline" onClick={() => addQuestion(sec)}>{t("addQuestion")}</button>
                  )}
                </div>
              </SortableItem>
            ))}
          </ol>
        </SortableContext>
      </DndContext>
      {!readOnly && <Button variant="secondary" className="mt-2" onClick={addSection}>{t("addSection")}</Button>}
    </section>
  );
}

function SortableItem({ id, disabled, handleLabel, children }: { id: string; disabled: boolean; handleLabel: string; children: ReactNode }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id, disabled });
  return (
    <li ref={setNodeRef} style={{ transform: CSS.Transform.toString(transform), transition, opacity: isDragging ? 0.6 : 1 }} className="flex items-start gap-1">
      {!disabled && (
        <button type="button" className="cursor-grab px-1 text-slate-400 hover:text-slate-700" aria-label={handleLabel} {...attributes} {...listeners}>
          ⠿
        </button>
      )}
      <div className="flex-1">{children}</div>
    </li>
  );
}

// ---- editors ----

function TextFields({ label, value, onChange, english, readOnly, multiline, testid }: { label: string; value: FormText | undefined; onChange: (v: FormText | undefined) => void; english: boolean; readOnly: boolean; multiline?: boolean; testid?: string }) {
  const t = useTranslations("forms.builder");
  const v = value ?? { th: "" };
  const set = (lang: "th" | "en", s: string) => {
    const next: FormText = { ...v, [lang]: s };
    if (!next.en) delete next.en;
    onChange(next.th || next.en ? next : undefined);
  };
  const Tag = multiline ? "textarea" : "input";
  return (
    <div className="grid gap-2 sm:grid-cols-2">
      <label className="block">
        <span className="block text-slate-600">{label} ({t("th")})</span>
        <Tag className="mt-1 w-full rounded border border-slate-300 px-2 py-1" value={v.th} onChange={(e) => set("th", e.target.value)} disabled={readOnly} data-testid={testid ? `${testid}-th` : undefined} />
      </label>
      {english && (
        <label className="block">
          <span className="block text-slate-600">{label} ({t("en")})</span>
          <Tag className="mt-1 w-full rounded border border-slate-300 px-2 py-1" value={v.en ?? ""} onChange={(e) => set("en", e.target.value)} disabled={readOnly} data-testid={testid ? `${testid}-en` : undefined} />
        </label>
      )}
    </div>
  );
}

function num(s: string): number | undefined {
  if (s.trim() === "") return undefined;
  const n = Number(s);
  return Number.isFinite(n) ? n : undefined;
}

function Editor({ sections, onChange, selected, readOnly, english }: { sections: BSection[]; onChange: (s: BSection[]) => void; selected: Selection; readOnly: boolean; english: boolean }) {
  const t = useTranslations("forms.builder");
  const si = sections.findIndex((s) => s._id === selected?.section);
  const sec = sections[si];
  if (!sec) return null;
  const earlierOf = (stopAt: string | null) => {
    const out: Question[] = [];
    for (const s of sections) {
      for (const q of s.questions) {
        if (q._id === stopAt) return out;
        out.push(q);
      }
      if (s._id === stopAt) return out;
    }
    return out;
  };
  const setSection = (patch: Partial<BSection>) => onChange(sections.map((s, i) => (i === si ? { ...s, ...patch } : s)));
  const removeSection = () => onChange(sections.filter((_, i) => i !== si));

  if (!selected?.question) {
    // Conditions of a section may use questions of the sections before it.
    const before = sections.slice(0, si).flatMap((s) => s.questions);
    return (
      <section className="space-y-3 rounded-md border border-slate-200 bg-white p-3" aria-labelledby="section-editor" data-testid="section-editor">
        <h2 id="section-editor" className="font-semibold">{t("sectionEditor")}</h2>
        <label className="block">
          <span className="block text-slate-600">{t("key")}</span>
          <input className="mt-1 w-full rounded border border-slate-300 px-2 py-1 font-mono" value={sec.key} onChange={(e) => setSection({ key: e.target.value })} disabled={readOnly} data-testid="section-key" />
        </label>
        <TextFields label={t("sectionTitle")} value={sec.title} onChange={(v) => setSection({ title: v ?? { th: "" } })} english={english} readOnly={readOnly} testid="section-title" />
        <TextFields label={t("description")} value={sec.description} onChange={(v) => setSection({ description: v })} english={english} readOnly={readOnly} multiline />
        <ConditionEditor value={sec.visible_if} onChange={(c) => setSection({ visible_if: c })} earlier={before} readOnly={readOnly} />
        {!readOnly && sections.length > 1 && <Button variant="ghost" onClick={removeSection}>{t("removeSection")}</Button>}
      </section>
    );
  }

  const qi = sec.questions.findIndex((q) => q._id === selected.question);
  const q = sec.questions[qi];
  if (!q) return null;
  const setQ = (patch: Partial<BQuestion>) => {
    const next = { ...q, ...patch } as BQuestion;
    for (const k of Object.keys(patch) as (keyof BQuestion)[]) if (next[k] === undefined) delete next[k];
    setSection({ questions: sec.questions.map((x, i) => (i === qi ? next : x)) });
  };
  const setType = (type: QuestionType) => {
    const patch: Partial<BQuestion> = { type, min: undefined, max: undefined, max_length: undefined };
    if (type === "single_choice" || type === "multi_choice") patch.options = q.options?.length && q.type !== "yes_no" ? q.options : [{ value: "option_1", label: { th: t("newOption") } }];
    else if (type === "yes_no") patch.options = undefined;
    else patch.options = undefined;
    setQ(patch);
  };
  const choice = q.type === "single_choice" || q.type === "multi_choice";
  return (
    <section className="space-y-3 rounded-md border border-slate-200 bg-white p-3" aria-labelledby="question-editor" data-testid="question-editor">
      <h2 id="question-editor" className="font-semibold">{t("questionEditor")}</h2>
      <div className="grid gap-2 sm:grid-cols-2">
        <label className="block">
          <span className="block text-slate-600">{t("key")}</span>
          <input className="mt-1 w-full rounded border border-slate-300 px-2 py-1 font-mono" value={q.key} onChange={(e) => setQ({ key: e.target.value })} disabled={readOnly} data-testid="question-key" />
        </label>
        <label className="block">
          <span className="block text-slate-600">{t("qtype")}</span>
          <select className="mt-1 w-full rounded border border-slate-300 px-2 py-1" value={q.type} onChange={(e) => setType(e.target.value as QuestionType)} disabled={readOnly} data-testid="question-type">
            {QUESTION_TYPES.map((x) => <option key={x} value={x}>{t(`qtypes.${x}`)}</option>)}
          </select>
        </label>
      </div>
      <TextFields label={t("label")} value={q.label} onChange={(v) => setQ({ label: v ?? { th: "" } })} english={english} readOnly={readOnly} testid="question-label" />
      <TextFields label={t("help")} value={q.help} onChange={(v) => setQ({ help: v })} english={english} readOnly={readOnly} />
      <div className="flex flex-wrap items-end gap-4">
        <label className="flex items-center gap-1">
          <input type="checkbox" checked={!!q.required} onChange={(e) => setQ({ required: e.target.checked || undefined })} disabled={readOnly} data-testid="question-required" />
          {t("required")}
        </label>
        {(choice || q.type === "yes_no") && (
          <label className="block">
            <span className="block text-slate-600">{t("weight")}</span>
            <input type="number" step="any" min={0} className="mt-1 w-24 rounded border border-slate-300 px-2 py-1" value={q.weight ?? ""} onChange={(e) => setQ({ weight: num(e.target.value) })} disabled={readOnly} data-testid="question-weight" />
          </label>
        )}
        {q.type === "number" && (
          <>
            <label className="block">
              <span className="block text-slate-600">{t("min")}</span>
              <input type="number" step="any" className="mt-1 w-28 rounded border border-slate-300 px-2 py-1" value={q.min ?? ""} onChange={(e) => setQ({ min: num(e.target.value) })} disabled={readOnly} />
            </label>
            <label className="block">
              <span className="block text-slate-600">{t("max")}</span>
              <input type="number" step="any" className="mt-1 w-28 rounded border border-slate-300 px-2 py-1" value={q.max ?? ""} onChange={(e) => setQ({ max: num(e.target.value) })} disabled={readOnly} />
            </label>
          </>
        )}
        {(q.type === "text" || q.type === "textarea") && (
          <label className="block">
            <span className="block text-slate-600">{t("maxLength")}</span>
            <input type="number" min={1} className="mt-1 w-28 rounded border border-slate-300 px-2 py-1" value={q.max_length ?? ""} onChange={(e) => setQ({ max_length: num(e.target.value) })} disabled={readOnly} />
          </label>
        )}
      </div>
      {(choice || q.type === "yes_no") && <OptionsEditor q={q} onChange={(options) => setQ({ options })} readOnly={readOnly} english={english} />}
      <ConditionEditor value={q.visible_if} onChange={(c) => setQ({ visible_if: c })} earlier={earlierOf(q._id)} readOnly={readOnly} />
      {!readOnly && sec.questions.length > 1 && (
        <Button variant="ghost" onClick={() => setSection({ questions: sec.questions.filter((_, i) => i !== qi) })}>{t("removeQuestion")}</Button>
      )}
    </section>
  );
}

function OptionsEditor({ q, onChange, readOnly, english }: { q: Question; onChange: (o: Question["options"]) => void; readOnly: boolean; english: boolean }) {
  const t = useTranslations("forms.builder");
  const yesNo = q.type === "yes_no";
  const options = yesNo
    ? (["yes", "no"] as const).map((v) => q.options?.find((o) => o.value === v) ?? { value: v })
    : q.options ?? [];
  const set = (i: number, patch: Partial<NonNullable<Question["options"]>[number]>) => {
    const next = options.map((o, n) => {
      if (n !== i) return o;
      const x = { ...o, ...patch };
      if (x.score === undefined) delete x.score;
      return x;
    });
    onChange(yesNo ? next.filter((o) => o.score !== undefined) : next);
  };
  return (
    <fieldset className="space-y-2">
      <legend className="text-slate-600">{yesNo ? t("scores") : t("options")}</legend>
      {options.map((o, i) => (
        <div key={i} className="flex flex-wrap items-end gap-2" data-testid={`option-${i}`}>
          {yesNo ? (
            <span className="w-40">{t(o.value === "yes" ? "yes" : "no")}</span>
          ) : (
            <>
              <input aria-label={t("optionValue")} className="w-28 rounded border border-slate-300 px-2 py-1 font-mono" value={o.value} onChange={(e) => set(i, { value: e.target.value })} disabled={readOnly} />
              <input aria-label={`${t("optionLabel")} (${t("th")})`} className="w-40 rounded border border-slate-300 px-2 py-1" value={o.label?.th ?? ""} onChange={(e) => set(i, { label: { ...(o.label ?? { th: "" }), th: e.target.value } })} disabled={readOnly} />
              {english && (
                <input aria-label={`${t("optionLabel")} (${t("en")})`} className="w-40 rounded border border-slate-300 px-2 py-1" value={o.label?.en ?? ""} onChange={(e) => set(i, { label: { ...(o.label ?? { th: "" }), en: e.target.value || undefined } })} disabled={readOnly} />
              )}
            </>
          )}
          <input aria-label={t("score")} type="number" step="any" className="w-20 rounded border border-slate-300 px-2 py-1" placeholder={t("score")} value={o.score ?? ""} onChange={(e) => set(i, { score: num(e.target.value) })} disabled={readOnly} />
          {!yesNo && !readOnly && options.length > 1 && (
            <button className="text-xs text-red-700 underline" onClick={() => onChange(options.filter((_, n) => n !== i))}>{t("remove")}</button>
          )}
        </div>
      ))}
      {!yesNo && !readOnly && (
        <button className="text-xs underline" onClick={() => onChange([...options, { value: `option_${options.length + 1}`, label: { th: t("newOption") } }])}>{t("addOption")}</button>
      )}
    </fieldset>
  );
}

// The editor handles "always", one comparison, or all/any of comparisons; deeper nesting (valid in the
// format) is shown as-is and can only be removed.
type Comparison = { question: string; op: ConditionOp; value?: unknown };
const OPS: ConditionOp[] = ["eq", "neq", "in", "not_in", "gt", "gte", "lt", "lte", "answered", "not_answered"];

function parse(c: Condition | undefined): { mode: "always" | "all" | "any" | "complex"; list: Comparison[] } {
  if (!c) return { mode: "always", list: [] };
  const flat = (x: Condition): x is Comparison => !!x.question && !!x.op && !x.all?.length && !x.any?.length;
  if (flat(c)) return { mode: "all", list: [c] };
  if (c.all?.length && c.all.every(flat)) return { mode: "all", list: c.all as Comparison[] };
  if (c.any?.length && c.any.every(flat)) return { mode: "any", list: c.any as Comparison[] };
  return { mode: "complex", list: [] };
}

function build(mode: "all" | "any", list: Comparison[]): Condition | undefined {
  if (!list.length) return undefined;
  if (list.length === 1 && mode === "all") return list[0];
  return mode === "all" ? { all: list } : { any: list };
}

function ConditionEditor({ value, onChange, earlier, readOnly }: { value: Condition | undefined; onChange: (c: Condition | undefined) => void; earlier: Question[]; readOnly: boolean }) {
  const t = useTranslations("forms.builder");
  const lang = useLocale() as Language;
  const parsed = parse(value);
  const [mode, setMode] = useState<"all" | "any">(parsed.mode === "any" ? "any" : "all");
  useEffect(() => {
    if (parsed.mode === "any" || parsed.mode === "all") setMode(parsed.mode);
  }, [parsed.mode]);

  if (parsed.mode === "complex") {
    return (
      <div className="rounded border border-slate-200 p-2">
        <p className="text-slate-600">{t("conditionComplex")}</p>
        <pre className="overflow-auto text-xs">{JSON.stringify(value, null, 2)}</pre>
        {!readOnly && <button className="text-xs underline" onClick={() => onChange(undefined)}>{t("conditionRemove")}</button>}
      </div>
    );
  }
  const list = parsed.list;
  const setList = (next: Comparison[], m = mode) => onChange(build(m, next));
  const add = () => {
    const q = earlier[earlier.length - 1];
    if (q) setList([...list, defaultComparison(q)]);
  };
  return (
    <fieldset className="space-y-2 rounded border border-slate-200 p-2" data-testid="condition-editor">
      <legend className="px-1 text-slate-600">{t("showWhen")}</legend>
      {list.length === 0 && <p className="text-slate-500">{t("always")}</p>}
      {list.length > 1 && (
        <select aria-label={t("combine")} className="rounded border border-slate-300 px-2 py-1" value={mode} onChange={(e) => { const m = e.target.value as "all" | "any"; setMode(m); setList(list, m); }} disabled={readOnly}>
          <option value="all">{t("combineAll")}</option>
          <option value="any">{t("combineAny")}</option>
        </select>
      )}
      {list.map((c, i) => {
        const q = earlier.find((x) => x.key === c.question);
        const set = (patch: Partial<Comparison>) => setList(list.map((x, n) => (n === i ? cleanComparison({ ...x, ...patch }, earlier) : x)));
        return (
          <div key={i} className="flex flex-wrap items-center gap-2" data-testid={`condition-${i}`}>
            <select aria-label={t("conditionQuestion")} className="max-w-56 rounded border border-slate-300 px-2 py-1" value={c.question} onChange={(e) => { const nq = earlier.find((x) => x.key === e.target.value); if (nq) set(defaultComparison(nq)); }} disabled={readOnly}>
              {!q && <option value={c.question}>{c.question}</option>}
              {earlier.map((x) => <option key={x.key} value={x.key}>{text(x.label, lang)}</option>)}
            </select>
            <select aria-label={t("conditionOp")} className="rounded border border-slate-300 px-2 py-1" value={c.op} onChange={(e) => set({ op: e.target.value as ConditionOp })} disabled={readOnly}>
              {OPS.filter((op) => q?.type === "number" || !["gt", "gte", "lt", "lte"].includes(op)).map((op) => <option key={op} value={op}>{t(`ops.${op}`)}</option>)}
            </select>
            {q && <ValueInput q={q} c={c} onChange={(v) => set({ value: v })} readOnly={readOnly} />}
            {!readOnly && <button className="text-xs text-red-700 underline" onClick={() => setList(list.filter((_, n) => n !== i))}>{t("remove")}</button>}
          </div>
        );
      })}
      {!readOnly && (earlier.length ? <button className="text-xs underline" onClick={add}>{t("addCondition")}</button> : <p className="text-xs text-slate-500">{t("noEarlier")}</p>)}
    </fieldset>
  );
}

function defaultComparison(q: Question): Comparison {
  if (q.type === "yes_no") return { question: q.key, op: "eq", value: "yes" };
  if (q.type === "single_choice" || q.type === "multi_choice") return { question: q.key, op: "eq", value: q.options?.[0]?.value ?? "" };
  if (q.type === "number") return { question: q.key, op: "gte", value: 0 };
  return { question: q.key, op: "answered" };
}

function cleanComparison(c: Comparison, earlier: Question[]): Comparison {
  const q = earlier.find((x) => x.key === c.question);
  if (c.op === "answered" || c.op === "not_answered") return { question: c.question, op: c.op };
  if ((c.op === "in" || c.op === "not_in") && !Array.isArray(c.value)) return { ...c, value: c.value === undefined || c.value === "" ? [] : [c.value] };
  if (c.op !== "in" && c.op !== "not_in" && Array.isArray(c.value)) return { ...c, value: c.value[0] ?? (q?.type === "number" ? 0 : "") };
  if (c.value === undefined) return { ...c, value: q?.type === "number" ? 0 : q?.type === "yes_no" ? "yes" : q?.options?.[0]?.value ?? "" };
  return c;
}

function ValueInput({ q, c, onChange, readOnly }: { q: Question; c: Comparison; onChange: (v: unknown) => void; readOnly: boolean }) {
  const t = useTranslations("forms.builder");
  const lang = useLocale() as Language;
  if (c.op === "answered" || c.op === "not_answered") return null;
  const options =
    q.type === "yes_no"
      ? [{ value: "yes", label: t("yes") }, { value: "no", label: t("no") }]
      : (q.options ?? []).map((o) => ({ value: o.value, label: text(o.label, lang) || o.value }));
  if (c.op === "in" || c.op === "not_in") {
    const list = Array.isArray(c.value) ? (c.value as unknown[]) : [];
    if (options.length)
      return (
        <span className="flex flex-wrap gap-2">
          {options.map((o) => (
            <label key={o.value} className="flex items-center gap-1">
              <input type="checkbox" checked={list.includes(o.value)} onChange={(e) => onChange(e.target.checked ? [...list, o.value] : list.filter((x) => x !== o.value))} disabled={readOnly} />
              {o.label}
            </label>
          ))}
        </span>
      );
    return <input aria-label={t("conditionValue")} className="w-48 rounded border border-slate-300 px-2 py-1" value={list.join(", ")} onChange={(e) => onChange(e.target.value.split(",").map((s) => s.trim()).filter(Boolean).map((s) => (q.type === "number" && Number.isFinite(Number(s)) ? Number(s) : s)))} disabled={readOnly} />;
  }
  if (options.length)
    return (
      <select aria-label={t("conditionValue")} className="rounded border border-slate-300 px-2 py-1" value={String(c.value ?? "")} onChange={(e) => onChange(e.target.value)} disabled={readOnly}>
        {options.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
      </select>
    );
  if (q.type === "number")
    return <input aria-label={t("conditionValue")} type="number" step="any" className="w-28 rounded border border-slate-300 px-2 py-1" value={typeof c.value === "number" ? c.value : ""} onChange={(e) => onChange(num(e.target.value) ?? 0)} disabled={readOnly} data-testid="condition-value" />;
  return <input aria-label={t("conditionValue")} className="w-40 rounded border border-slate-300 px-2 py-1" value={String(c.value ?? "")} onChange={(e) => onChange(e.target.value)} disabled={readOnly} />;
}

function ScoringEditor({ bands, onChange, readOnly, english }: { bands: Band[]; onChange: (b: Band[]) => void; readOnly: boolean; english: boolean }) {
  const t = useTranslations("forms.builder");
  const set = (i: number, patch: Partial<Band>) =>
    onChange(bands.map((b, n) => {
      if (n !== i) return b;
      const x = { ...b, ...patch };
      if (x.max === undefined) delete x.max;
      return x;
    }));
  const add = () => {
    const last = bands[bands.length - 1];
    const min = last ? (last.max ?? last.min) + 1 : 0;
    onChange([...bands.map((b, i) => (i === bands.length - 1 && b.max === undefined ? { ...b, max: b.min } : b)), { key: `band_${bands.length + 1}`, label: { th: t("newBand") }, min }]);
  };
  return (
    <section className="space-y-2 rounded-md border border-slate-200 bg-white p-3" aria-labelledby="scoring" data-testid="scoring-editor">
      <h2 id="scoring" className="font-semibold">{t("scoring")}</h2>
      <p className="text-xs text-slate-500">{t("scoringHelp")}</p>
      {bands.map((b, i) => (
        <div key={i} className="flex flex-wrap items-end gap-2" data-testid={`band-${i}`}>
          <input aria-label={t("bandKey")} className="w-24 rounded border border-slate-300 px-2 py-1 font-mono" value={b.key} onChange={(e) => set(i, { key: e.target.value })} disabled={readOnly} />
          <input aria-label={`${t("bandLabel")} (${t("th")})`} className="w-32 rounded border border-slate-300 px-2 py-1" value={b.label.th} onChange={(e) => set(i, { label: { ...b.label, th: e.target.value } })} disabled={readOnly} />
          {english && <input aria-label={`${t("bandLabel")} (${t("en")})`} className="w-32 rounded border border-slate-300 px-2 py-1" value={b.label.en ?? ""} onChange={(e) => set(i, { label: { ...b.label, en: e.target.value || undefined } })} disabled={readOnly} />}
          <input aria-label={t("min")} type="number" step="any" className="w-20 rounded border border-slate-300 px-2 py-1" value={b.min} onChange={(e) => set(i, { min: num(e.target.value) ?? 0 })} disabled={readOnly} />
          <input aria-label={t("max")} type="number" step="any" className="w-20 rounded border border-slate-300 px-2 py-1" placeholder="∞" value={b.max ?? ""} onChange={(e) => set(i, { max: num(e.target.value) })} disabled={readOnly} />
          {!readOnly && <button className="text-xs text-red-700 underline" onClick={() => onChange(bands.filter((_, n) => n !== i))}>{t("remove")}</button>}
        </div>
      ))}
      {!readOnly && <button className="text-xs underline" onClick={add}>{t("addBand")}</button>}
    </section>
  );
}

function Preview({ schema, scoring, english }: { schema: FormSchema; scoring: Scoring | null; english: boolean }) {
  const t = useTranslations("forms.builder");
  const [lang, setLang] = useState<Language>("th");
  const messages = useRendererMessages(lang);
  const [ok, setOk] = useState(false);
  useEffect(() => {
    if (!english) setLang("th");
  }, [english]);
  const issues = validateSchema(schema, scoring);
  return (
    <section className="space-y-2 lg:sticky lg:top-4 lg:self-start" aria-labelledby="preview" data-testid="preview">
      <div className="flex items-center justify-between">
        <h2 id="preview" className="font-semibold">{t("preview")}</h2>
        {english && (
          <div role="group" aria-label={t("previewLanguage")} className="flex gap-1">
            {(["th", "en"] as const).map((l) => (
              <button key={l} aria-pressed={lang === l} className={`rounded px-2 py-0.5 ${lang === l ? "bg-slate-900 text-white" : "bg-slate-100"}`} onClick={() => setLang(l)}>
                {t(l)}
              </button>
            ))}
          </div>
        )}
      </div>
      {issues.length ? (
        <p className="text-slate-500">{t("previewInvalid")}</p>
      ) : (
        <FormRenderer
          schema={schema}
          scoring={scoring}
          language={lang}
          messages={messages}
          showScore
          idPrefix="preview"
          onSubmit={() => setOk(true)}
          actions={({ submit }) => (
            <div className="flex items-center gap-2">
              <Button type="button" variant="secondary" onClick={() => { setOk(false); submit(); }}>{t("previewCheck")}</Button>
              {ok && <span role="status" className="text-emerald-800">{t("previewOk")}</span>}
            </div>
          )}
        />
      )}
    </section>
  );
}

function Versions({ form }: { form: FormDefinition }) {
  const t = useTranslations("forms");
  return (
    <section className="rounded-md border border-slate-200 bg-white p-3" aria-labelledby="versions">
      <h2 id="versions" className="mb-1 font-semibold">{t("versions")}</h2>
      <ul className="space-y-1">
        {form.versions.map((v) => (
          <li key={v.id} data-testid={`version-${v.version}`}>
            {t("versionItem", { no: v.version })} · {v.published_at ? t("publishedAt", { at: new Date(v.published_at).toLocaleString() }) : t("status.draft")}
            {v.id === form.current_version_id && <span className="ml-1 text-emerald-800">{t("current")}</span>}
          </li>
        ))}
      </ul>
    </section>
  );
}

function Responses({ form, canRespond, client }: { form: FormDefinition; canRespond: boolean; client: ReturnType<typeof createApiClient> }) {
  const t = useTranslations("forms");
  const lang = useLocale() as Language;
  const router = useRouter();
  const list = useFormResponses(client, form.id);
  const m = useFormResponseMutations(client);
  const scoring = form.versions.find((v) => v.id === form.current_version_id)?.scoring as Scoring | undefined;
  return (
    <section className="space-y-3">
      {canRespond && form.status === "published" && (
        <Button onClick={() => m.start.mutate({ formId: form.id }, { onSuccess: (r) => router.push(`/form-responses/${r.id}`) })} disabled={m.start.isPending}>
          {t("responses.start")}
        </Button>
      )}
      {list.isPending ? (
        <p className="text-slate-500">{t("loading")}</p>
      ) : list.isError ? (
        <p className="text-red-700">{t("loadError")}</p>
      ) : list.data.length === 0 ? (
        <p className="text-slate-500">{t("responses.empty")}</p>
      ) : (
        <table className="w-full rounded-md border border-slate-200 bg-white" data-testid="responses">
          <thead className="bg-slate-50 text-left text-slate-600">
            <tr>
              <th className="px-3 py-2">{t("responses.owner")}</th>
              <th className="px-3 py-2">{t("statusLabel")}</th>
              <th className="px-3 py-2">{t("responses.score")}</th>
              <th className="px-3 py-2">{t("responses.band")}</th>
            </tr>
          </thead>
          <tbody>
            {list.data.map((r) => (
              <tr key={r.id} className="border-t border-slate-100">
                <td className="px-3 py-2"><Link href={`/form-responses/${r.id}`} className="underline">{r.owner_name ?? "—"}</Link></td>
                <td className="px-3 py-2">{t(`responses.status.${r.status}`)}</td>
                <td className="px-3 py-2">{r.status === "submitted" ? `${r.result.score} / ${r.result.max_score}` : "—"}</td>
                <td className="px-3 py-2">{r.result.band ? text(scoring?.bands.find((b) => b.key === r.result.band)?.label, lang) || r.result.band : "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
