"use server";

import { randomUUID } from "node:crypto";
import { downloadPublicPackage } from "@/lib/dsar-package";

export type DownloadState = {
  downloadUrl?: string;
  error?: { code: string; fields: { field: string; code: string }[] };
};

export async function downloadPackage(key: string, locale: string, _prev: DownloadState, form: FormData): Promise<DownloadState> {
  const accessCode = String(form.get("access_code") ?? "").trim();
  try {
    const res = await downloadPublicPackage(key, locale, accessCode, randomUUID());
    if (res.ok) return { downloadUrl: res.downloadUrl };
    if (res.problem.status >= 500) return { error: { code: "unavailable", fields: [] } };
    return { error: { code: res.problem.code, fields: res.problem.errors ?? [] } };
  } catch {
    return { error: { code: "unavailable", fields: [] } };
  }
}
