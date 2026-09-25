// The PLT-06 form format — the same JSON as backend/internal/platform/forms (schema.go), which is the
// authority: the server re-validates everything this package checks in the browser.

export type Language = "th" | "en";

/** Text by language; th is always present. */
export type FormText = { th: string; en?: string };

export const QUESTION_TYPES = ["text", "textarea", "number", "date", "email", "single_choice", "multi_choice", "yes_no"] as const;
export type QuestionType = (typeof QUESTION_TYPES)[number];

export const CONDITION_OPS = ["eq", "neq", "in", "not_in", "gt", "gte", "lt", "lte", "answered", "not_answered"] as const;
export type ConditionOp = (typeof CONDITION_OPS)[number];

export interface Condition {
  question?: string;
  op?: ConditionOp;
  value?: unknown;
  all?: Condition[];
  any?: Condition[];
}

export interface Option {
  value: string;
  label?: FormText;
  score?: number;
}

export interface Question {
  key: string;
  type: QuestionType;
  label: FormText;
  help?: FormText;
  required?: boolean;
  options?: Option[];
  min?: number;
  max?: number;
  max_length?: number;
  weight?: number;
  visible_if?: Condition;
}

export interface Section {
  key: string;
  title: FormText;
  description?: FormText;
  visible_if?: Condition;
  questions: Question[];
}

export interface FormSchema {
  sections: Section[];
}

export interface Band {
  key: string;
  label: FormText;
  min: number;
  max?: number;
}

export interface Scoring {
  bands: Band[];
}

export type AnswerValue = string | number | string[];
export type Answers = Record<string, unknown>;

export type ErrorCode =
  | "required"
  | "invalid_type"
  | "invalid_option"
  | "out_of_range"
  | "too_long"
  | "invalid_date"
  | "invalid_email"
  | "unknown_question";

export interface FieldError {
  question: string;
  code: ErrorCode;
}

export interface Result {
  /** Question keys shown, in order. */
  visible: string[];
  /** Valid answers of visible questions only. */
  answers: Record<string, AnswerValue>;
  /** Empty when the answers can be submitted. */
  errors: FieldError[];
  score: number;
  max_score: number;
  band?: string;
}

/** A text in the wanted language, falling back to Thai. */
export function text(t: FormText | undefined, lang: Language): string {
  if (!t) return "";
  return (lang === "en" && t.en) || t.th;
}
