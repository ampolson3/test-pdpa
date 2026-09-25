"use client";

import { useLocale, useTranslations } from "next-intl";
import type { ConsentText } from "@pdpa/api-client";

export const input = "w-full rounded-md border border-slate-300 px-2 py-1.5";

/** A problem+json's title and detail, or its field errors' codes, for an inline error line. */
export function problemText(e: unknown): string {
  if (typeof e !== "object" || e === null) return "";
  const p = e as { title?: string; detail?: string; errors?: { field: string; code: string }[] };
  const fields = (p.errors ?? []).map((f) => `${f.field}: ${f.code}`).join(", ");
  return [p.title, p.detail, fields].filter(Boolean).join(" — ");
}

/** Text in the UI language, falling back to Thai. */
export function useText() {
  const locale = useLocale();
  return (t?: ConsentText | null) => (t ? (locale === "en" && t.en) || t.th : "");
}

/** Thai + English inputs for one ConsentText. */
export function TextPair({ label, value, onChange, multiline, testId }: { label: string; value?: ConsentText; onChange: (v: ConsentText) => void; multiline?: boolean; testId?: string }) {
  const t = useTranslations("consent");
  const v = value ?? { th: "" };
  const Field = multiline ? "textarea" : "input";
  return (
    <fieldset className="space-y-1">
      <legend className="font-medium">{label}</legend>
      <div className="grid gap-2 md:grid-cols-2">
        <label className="space-y-0.5">
          <span className="text-xs text-slate-500">{t("thai")}</span>
          <Field className={input} rows={multiline ? 3 : undefined} value={v.th} data-testid={testId && `${testId}-th`} onChange={(e) => onChange({ ...v, th: e.target.value })} />
        </label>
        <label className="space-y-0.5">
          <span className="text-xs text-slate-500">{t("english")}</span>
          <Field className={input} rows={multiline ? 3 : undefined} value={v.en ?? ""} data-testid={testId && `${testId}-en`} onChange={(e) => onChange({ ...v, en: e.target.value || undefined })} />
        </label>
      </div>
    </fieldset>
  );
}

const tone: Record<string, string> = {
  draft: "bg-slate-100 text-slate-700",
  active: "bg-emerald-50 text-emerald-800",
  retired: "bg-slate-50 text-slate-500",
  ACTIVE: "bg-emerald-50 text-emerald-800",
  NOT_GIVEN: "bg-slate-100 text-slate-700",
  WITHDRAWN: "bg-rose-50 text-rose-800",
  EXPIRED: "bg-amber-50 text-amber-800",
  PENDING: "bg-sky-50 text-sky-800",
};

export function StatusBadge({ status }: { status: string }) {
  const t = useTranslations("consent.status");
  return <span className={`rounded-full px-2 py-0.5 text-xs ${tone[status] ?? tone.draft}`} data-status={status}>{t(status)}</span>;
}
