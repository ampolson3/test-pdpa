// TypeScript twin of backend/internal/platform/forms/eval.go. Both must give the results in
// fixtures/engine-cases.json (engine.test.ts here, eval_test.go there) — change them together.
import type { AnswerValue, Answers, Condition, ErrorCode, FieldError, FormSchema, Question, Result, Scoring } from "./types";

const DEFAULT_MAX_CHARS = 2000;

/**
 * Applies a schema to answers: which questions are visible (in order — a hidden question counts as
 * unanswered for later conditions), which answers are valid, the score and its band. With requireAll,
 * visible required questions must be answered (final submission); without it only given answers are
 * checked (saving a draft). sections, when given, limits checking and scoring to those sections.
 */
export function evaluate(schema: FormSchema, scoring: Scoring | null | undefined, input: Answers, requireAll: boolean, sections?: string[] | null): Result {
  const res: Result = { visible: [], answers: {}, errors: [], score: 0, max_score: 0 };
  const shown: Record<string, AnswerValue> = {};
  for (const sec of schema.sections) {
    if (sec.visible_if && !holds(sec.visible_if, shown)) continue;
    const inScope = !sections || sections.includes(sec.key);
    for (const q of sec.questions) {
      if (q.visible_if && !holds(q.visible_if, shown)) continue;
      res.visible.push(q.key);
      const raw = input[q.key];
      if (raw === undefined || isEmpty(raw)) {
        if (requireAll && q.required && inScope) res.errors.push({ question: q.key, code: "required" });
        if (inScope) res.max_score += maxScore(q);
        continue;
      }
      const [v, code] = normalize(q, raw);
      if (code) {
        if (inScope) {
          res.errors.push({ question: q.key, code });
          res.max_score += maxScore(q);
        }
        continue;
      }
      shown[q.key] = v!;
      if (inScope) {
        res.answers[q.key] = v!;
        res.score += score(q, v!);
        res.max_score += maxScore(q);
      }
    }
  }
  res.score = round(res.score);
  res.max_score = round(res.max_score);
  for (const b of scoring?.bands ?? []) {
    if (res.score >= b.min && (b.max === undefined || b.max === null || res.score <= b.max)) {
      res.band = b.key;
      break;
    }
  }
  return res;
}

/** The key of the section a question belongs to ("" when none). */
export function sectionOf(schema: FormSchema, question: string): string {
  return schema.sections.find((s) => s.questions.some((q) => q.key === question))?.key ?? "";
}

export function isEmpty(v: unknown): boolean {
  if (v === null || v === undefined) return true;
  if (typeof v === "string") return v.trim() === "";
  if (Array.isArray(v)) return v.length === 0;
  return false;
}

// Same address syntax Go's net/mail accepts for a bare addr-spec (dot-atom local part and domain).
const ATEXT = "[A-Za-z0-9!#$%&'*+/=?^_`{|}~\\-\\u0080-\\uFFFF]+";
const EMAIL = new RegExp(`^${ATEXT}(\\.${ATEXT})*@${ATEXT}(\\.${ATEXT})*$`);

function validDate(s: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(s)) return false;
  const [y, m, d] = s.split("-").map(Number) as [number, number, number];
  const dt = new Date(Date.UTC(y, m - 1, d));
  return dt.getUTCFullYear() === y && dt.getUTCMonth() === m - 1 && dt.getUTCDate() === d;
}

function normalize(q: Question, v: unknown): [AnswerValue | null, ErrorCode | ""] {
  switch (q.type) {
    case "text":
    case "textarea": {
      if (typeof v !== "string") return [null, "invalid_type"];
      const s = v.trim();
      if ([...s].length > (q.max_length || DEFAULT_MAX_CHARS)) return [null, "too_long"];
      return [s, ""];
    }
    case "email": {
      if (typeof v !== "string") return [null, "invalid_type"];
      const s = v.trim();
      if (!EMAIL.test(s) || s.length > 254) return [null, "invalid_email"];
      return [s, ""];
    }
    case "date":
      if (typeof v !== "string") return [null, "invalid_type"];
      return validDate(v) ? [v, ""] : [null, "invalid_date"];
    case "number": {
      if (typeof v !== "number" || !Number.isFinite(v)) return [null, "invalid_type"];
      if ((q.min !== undefined && v < q.min) || (q.max !== undefined && v > q.max)) return [null, "out_of_range"];
      return [v, ""];
    }
    case "single_choice":
    case "yes_no":
      if (typeof v !== "string") return [null, "invalid_type"];
      return validOption(q, v) ? [v, ""] : [null, "invalid_option"];
    case "multi_choice": {
      if (!Array.isArray(v) || v.some((x) => typeof x !== "string")) return [null, "invalid_type"];
      const out: string[] = [];
      for (const s of v as string[]) {
        if (!validOption(q, s)) return [null, "invalid_option"];
        if (!out.includes(s)) out.push(s);
      }
      return [out, ""];
    }
  }
  return [null, "invalid_type"];
}

function validOption(q: Question, s: string): boolean {
  if (q.type === "yes_no") return s === "yes" || s === "no";
  return (q.options ?? []).some((o) => o.value === s);
}

const weight = (q: Question) => q.weight ?? 1;

function optionScore(q: Question, value: string): number {
  return q.options?.find((o) => o.value === value)?.score ?? 0;
}

function score(q: Question, v: AnswerValue): number {
  if (typeof v === "string" && (q.type === "single_choice" || q.type === "yes_no")) return weight(q) * optionScore(q, v);
  if (Array.isArray(v)) return weight(q) * v.reduce((sum, s) => sum + optionScore(q, s), 0);
  return 0;
}

/** The most a question can add: the best option, or all positive options of a multi choice. */
export function maxScore(q: Question): number {
  let best = 0;
  for (const o of q.options ?? []) {
    if (o.score === undefined || o.score === null) continue;
    if (q.type === "multi_choice") {
      if (o.score > 0) best += o.score;
    } else if (o.score > best) best = o.score;
  }
  return weight(q) * best;
}

function holds(c: Condition, answers: Record<string, AnswerValue>): boolean {
  if (c.all?.length) return c.all.every((n) => holds(n, answers));
  if (c.any?.length) return c.any.some((n) => holds(n, answers));
  const answered = c.question !== undefined && c.question in answers;
  if (c.op === "answered") return answered;
  if (c.op === "not_answered") return !answered;
  if (!answered) return c.op === "neq" || c.op === "not_in";
  const v = answers[c.question!]!;
  switch (c.op) {
    case "eq":
      return matches(v, c.value);
    case "neq":
      return !matches(v, c.value);
    case "in":
    case "not_in": {
      const list = Array.isArray(c.value) ? c.value : [];
      return list.some((x) => matches(v, x)) === (c.op === "in");
    }
    case "gt":
    case "gte":
    case "lt":
    case "lte": {
      if (typeof v !== "number" || typeof c.value !== "number") return false;
      if (c.op === "gt") return v > c.value;
      if (c.op === "gte") return v >= c.value;
      if (c.op === "lt") return v < c.value;
      return v <= c.value;
    }
  }
  return false;
}

/** Compares an answer with a condition value; a multi-choice answer matches when it includes it. */
function matches(answer: AnswerValue, value: unknown): boolean {
  if (Array.isArray(answer)) return typeof value === "string" && answer.includes(value);
  if (typeof answer === "number") return typeof value === "number" && answer === value;
  if (typeof value === "boolean") return (value && answer === "yes") || (!value && answer === "no");
  return answer === value;
}

const round = (f: number) => Math.round(f * 100) / 100;

export type { FieldError };
