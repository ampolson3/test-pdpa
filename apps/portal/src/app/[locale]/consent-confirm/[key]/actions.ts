"use server";

import { randomUUID } from "node:crypto";
import { confirmDoubleOptIn } from "@/lib/consent";

// CON-23's own acceptance criterion, server-side: the decision has no effect until this call succeeds.
export type ConfirmState = {
  idem: string;
  purposeCode?: string;
  error?: string;
};

export async function confirmDoubleOptInAction(key: string, locale: string, prev: ConfirmState, form: FormData): Promise<ConfirmState> {
  const token = String(form.get("token") ?? "").trim();
  if (!token) return { idem: prev.idem, error: "not_found" };
  try {
    const res = await confirmDoubleOptIn(key, prev.idem, token, locale);
    if (res.ok) return { idem: prev.idem, purposeCode: res.purpose_code };
    if (res.problem.status >= 500) return { idem: prev.idem, error: "unavailable" };
    const code = res.problem.code === "consent.invalid_transition" ? "invalid_transition" : res.problem.code === "not_found" ? "not_found" : "generic";
    return { idem: randomUUID(), error: code };
  } catch {
    return { idem: prev.idem, error: "unavailable" };
  }
}
