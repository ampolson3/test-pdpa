import { describe, expect, it } from "vitest";
import { formatDate } from "./date";

describe("formatDate", () => {
  it("shows the Buddhist Era year (Gregorian + 543) for th", () => {
    // 2026-01-01T12:00:00Z is still 2026-01-01 in Asia/Bangkok (UTC+7), so BE = 2026 + 543 = 2569.
    const result = formatDate("2026-01-01T12:00:00Z", "th");
    expect(result).toContain("2569");
    expect(result).not.toContain("2026");
  });

  it("shows the Gregorian year for en", () => {
    const result = formatDate("2026-01-01T12:00:00Z", "en");
    expect(result).toContain("2026");
  });

  it("converts to Asia/Bangkok before picking the calendar date (CLAUDE.md rule 11)", () => {
    // 23:30 UTC on 2025-12-31 is already 06:30 on 2026-01-01 in Asia/Bangkok (UTC+7).
    const result = formatDate("2025-12-31T23:30:00Z", "en");
    expect(result).toContain("January 1, 2026");
  });
});
