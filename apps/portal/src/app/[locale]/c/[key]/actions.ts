"use server";

import { randomUUID } from "node:crypto";
import { submitConsents, type Decision } from "@/lib/consent";

export type SubmitState = {
  // Idempotency-Key of the next attempt: the same one while a submission may have reached the API (a double
  // click or a lost response replays the first result), a fresh one after the API answered with an error.
  idem: string;
  receipt?: string;
  error?: { code: string; fields: { field: string; code: string }[] };
};

/** Posts the form's decisions: every purpose shown is decided — ticked = consent, unticked = no consent. */
export async function submitConsent(key: string, locale: string, prev: SubmitState, form: FormData): Promise<SubmitState> {
  const identifiers: { type: string; value: string }[] = [];
  for (const type of ["email", "phone"]) {
    const v = String(form.get(type) ?? "").trim();
    if (v) identifiers.push({ type, value: v });
  }
  const decisions: Decision[] = [];
  for (const [name, value] of form.entries()) {
    if (!name.startsWith("version:")) continue;
    const code = name.slice("version:".length);
    const consented = form.get(`consent:${code}`) === "on";
    const d: Decision = { purpose_code: code, purpose_version_no: Number(value), decision: consented ? "CONSENTED" : "NOT_CONSENTED" };
    if (consented) {
      const prefs: Record<string, string[]> = {};
      for (const [n, v] of form.entries()) {
        const prefix = `pref:${code}:`;
        if (n.startsWith(prefix)) (prefs[n.slice(prefix.length)] ??= []).push(String(v));
      }
      if (Object.keys(prefs).length > 0) d.preferences = prefs;
    }
    decisions.push(d);
  }
  if (identifiers.length === 0) return { idem: prev.idem, error: { code: "identifier_missing", fields: [] } };

  try {
    const res = await submitConsents(key, prev.idem, { subject: { identifiers }, decisions, language: locale });
    if (res.ok) return { idem: prev.idem, receipt: res.receipt_no };
    if (res.problem.status >= 500) return { idem: prev.idem, error: { code: "unavailable", fields: [] } };
    const fields = res.problem.errors ?? [];
    // Everything unticked was already given: nothing to record (withdrawing is done through the organization).
    const code = fields.some((f) => f.field === "decisions" && f.code === "no_change") ? "no_change" : res.problem.code;
    return { idem: randomUUID(), error: { code, fields } };
  } catch {
    return { idem: prev.idem, error: { code: "unavailable", fields: [] } };
  }
}
