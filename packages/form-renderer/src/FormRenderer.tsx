"use client";

import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Controller, useForm, useWatch, type Control, type FieldErrors, type UseFormRegister } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { evaluate } from "./engine";
import { answersSchema } from "./zod";
import { text, type AnswerValue, type Answers, type ErrorCode, type FieldError, type FormSchema, type Language, type Question, type Result, type Scoring, type Section } from "./types";

/** UI strings, localized by the app (packages/i18n `formRenderer.*`). */
export interface RendererMessages {
  errors: Record<ErrorCode, string>;
  yes: string;
  no: string;
  choose: string;
  required: string;
  /** e.g. "Score 12 of 20" */
  score: (score: number, max: number) => string;
  readOnly: string;
}

export interface RendererActions {
  /** Validate everything (required answers too) and call onSubmit. */
  submit: () => void;
  /** Validate only the answers given and call onSaveDraft. */
  saveDraft: () => void;
  busy: boolean;
}

export interface FormRendererProps {
  schema: FormSchema;
  scoring?: Scoring | null;
  language: Language;
  messages: RendererMessages;
  /** Current answers. */
  values?: Answers;
  /** Sections the user may answer; the others are shown read-only. Default: all. */
  editable?: string[];
  /** Answers of the editable sections, cleared ones as null. */
  onSubmit?: (answers: Record<string, AnswerValue | null>, result: Result) => void | Promise<void>;
  onSaveDraft?: (answers: Record<string, AnswerValue | null>, result: Result) => void | Promise<void>;
  /** Problems the server reported (422 errors[]). */
  serverErrors?: FieldError[];
  showScore?: boolean;
  disabled?: boolean;
  /** Rendered under each section's title (e.g. who it is assigned to). */
  sectionExtra?: (section: Section) => ReactNode;
  actions?: (a: RendererActions) => ReactNode;
  idPrefix?: string;
}

/** The one renderer for PLT-06 forms: admin builder preview, responses, and every module's forms. */
export function FormRenderer({
  schema,
  scoring,
  language,
  messages,
  values,
  editable,
  onSubmit,
  onSaveDraft,
  serverErrors,
  showScore,
  disabled,
  sectionExtra,
  actions,
  idPrefix = "f",
}: FormRendererProps) {
  const requireAll = useRef(false);
  const resolver = useMemo(
    () => zodResolver(answersSchema(schema, scoring, { requireAll: () => requireAll.current, sections: editable })),
    [schema, scoring, editable],
  );
  const { register, control, handleSubmit, setError, reset, formState } = useForm<Answers>({
    resolver,
    defaultValues: toFormValues(schema, values ?? {}),
    mode: "onSubmit",
    reValidateMode: "onChange",
  });
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    reset(toFormValues(schema, values ?? {}));
  }, [schema, values, reset]);
  useEffect(() => {
    for (const e of serverErrors ?? []) setError(e.question, { type: "server", message: e.code });
  }, [serverErrors, setError]);

  const watched = useWatch({ control }) as Answers;
  const live = useMemo(() => evaluate(schema, scoring, fromFormValues(schema, watched), false), [schema, scoring, watched]);
  const visible = new Set(live.visible);
  const canEdit = (sec: Section) => !disabled && (!editable || editable.includes(sec.key));

  const run = (full: boolean, cb?: FormRendererProps["onSubmit"]) => () => {
    requireAll.current = full;
    void handleSubmit(async (vals) => {
      if (!cb) return;
      const answers = fromFormValues(schema, vals);
      const out: Record<string, AnswerValue | null> = {};
      for (const sec of schema.sections) {
        if (editable && !editable.includes(sec.key)) continue;
        for (const q of sec.questions) {
          const v = answers[q.key];
          if (v !== undefined) out[q.key] = v as AnswerValue;
          else if (values && values[q.key] !== undefined) out[q.key] = null;
        }
      }
      setBusy(true);
      try {
        await cb(out, evaluate(schema, scoring, answers, full, editable));
      } finally {
        setBusy(false);
      }
    })();
  };

  return (
    <form
      noValidate
      onSubmit={(e) => {
        e.preventDefault();
        run(true, onSubmit)();
      }}
      className="space-y-6"
    >
      {schema.sections.map((sec) => {
        const questions = sec.questions.filter((q) => visible.has(q.key));
        if (!questions.length) return null;
        const edit = canEdit(sec);
        return (
          <fieldset key={sec.key} className="rounded-md border border-slate-200 bg-white p-4" data-section={sec.key}>
            <legend className="px-1 text-base font-semibold">{text(sec.title, language)}</legend>
            {sec.description && <p className="mb-2 text-sm text-slate-600">{text(sec.description, language)}</p>}
            {sectionExtra?.(sec)}
            {!edit && !disabled && <p className="mb-2 text-xs text-slate-500">{messages.readOnly}</p>}
            <div className="space-y-4">
              {questions.map((q) => (
                <Field
                  key={q.key}
                  q={q}
                  id={`${idPrefix}-${q.key}`}
                  language={language}
                  messages={messages}
                  register={register}
                  control={control}
                  errors={formState.errors}
                  readOnly={!edit}
                />
              ))}
            </div>
          </fieldset>
        );
      })}
      {showScore && scoring && (
        <p className="text-sm text-slate-700" data-testid="form-score">
          {messages.score(live.score, live.max_score)}
          {live.band && <> · {text(scoring.bands.find((b) => b.key === live.band)?.label, language)}</>}
        </p>
      )}
      {actions?.({ submit: run(true, onSubmit), saveDraft: run(false, onSaveDraft), busy })}
    </form>
  );
}

