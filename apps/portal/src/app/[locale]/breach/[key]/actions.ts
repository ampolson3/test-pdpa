"use server";

import { randomUUID } from "node:crypto";
import { submitPublicReport, type BreachType } from "@/lib/breach";

export type SubmitState = {
  // Idempotency-Key of the next attempt: the same one while a submission may have reached the API (a double
  // click or a lost response replays the first result), a fresh one after the API answered with an error.
  idem: string;
  incidentNo?: string;
  error?: { code: string; fields: { field: string; code: string }[] };
};

export async function submitBreachReport(key: string, locale: string, prev: SubmitState, form: FormData): Promise<SubmitState> {
  const title = String(form.get("title") ?? "").trim();
  const description = String(form.get("description") ?? "").trim();
  const breachTypes = form.getAll("breach_types").map(String) as BreachType[];
  const occurredAt = String(form.get("occurred_at") ?? "").trim();
  const reporterContact = String(form.get("reporter_contact") ?? "").trim();
  if (!title || !description || breachTypes.length === 0) {
    return { idem: prev.idem, error: { code: "missing_fields", fields: [] } };
  }
  try {
    const res = await submitPublicReport(key, prev.idem, locale, {
      title, description, breach_types: breachTypes,
      occurred_at: occurredAt ? new Date(occurredAt).toISOString() : undefined,
      reporter_contact: reporterContact || undefined,
    });
    if (res.ok) return { idem: prev.idem, incidentNo: res.result.incident_no };
    if (res.problem.status >= 500) return { idem: prev.idem, error: { code: "unavailable", fields: [] } };
    return { idem: randomUUID(), error: { code: res.problem.code, fields: res.problem.errors ?? [] } };
  } catch {
    return { idem: prev.idem, error: { code: "unavailable", fields: [] } };
  }
}
