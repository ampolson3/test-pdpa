"use client";

import { useLocale, useTranslations } from "next-intl";
import { formatDate, type Locale } from "@pdpa/i18n";
import type { SlaStatus } from "@pdpa/api-client";

const tone: Record<SlaStatus, string> = {
  on_track: "bg-emerald-50 text-emerald-800 ring-emerald-200",
  at_risk: "bg-amber-50 text-amber-800 ring-amber-200",
  overdue: "bg-red-50 text-red-800 ring-red-200",
  paused: "bg-slate-100 text-slate-700 ring-slate-200",
  done: "bg-slate-50 text-slate-600 ring-slate-200",
};

/** The SLA badge (PLT-05): status colour plus the due time in the viewer's locale (Buddhist Era in th). */
export function SlaBadge({ status, dueAt }: { status: SlaStatus; dueAt?: string | null }) {
  const t = useTranslations("workflow.sla");
  const locale = useLocale() as Locale;
  return (
    <span className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs ring-1 ${tone[status]}`} data-sla={status}>
      <span className="font-medium">{t(status)}</span>
      {dueAt && status !== "done" && <span>· {t("due", { date: formatDate(dueAt, locale, { month: "short", hour: "2-digit", minute: "2-digit" }) })}</span>}
    </span>
  );
}