interface FieldProps {
  q: Question;
  id: string;
  language: Language;
  messages: RendererMessages;
  register: UseFormRegister<Answers>;
  control: Control<Answers>;
  errors: FieldErrors<Answers>;
  readOnly: boolean;
}

function Field({ q, id, language, messages, register, control, errors, readOnly }: FieldProps) {
  const code = errors[q.key]?.message as ErrorCode | undefined;
  const errId = `${id}-error`;
  const helpId = `${id}-help`;
  const describedBy = [q.help ? helpId : "", code ? errId : ""].filter(Boolean).join(" ") || undefined;
  const label = (
    <>
      {text(q.label, language)}
      {q.required && (
        <span className="text-red-600" aria-label={messages.required}>
          {" "}*
        </span>
      )}
    </>
  );
  const input = "block w-full rounded border border-slate-300 px-2 py-1 text-sm disabled:bg-slate-50";
  const common = { id, disabled: readOnly, "aria-invalid": code ? true : undefined, "aria-describedby": describedBy };
  const group = q.type === "single_choice" || q.type === "multi_choice" || q.type === "yes_no";
  const options =
    q.type === "yes_no"
      ? [
          { value: "yes", label: messages.yes },
          { value: "no", label: messages.no },
        ]
      : (q.options ?? []).map((o) => ({ value: o.value, label: text(o.label, language) || o.value }));

  let control_: ReactNode;
  switch (q.type) {
    case "textarea":
      control_ = <textarea {...common} {...register(q.key)} rows={3} className={input} />;
      break;
    case "number":
      control_ = <input {...common} type="number" step="any" {...register(q.key)} className={input} />;
      break;
    case "date":
      control_ = <input {...common} type="date" {...register(q.key)} className={input} />;
      break;
    case "email":
      control_ = <input {...common} type="email" {...register(q.key)} className={input} />;
      break;
    case "text":
      control_ = <input {...common} type="text" {...register(q.key)} className={input} />;
      break;
    case "single_choice":
      if (options.length > 5) {
        control_ = (
          <select {...common} {...register(q.key)} className={input}>
            <option value="">{messages.choose}</option>
            {options.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        );
        break;
      }
    // fall through: few options render as radios
    case "yes_no":
      control_ = (
        <div className="flex flex-wrap gap-4" role="radiogroup" aria-labelledby={`${id}-label`} aria-describedby={describedBy}>
          {options.map((o) => (
            <label key={o.value} className="flex items-center gap-1 text-sm">
              <input type="radio" value={o.value} disabled={readOnly} {...register(q.key)} />
              {o.label}
            </label>
          ))}
        </div>
      );
      break;
    case "multi_choice":
      control_ = (
        <Controller
          name={q.key}
          control={control}
          render={({ field }) => {
            const list = Array.isArray(field.value) ? (field.value as string[]) : [];
            return (
              <div className="flex flex-col gap-1" role="group" aria-labelledby={`${id}-label`} aria-describedby={describedBy}>
                {options.map((o) => (
                  <label key={o.value} className="flex items-center gap-1 text-sm">
                    <input
                      type="checkbox"
                      disabled={readOnly}
                      checked={list.includes(o.value)}
                      onChange={(e) => field.onChange(e.target.checked ? [...list, o.value] : list.filter((v) => v !== o.value))}
                    />
                    {o.label}
                  </label>
                ))}
              </div>
            );
          }}
        />
      );
      break;
  }

  return (
    <div data-question={q.key}>
      {group ? (
        <p id={`${id}-label`} className="mb-1 text-sm font-medium">
          {label}
        </p>
      ) : (
        <label id={`${id}-label`} htmlFor={id} className="mb-1 block text-sm font-medium">
          {label}
        </label>
      )}
      {q.help && (
        <p id={helpId} className="mb-1 text-xs text-slate-500">
          {text(q.help, language)}
        </p>
      )}
      {control_}
      {code && (
        <p id={errId} role="alert" className="mt-1 text-xs text-red-700">
          {messages.errors[code] ?? code}
        </p>
      )}
    </div>
  );
}

/** Answers → what the inputs hold (numbers as strings, empty multi as []). */
function toFormValues(schema: FormSchema, answers: Answers): Answers {
  const out: Answers = {};
  for (const sec of schema.sections)
    for (const q of sec.questions) {
      const v = answers[q.key];
      if (q.type === "multi_choice") out[q.key] = Array.isArray(v) ? v : [];
      else if (q.type === "number") out[q.key] = typeof v === "number" ? String(v) : "";
      else out[q.key] = typeof v === "string" ? v : "";
    }
  return out;
}

/** What the inputs hold → answers (empty inputs left out, numbers parsed). */
export function fromFormValues(schema: FormSchema, vals: Answers): Answers {
  const out: Answers = {};
  for (const sec of schema.sections)
    for (const q of sec.questions) {
      const v = vals[q.key];
      if (v === undefined || v === null || v === "" || (Array.isArray(v) && !v.length)) continue;
      if (q.type === "number" && typeof v === "string") {
        const n = Number(v);
        out[q.key] = v.trim() === "" ? undefined : Number.isFinite(n) ? n : v;
        if (out[q.key] === undefined) delete out[q.key];
      } else out[q.key] = v;
    }
  return out;
}
