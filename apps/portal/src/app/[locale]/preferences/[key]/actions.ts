"use server";

import { randomUUID } from "node:crypto";
import { startPreferenceVerification, updatePreferences, viewPreferenceCenter, type PreferenceCenter } from "@/lib/preferences";

export type StartState = {
  idem: string;
  verificationId?: string;
  channel?: string;
  identifier?: { type: string; value: string };
  error?: string;
};

/** Phase 1: send an OTP to the e-mail/phone the subject gave. */
export async function requestCode(key: string, locale: string, prev: StartState, form: FormData): Promise<StartState> {
  const email = String(form.get("email") ?? "").trim();
  const phone = String(form.get("phone") ?? "").trim();
  const identifier = email ? { type: "email", value: email } : phone ? { type: "phone", value: phone } : null;
  if (!identifier) return { idem: prev.idem, error: "identifier_missing" };
  const res = await startPreferenceVerification(key, prev.idem, identifier, locale);
  if (res.ok) return { idem: randomUUID(), verificationId: res.data.verification_id, channel: res.data.channel, identifier };
  return { idem: randomUUID(), error: res.problem.code, identifier };
}

export type ViewState = {
  idem: string;
  center?: PreferenceCenter;
  error?: string;
};

/** Phase 2: the OTP code confirms the identity; shows every active purpose with the subject's own decision. */
export async function confirmCode(
  key: string,
  locale: string,
  verificationId: string,
  identifier: { type: string; value: string },
  prev: ViewState,
  form: FormData,
): Promise<ViewState> {
  const code = String(form.get("code") ?? "").trim();
  const res = await viewPreferenceCenter(key, prev.idem, verificationId, code, identifier, locale);
  if (res.ok) return { idem: randomUUID(), center: res.data };
  return { idem: randomUUID(), error: res.problem.code };
}

export type SaveState = {
  idem: string;
  receipt?: string;
  error?: string;
};

/** Phase 3: grant/refuse/withdraw any purpose in one call — withdrawal is allowed here, unlike a public form. */
export async function savePreferences(
  key: string,
  locale: string,
  verificationId: string,
  identifier: { type: string; value: string },
  center: PreferenceCenter,
  prev: SaveState,
  form: FormData,
): Promise<SaveState> {
  const decisions: { purpose_code: string; purpose_version_no: number; decision: "CONSENTED" | "NOT_CONSENTED" | "WITHDRAWN" }[] = [];
  for (const p of center.purposes) {
    const choice = form.get(`decision:${p.code}`);
    if (choice !== "grant" && choice !== "withdraw") continue; // untouched purposes are left as they are
    decisions.push({ purpose_code: p.code, purpose_version_no: p.version, decision: choice === "grant" ? "CONSENTED" : "WITHDRAWN" });
  }
  if (decisions.length === 0) return { idem: prev.idem, error: "no_change" };
  const res = await updatePreferences(key, prev.idem, verificationId, identifier, decisions, locale);
  if (res.ok) return { idem: randomUUID(), receipt: res.data.receipt_no };
  return { idem: randomUUID(), error: res.problem.code };
}
