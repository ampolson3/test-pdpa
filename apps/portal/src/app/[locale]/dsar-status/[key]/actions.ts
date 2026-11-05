"use server";

import { randomUUID } from "node:crypto";
import {
  startTrackingVerification,
  getTrackedStatus,
  submitTrackingDocument,
  type DsarTrackMethod,
  type DsarTrackedStatus,
} from "@/lib/dsar-track";

export type TrackState = {
  // Idempotency-Key of the next attempt — the same one while a call may have reached the API, a fresh one
  // after the API answered with an error (dsar/[key]'s own actions.ts already established this pattern).
  idem: string;
  requestNo?: string;
  method?: DsarTrackMethod;
  contact?: string;
  verificationId?: string;
  channel?: "sms" | "email";
  status?: DsarTrackedStatus;
  documentSubmitted?: boolean;
  error?: { code: string; fields: { field: string; code: string }[] };
};

export async function startTracking(key: string, locale: string, prev: TrackState, form: FormData): Promise<TrackState> {
  const requestNo = String(form.get("request_no") ?? "").trim();
  const method = String(form.get("method") ?? "otp_email") as DsarTrackMethod;
  const contact = String(form.get("contact") ?? "").trim();
  if (!requestNo || !contact) {
    return { idem: prev.idem, error: { code: "missing_fields", fields: [] } };
  }
  try {
    const res = await startTrackingVerification(key, prev.idem, locale, { request_no: requestNo, method, contact });
    if (res.ok) return { idem: prev.idem, requestNo, method, contact, verificationId: res.result.verification_id, channel: res.result.channel };
    if (res.problem.status >= 500) return { idem: prev.idem, error: { code: "unavailable", fields: [] } };
    return { idem: randomUUID(), error: { code: res.problem.code, fields: res.problem.errors ?? [] } };
  } catch {
    return { idem: prev.idem, error: { code: "unavailable", fields: [] } };
  }
}

async function checkStatus(key: string, locale: string, prev: TrackState, form: FormData): Promise<TrackState> {
  if (!prev.requestNo || !prev.method || !prev.contact || !prev.verificationId) return prev;
  const code = String(form.get("code") ?? "").trim();
  try {
    const res = await getTrackedStatus(key, randomUUID(), locale, {
      request_no: prev.requestNo, method: prev.method, contact: prev.contact, verification_id: prev.verificationId,
      code: code || undefined,
    });
    if (res.ok) return { ...prev, status: res.status, error: undefined };
    return { ...prev, error: { code: res.problem.code, fields: res.problem.errors ?? [] } };
  } catch {
    return { ...prev, error: { code: "unavailable", fields: [] } };
  }
}

// trackSession dispatches by the form's own "intent" field — a single useActionState hook on the client
// side, so a status check's result and a later document submission share one, always-current state object
// (two independent hooks would each start from the same stale initial value and never see each other's
// updates).
export async function trackSession(key: string, locale: string, prev: TrackState, form: FormData): Promise<TrackState> {
  const intent = String(form.get("intent") ?? "");
  if (intent === "submit_document") return submitDocument(key, locale, prev, form);
  return checkStatus(key, locale, prev, form);
}

async function submitDocument(key: string, locale: string, prev: TrackState, form: FormData): Promise<TrackState> {
  if (!prev.requestNo || !prev.method || !prev.contact || !prev.verificationId) return prev;
  const file = form.get("file");
  if (!(file instanceof File) || file.size === 0) {
    return { ...prev, error: { code: "missing_fields", fields: [] } };
  }
  try {
    const res = await submitTrackingDocument(key, randomUUID(), locale,
      { request_no: prev.requestNo, method: prev.method, contact: prev.contact, verification_id: prev.verificationId }, file);
    if (res.ok) return { ...prev, documentSubmitted: true, error: undefined };
    return { ...prev, error: { code: res.problem.code, fields: res.problem.errors ?? [] } };
  } catch {
    return { ...prev, error: { code: "unavailable", fields: [] } };
  }
}
