"use server";

import { randomUUID } from "node:crypto";
import { submitPublicRequest, confirmPublicVerification, type DsarVerification } from "@/lib/dsar";

export type SubmitState = {
  // Idempotency-Key of the next attempt: the same one while a submission may have reached the API (a double
  // click or a lost response replays the first result), a fresh one after the API answered with an error.
  idem: string;
  requestNo?: string;
  verification?: DsarVerification | null;
  confirmed?: boolean;
  error?: { code: string; fields: { field: string; code: string }[] };
};

export async function submitDsarRequest(key: string, locale: string, prev: SubmitState, form: FormData): Promise<SubmitState> {
  const requestTypeCode = String(form.get("request_type_code") ?? "");
  const requesterName = String(form.get("requester_name") ?? "").trim();
  const email = String(form.get("requester_email") ?? "").trim();
  const phone = String(form.get("requester_phone") ?? "").trim();
  const dataSource = String(form.get("data_source") ?? "").trim();
  if (!requestTypeCode || !requesterName || (!email && !phone)) {
    return { idem: prev.idem, error: { code: "missing_fields", fields: [] } };
  }
  try {
    const res = await submitPublicRequest(key, prev.idem, locale, {
      request_type_code: requestTypeCode,
      requester_name: requesterName,
      requester_email: email || undefined,
      requester_phone: phone || undefined,
      data_source: dataSource || undefined,
    });
    if (res.ok) return { idem: prev.idem, requestNo: res.result.request_no, verification: res.result.verification };
    if (res.problem.status >= 500) return { idem: prev.idem, error: { code: "unavailable", fields: [] } };
    return { idem: randomUUID(), error: { code: res.problem.code, fields: res.problem.errors ?? [] } };
  } catch {
    return { idem: prev.idem, error: { code: "unavailable", fields: [] } };
  }
}

export async function confirmOtp(
  key: string,
  requestId: string,
  verificationId: string,
  locale: string,
  prev: SubmitState,
  form: FormData,
): Promise<SubmitState> {
  const code = String(form.get("code") ?? "").trim();
  if (!code) return { ...prev, error: { code: "missing_fields", fields: [] } };
  try {
    const res = await confirmPublicVerification(key, requestId, randomUUID(), locale, { verification_id: verificationId, code });
    if (res.ok) return { ...prev, confirmed: res.verification.status === "passed", verification: res.verification, error: undefined };
    return { ...prev, error: { code: res.problem.code, fields: res.problem.errors ?? [] } };
  } catch {
    return { ...prev, error: { code: "unavailable", fields: [] } };
  }
}
