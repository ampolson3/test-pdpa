import { describe, expect, it } from "vitest";
import fixture from "./fixtures/engine-cases.json";
import { evaluate } from "./engine";
import type { FieldError, FormSchema, Scoring } from "./types";

// The same fixture drives backend/internal/platform/forms/eval_test.go: the browser and the server agree.
describe("evaluate (shared fixture)", () => {
  const schema = fixture.schema as unknown as FormSchema;
  const scoring = fixture.scoring as Scoring;
  for (const c of fixture.cases) {
    it(c.name, () => {
      const r = evaluate(schema, scoring, c.answers as Record<string, unknown>, c.require_all, (c as { sections?: string[] }).sections);
      expect(r.visible).toEqual(c.visible);
      expect(r.errors).toEqual(c.errors as FieldError[]);
      expect(Object.keys(r.answers).sort()).toEqual(c.answer_keys);
      expect(r.score).toBe(c.score);
      expect(r.max_score).toBe(c.max_score);
      expect(r.band ?? "").toBe(c.band);
    });
  }
});
