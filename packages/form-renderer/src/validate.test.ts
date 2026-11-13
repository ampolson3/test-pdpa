import { describe, expect, it } from "vitest";
import fixture from "./fixtures/engine-cases.json";
import { validateSchema } from "./validate";
import { fromFormValues } from "./FormRenderer";
import type { FormSchema, Scoring } from "./types";

const schema = fixture.schema as unknown as FormSchema;

describe("validateSchema", () => {
  it("accepts the shared fixture", () => {
    expect(validateSchema(schema, fixture.scoring as Scoring)).toEqual([]);
  });
  it("points at the broken parts", () => {
    const s: FormSchema = {
      sections: [
        {
          key: "a",
          title: { th: "" },
          visible_if: { question: "later", op: "eq", value: "x" },
          questions: [
            { key: "q", type: "single_choice", label: { th: "ถาม" } },
            { key: "q", type: "number", label: { th: "ซ้ำ" }, min: 5, max: 1 },
            { key: "later", type: "text", label: { th: "ท" }, visible_if: { question: "q", op: "gt", value: "x" } },
          ],
        },
      ],
    };
    const bands: Scoring = { bands: [ { key: "hi", label: { th: "สูง" }, min: 5, max: 10 }, { key: "lo", label: { th: "ต่ำ" }, min: 0 } ] };
    expect(validateSchema(s, bands)).toEqual([
      { at: "a", code: "missing_label" },
      { at: "a", code: "bad_condition" },
      { at: "q", code: "needs_options" },
      { at: "q", code: "duplicate_key" },
      { at: "q", code: "bad_range" },
      { at: "later", code: "bad_condition" },
      { at: "lo", code: "bad_band" },
    ]);
  });
});

describe("fromFormValues", () => {
  it("parses numbers and leaves out empty inputs", () => {
    expect(fromFormValues(schema, { org_name: "", subjects: "20000", sensitive_types: [], start_date: "2026-01-02", vendor: "cloud" })).toEqual({
      subjects: 20000,
      start_date: "2026-01-02",
      vendor: "cloud",
    });
  });
});
