import { z } from "zod";
import { evaluate } from "./engine";
import type { Answers, FormSchema, Scoring } from "./types";

/**
 * A zod schema for react-hook-form whose only rule is the engine itself: every problem evaluate() finds
 * becomes an issue on that question (message = the error code), so the browser and the server can't
 * disagree about what is valid.
 */
export function answersSchema(schema: FormSchema, scoring: Scoring | null | undefined, opts: { requireAll: () => boolean; sections?: string[] | null }) {
  return z.record(z.string(), z.unknown()).superRefine((values, ctx) => {
    const r = evaluate(schema, scoring, values as Answers, opts.requireAll(), opts.sections);
    for (const e of r.errors) ctx.addIssue({ code: z.ZodIssueCode.custom, path: [e.question], message: e.code });
  });
}
