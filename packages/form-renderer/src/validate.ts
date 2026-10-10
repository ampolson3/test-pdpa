// Builder-side checks of a schema, mirroring Validate in backend/internal/platform/forms/schema.go so the
// builder can point at the problem before saving. The server check remains the authority.
import type { Condition, FormSchema, FormText, Question, Scoring } from "./types";

export type SchemaIssueCode =
  | "no_sections"
  | "bad_key"
  | "duplicate_key"
  | "missing_label"
  | "no_questions"
  | "needs_options"
  | "bad_option"
  | "bad_range"
  | "bad_condition"
  | "bad_band";

export interface SchemaIssue {
  /** Section key, question key or band key the issue is about. */
  at: string;
  code: SchemaIssueCode;
}

const KEY = /^[a-z][a-z0-9_]{0,59}$/;
const hasThai = (t?: FormText) => !!t && t.th.trim() !== "";

export function validateSchema(schema: FormSchema, scoring?: Scoring | null): SchemaIssue[] {
  const issues: SchemaIssue[] = [];
  if (!schema.sections.length) issues.push({ at: "", code: "no_sections" });
  const sections = new Set<string>();
  const earlier = new Map<string, Question>();
  for (const sec of schema.sections) {
    if (!KEY.test(sec.key)) issues.push({ at: sec.key, code: "bad_key" });
    else if (sections.has(sec.key)) issues.push({ at: sec.key, code: "duplicate_key" });
    sections.add(sec.key);
    if (!hasThai(sec.title)) issues.push({ at: sec.key, code: "missing_label" });
    if (sec.visible_if && !conditionOK(sec.visible_if, earlier)) issues.push({ at: sec.key, code: "bad_condition" });
    if (!sec.questions.length) issues.push({ at: sec.key, code: "no_questions" });
    for (const q of sec.questions) {
      if (!KEY.test(q.key)) issues.push({ at: q.key, code: "bad_key" });
      else if (earlier.has(q.key)) issues.push({ at: q.key, code: "duplicate_key" });
      if (!hasThai(q.label)) issues.push({ at: q.key, code: "missing_label" });
      const choice = q.type === "single_choice" || q.type === "multi_choice";
      if (choice && !q.options?.length) issues.push({ at: q.key, code: "needs_options" });
      const values = new Set<string>();
      for (const o of q.options ?? []) {
        const bad =
          !o.value ||
          values.has(o.value) ||
          (q.type === "yes_no" && o.value !== "yes" && o.value !== "no") ||
          (!choice && q.type !== "yes_no") ||
          (q.type !== "yes_no" && !hasThai(o.label));
        if (bad) {
          issues.push({ at: q.key, code: "bad_option" });
          break;
        }
        values.add(o.value);
      }
      if (q.min !== undefined && q.max !== undefined && q.max < q.min) issues.push({ at: q.key, code: "bad_range" });
      if (q.visible_if && !conditionOK(q.visible_if, earlier)) issues.push({ at: q.key, code: "bad_condition" });
      earlier.set(q.key, q);
    }
  }
  const bands = scoring?.bands ?? [];
  bands.forEach((b, i) => {
    const prev = bands[i - 1];
    if (
      !KEY.test(b.key) ||
      bands.findIndex((x) => x.key === b.key) !== i ||
      !hasThai(b.label) ||
      (b.max !== undefined && b.max < b.min) ||
      (prev && (prev.max === undefined || b.min <= prev.max))
    )
      issues.push({ at: b.key, code: "bad_band" });
  });
  return issues;
}

function conditionOK(c: Condition, earlier: Map<string, Question>, depth = 0): boolean {
  if (depth > 5) return false;
  const nested = [...(c.all ?? []), ...(c.any ?? [])];
  if (nested.length) {
    if (c.question || c.op || (c.all?.length && c.any?.length)) return false;
    return nested.every((n) => conditionOK(n, earlier, depth + 1));
  }
  const q = c.question ? earlier.get(c.question) : undefined;
  if (!q || !c.op) return false;
  switch (c.op) {
    case "in":
    case "not_in":
      return Array.isArray(c.value);
    case "gt":
    case "gte":
    case "lt":
    case "lte":
      return typeof c.value === "number" && q.type === "number";
    case "eq":
    case "neq":
      return c.value !== undefined && c.value !== null && c.value !== "";
  }
  return true;
}
